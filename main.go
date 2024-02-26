package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	verbrauchTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "smartmeter",
		Subsystem: "wirkarbeit",
		Name:      "verbrauch_wh_total",
		Help:      "Summe Wirkarbeit Verbrauch über alle Phasen",
	})

	einspeisungTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "smartmeter",
		Subsystem: "wirkarbeit",
		Name:      "einspeisung_wh_total",
		Help:      "Summe Wirkarbeit Einspeisung über alle Phasen",
	})

	wirkleistung = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "smartmeter",
		Subsystem: "wirkleistung",
		Name:      "gesamt_w",
		Help:      "gelieferte Leistung ueber alle Phasen",
	})
)

func main() {

	//we don')t want to export the go garble
	registry := prometheus.NewRegistry()
	registry.Register(verbrauchTotal)
	registry.Register(einspeisungTotal)
	registry.Register(wirkleistung)

	var serialDev, smlServerBinary, listen string
	var mqttServer, mqttUsername, mqttPassword, mqttTopicPrefix string
	flag.StringVar(&serialDev, "serial", "", "Serial device to read from")
	flag.StringVar(&smlServerBinary, "sml-server", "sml_server", "sml server binary to run")
	flag.StringVar(&listen, "metrics-address", ":9761", "The address to listen on for HTTP requests.")
	flag.StringVar(&mqttServer, "mqtt-server", "", "MQTT server to publish values to as they are received")
	flag.StringVar(&mqttUsername, "mqtt-username", "", "MQTT username")
	flag.StringVar(&mqttPassword, "mqtt-password", "", "MQTT password")
	flag.StringVar(&mqttTopicPrefix, "mqtt-topic-prefix", "smartmeter", "MQTT topic prefix for publishing values")
	flag.Parse()

	if _, err := os.Stat(serialDev); err != nil {
		log.Fatalf("No or invalid serial device given: %s", serialDev)
	}

	smartmeter := NewSmartmeterReader(smlServerBinary, serialDev)

	if mqttServer != "" {
		opts := MQTT.NewClientOptions()
		opts.AddBroker(mqttServer)
		opts.SetUsername(mqttUsername)
		opts.SetPassword(mqttPassword)
		client := MQTT.NewClient(opts)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Println("Failed to connect to broker: %s", token.Error())
		}

		smartmeter.RegisterHandler(func(t SmartmeterDataType, v float64) {
			switch t {
			case SmartmeterMomentaneWirkleistung:
				client.Publish(mqttTopicPrefix+"/momentane-wirkleistung", 0, false, strconv.Itoa(int(math.Round(v))))
			}
		})

	}

	smartmeter.RegisterHandler(func(t SmartmeterDataType, value float64) {
		switch t {
		case SmartmeterMomentaneWirkleistung:
			wirkleistung.Set(value)
		case SmartmeterPositiveWirkenergieTariflos:
			verbrauchTotal.Set(value)
		case SmartmeterNegativeWirkenergieTariflos:
			einspeisungTotal.Set(value)
		default:
			return
		}

	})
	go func() {
		err := smartmeter.Run()
		if err != nil {
			log.Fatalf("smartreader failed: %s", err)
		}
	}()

	log.Printf("Listening on %s", listen)
	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	log.Fatal(http.ListenAndServe(listen, nil))

}

type SmartmeterDataType string

const (
	SmartmeterPositiveWirkenergieTariflos SmartmeterDataType = "1-0:1.8.0*255"
	SmartmeterNegativeWirkenergieTariflos SmartmeterDataType = "1-0:2.8.0*255"
	SmartmeterMomentaneWirkleistung       SmartmeterDataType = "1-0:16.7.0*255"
)

type SmartmeterValueHandler func(SmartmeterDataType, float64)

type SmartmeterReader struct {
	binary, serial string
	handlers       []SmartmeterValueHandler
}

func NewSmartmeterReader(binary string, serial string) *SmartmeterReader {

	return &SmartmeterReader{
		binary: binary,
		serial: serial,
	}

}

func (s *SmartmeterReader) RegisterHandler(h SmartmeterValueHandler) {
	s.handlers = append(s.handlers, h)
}

func (s *SmartmeterReader) Run() error {
	for {
		cmd := exec.Command(s.binary, s.serial)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("Failed to create pipe for sub process: %w", err)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("Failed to start: %v: %w", strings.Join(cmd.Args, " "), err)
		}
		log.Printf("Started %v", strings.Join(cmd.Args, " "))

		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				s.processLine(scanner.Text())
			}
		}()
		log.Printf("%s exited: %s", s.binary, cmd.Wait())
	}
}

var smlServerRE = regexp.MustCompile(`^([^:]+:[^*]+\*\d+)#([^#]+)#(.*)`)

func (s *SmartmeterReader) processLine(line string) {
	// 1-0:96.50.1*1#EMH#
	// 1-0:96.1.0*255# XX XX XX XX XX #
	// 1-0:1.8.0*255#768.2#Wh
	// 1-0:16.7.0*255#-178#W

	matches := smlServerRE.FindStringSubmatch(line)
	if matches == nil {
		log.Println("Ignoring invalid line: %s", line)
	}
	obis := matches[1]
	value := matches[2]
	//unit := matches[3]

	switch SmartmeterDataType(obis) {
	case SmartmeterPositiveWirkenergieTariflos:
		s.callHandlers(SmartmeterPositiveWirkenergieTariflos, value)
	case SmartmeterNegativeWirkenergieTariflos:
		s.callHandlers(SmartmeterNegativeWirkenergieTariflos, value)
	case SmartmeterMomentaneWirkleistung:
		s.callHandlers(SmartmeterMomentaneWirkleistung, value)
	}
}

func (s *SmartmeterReader) callHandlers(t SmartmeterDataType, value string) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("Failed to parse value %v for %s: %s", value, t, err)
		return
	}
	for _, handler := range s.handlers {
		go handler(t, v)
	}
}
