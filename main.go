package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/jacobsa/go-serial/serial"
	sml "github.com/databus23/go-sml"
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

type SmartmeterValueHandler func(code string, config ObisConfig, value float64, unit string)

var debug bool

var (
	lastUpdateTime time.Time
	healthMutex    sync.Mutex
	healthTimeout  time.Duration
)

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
	flag.DurationVar(&healthTimeout, "health-timeout", 10*time.Second, "Timeout duration for health check (e.g., 10s, 1m)")
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

	smartmeter := NewSmartmeterReader(serialDev, obisMappings)

	if mqttServer != "" {
		opts := MQTT.NewClientOptions()
		opts.AddBroker(mqttServer)
		opts.SetUsername(mqttUsername)
		opts.SetPassword(mqttPassword)
		client := MQTT.NewClient(opts)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Fatalf("Failed to connect to broker: %s", token.Error())
		}

		smartmeter.RegisterHandler(func(_ string, config ObisConfig, v float64, _ string) {
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
		}, []string{"server_id", "unit"})

		registry.MustRegister(metricsMap[obisCode])
	}

	smartmeter.RegisterHandler(func(code string, config ObisConfig, value float64, unit string) {
		metric, ok := metricsMap[code]
		if ok {
			metric.With(prometheus.Labels{"server_id": smartmeter.Var("server_id"), "unit": unit}).Set(value)
		}
	})

	lastUpdateTime = time.Now()

	// Register a health check handler
	smartmeter.RegisterHandler(func(_ string, _ ObisConfig, _ float64, _ string) {
		healthMutex.Lock()
		lastUpdateTime = time.Now()
		healthMutex.Unlock()
	})

	// Start the health check endpoint
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		healthMutex.Lock()
		defer healthMutex.Unlock()

		if time.Since(lastUpdateTime) > healthTimeout {
			http.Error(w, "No updates received from smartmeter", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
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
		port, err := serial.Open(serial.OpenOptions{
			PortName:        s.serial,
			BaudRate:        9600,
			DataBits:        8,
			StopBits:        1,
			MinimumReadSize: 1,
		})
		if err == nil {
			r := bufio.NewReader(port)
			log.Printf("Reading SML data from %s", s.serial)
			if err := sml.Listen(context.Background(), r, func(f *sml.File) error {
				for _, entry := range f.Readings() {
					s.obisCallback(entry)
				}
				return nil
			}); err != nil {
				log.Printf("Error reading SML data: %s", err)
			}
			port.Close()
		} else {
			log.Printf("Error opening file: %s", err)
		}
		time.Sleep(1 * time.Second)
	}
}
func (s *SmartmeterReader) Var(key string) string {
	return s.variables[key]
}

func (s *SmartmeterReader) obisCallback(entry sml.ListEntry) {
	code := entry.OBISString()
	if debug {
		log.Printf("Got message: %s %v", code, entry.Value)
	}

	obisConfig, ok := s.mappings[code]
	if !ok {
		return
	}
	if obisConfig.Var != "" && obisConfig.Type == "string" {
		if v, ok := entry.Value.(sml.OctetString); ok {
			s.variables[obisConfig.Var] = hex.EncodeToString(v)
		}
	}

	if obisConfig.Type == "float" || obisConfig.Type == "" {
		if v, ok := entry.ScaledValue(); ok {
			s.callHandlers(code, obisConfig, v, entry.UnitString())
		}
	}
}

func (s *SmartmeterReader) callHandlers(code string, config ObisConfig, value float64, unit string) {
	for _, handler := range s.handlers {
		go handler(code, config, value, unit)
	}
}
