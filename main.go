package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	sml "github.com/mfmayer/gosml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	yaml "gopkg.in/yaml.v2"
)

type ObisConfig struct {
	Type   string       `yaml:"type"`
	Var    string       `yaml:"var"`
	MQTT   MQTTConfig   `yaml:"mqtt"`
	Metric MetricConfig `yaml:"metric"`
}
type MQTTConfig struct {
	Topic string `yaml:"topic"`
}
type MetricConfig struct {
	Name string `yaml:"name"`
	Help string `yaml:"help"`
	Type string `yaml:"type"`
}

type SmartmeterValueHandler func(string, ObisConfig, float64)

var debug bool

func main() {

	var serialDev, listen, configFile string
	var mqttServer, mqttUsername, mqttPassword, mqttTopicPrefix string
	flag.StringVar(&serialDev, "serial", "", "Serial device to read from")
	flag.StringVar(&listen, "metrics-address", ":9761", "The address to listen on for HTTP requests.")
	flag.StringVar(&mqttServer, "mqtt-server", "", "MQTT server to publish values to as they are received")
	flag.StringVar(&mqttUsername, "mqtt-username", "", "MQTT username")
	flag.StringVar(&mqttPassword, "mqtt-password", "", "MQTT password")
	flag.StringVar(&mqttTopicPrefix, "mqtt-topic-prefix", "smartmeter", "MQTT topic prefix for publishing values")
	flag.StringVar(&configFile, "config", "", "configfile with obis code mappings")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Parse()

	if _, err := os.Stat(serialDev); err != nil {
		log.Fatalf("No or invalid serial device given: %s", serialDev)
	}

	configData, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Error reading config file %s: %s", configFile, err)
	}
	var obisMappings map[string]ObisConfig
	if err := yaml.Unmarshal(configData, &obisMappings); err != nil {
		log.Fatalf("Error parsing config file %s: %s", configFile, err)
	}

	log.Printf("%#v", obisMappings)
	smartmeter := NewSmartmeterReader(serialDev, obisMappings)

	if mqttServer != "" {
		opts := MQTT.NewClientOptions()
		opts.AddBroker(mqttServer)
		opts.SetUsername(mqttUsername)
		opts.SetPassword(mqttPassword)
		client := MQTT.NewClient(opts)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Printf("Failed to connect to broker: %s", token.Error())
		}

		smartmeter.RegisterHandler(func(_ string, config ObisConfig, v float64) {
			if config.MQTT.Topic != "" {
				client.Publish(config.MQTT.Topic, 0, false, strconv.FormatFloat(v, 'f', -1, 64))
			}
		})

	}
	//we don't want to export the go garble
	registry := prometheus.NewRegistry()
	metricsMap := make(map[string]prometheus.GaugeVec)
	for obisCode, config := range obisMappings {
		if config.Metric.Name == "" {
			continue
		}
		metricsMap[obisCode] = *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: config.Metric.Name,
			Help: config.Metric.Help,
		}, []string{"server_id"})

		registry.MustRegister(metricsMap[obisCode])
	}

	smartmeter.RegisterHandler(func(code string, config ObisConfig, value float64) {
		metric, ok := metricsMap[code]
		if ok {
			metric.With(prometheus.Labels{"server_id": smartmeter.Var("server_id")}).Set(value)
		}
	})
	go func() {
		err := smartmeter.Run()
		if err != nil {
			log.Fatalf("smartmeter reader failed: %s", err)
		}
	}()

	log.Printf("Listening on %s", listen)
	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	log.Fatal(http.ListenAndServe(listen, nil))
}

type SmartmeterReader struct {
	serial    string
	mappings  map[string]ObisConfig
	handlers  []SmartmeterValueHandler
	variables map[string]string
}

func NewSmartmeterReader(serial string, config map[string]ObisConfig) *SmartmeterReader {
	return &SmartmeterReader{
		serial:    serial,
		mappings:  config,
		variables: make(map[string]string),
	}
}

func (s *SmartmeterReader) RegisterHandler(h SmartmeterValueHandler) {
	s.handlers = append(s.handlers, h)
}

func (s *SmartmeterReader) Run() error {
	if _, err := os.Stat(s.serial); os.IsNotExist(err) {
		return fmt.Errorf("file '%s' does not exist", s.serial)
	}
	for {
		f, err := os.OpenFile(s.serial, os.O_RDONLY|256, 0666)
		if err == nil {
			r := bufio.NewReader(f)
			log.Printf("Reading SML data from %s", s.serial)
			if err := sml.Read(r, sml.WithObisCallback(sml.OctetString{}, s.obisCallback)); err != nil {
				log.Printf("Error reading SML data: %s", err)
			}
		} else {
			log.Printf("Error opening file: %s", err)
		}
		time.Sleep(1 * time.Second)
	}
}
func (s *SmartmeterReader) Var(key string) string {
	return s.variables[key]
}

func (s *SmartmeterReader) obisCallback(msg *sml.ListEntry) {
	log.Printf("Got message:  %#v %s", msg.ObjectName(), strings.TrimSpace(msg.ValueString()))
	//log.Printf("Got message2:  %#v %#v", msg.ObjectName(), msg)

	code := msg.ObjectName()
	obisConfig, ok := s.mappings[code]
	if !ok {
		return
	}
	if obisConfig.Var != "" && obisConfig.Type == "string" {
		s.variables[obisConfig.Var] = msg.ValueString()
	}

	if obisConfig.Type == "float" || obisConfig.Type == "" {
		s.callHandlers(code, obisConfig, msg.Float())
	}
}

func (s *SmartmeterReader) callHandlers(code string, config ObisConfig, value float64) {
	for _, handler := range s.handlers {
		go handler(code, config, value)
	}
}
