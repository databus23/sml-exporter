package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	sml "github.com/mfmayer/gosml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	verbrauchTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "smartmeter",
		Subsystem: "wirkarbeit",
		Name:      "verbrauch_wh_total",
		Help:      "Summe Wirkarbeit Verbrauch über alle Phasen",
	}, []string{"server_id"})

	einspeisungTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "smartmeter",
		Subsystem: "wirkarbeit",
		Name:      "einspeisung_wh_total",
		Help:      "Summe Wirkarbeit Einspeisung über alle Phasen",
	}, []string{"server_id"})

	wirkleistung = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "smartmeter",
		Subsystem: "wirkleistung",
		Name:      "gesamt_w",
		Help:      "gelieferte Leistung ueber alle Phasen",
	}, []string{"server_id"})
)

func main() {

	//we don')t want to export the go garble
	registry := prometheus.NewRegistry()
	if err := registry.Register(verbrauchTotal); err != nil {
		log.Fatalf("Error registering verbrauch_wh_total metric: %s", err)
	}
	if err := registry.Register(einspeisungTotal); err != nil {
		log.Fatalf("Error registering einspeisung_wh_total metric: %s", err)
	}
	if err := registry.Register(wirkleistung); err != nil {
		log.Fatalf("Error registering gesamt_w metric: %s", err)
	}

	var serialDev, listen string
	var mqttServer, mqttUsername, mqttPassword, mqttTopicPrefix string
	flag.StringVar(&serialDev, "serial", "", "Serial device to read from")
	flag.StringVar(&listen, "metrics-address", ":9761", "The address to listen on for HTTP requests.")
	flag.StringVar(&mqttServer, "mqtt-server", "", "MQTT server to publish values to as they are received")
	flag.StringVar(&mqttUsername, "mqtt-username", "", "MQTT username")
	flag.StringVar(&mqttPassword, "mqtt-password", "", "MQTT password")
	flag.StringVar(&mqttTopicPrefix, "mqtt-topic-prefix", "smartmeter", "MQTT topic prefix for publishing values")
	flag.Parse()

	if _, err := os.Stat(serialDev); err != nil {
		log.Fatalf("No or invalid serial device given: %s", serialDev)
	}

	smartmeter := NewSmartmeterReader(serialDev)

	if mqttServer != "" {
		opts := MQTT.NewClientOptions()
		opts.AddBroker(mqttServer)
		opts.SetUsername(mqttUsername)
		opts.SetPassword(mqttPassword)
		client := MQTT.NewClient(opts)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Printf("Failed to connect to broker: %s", token.Error())
		}

		smartmeter.RegisterHandler(func(_ string, t SmartmeterDataType, v float64) {
			switch t {
			case SmartmeterMomentaneWirkleistung:
				client.Publish(mqttTopicPrefix+"/momentane-wirkleistung", 0, false, strconv.Itoa(int(math.Round(v))))
			}
		})

	}

	smartmeter.RegisterHandler(func(serverID string, t SmartmeterDataType, value float64) {
		switch t {
		case SmartmeterMomentaneWirkleistung:
			wirkleistung.WithLabelValues(serverID).Set(value)
		case SmartmeterPositiveWirkenergieTariflos:
			verbrauchTotal.WithLabelValues(serverID).Set(value)
		case SmartmeterNegativeWirkenergieTariflos:
			einspeisungTotal.WithLabelValues(serverID).Set(value)
		default:
			return
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

type SmartmeterDataType string

const (
	SmartmeterPositiveWirkenergieTariflos SmartmeterDataType = "1-0:1.8.0*255"
	SmartmeterNegativeWirkenergieTariflos SmartmeterDataType = "1-0:2.8.0*255"
	SmartmeterMomentaneWirkleistung       SmartmeterDataType = "1-0:16.7.0*255"
	SmartmeterMomentaneWirkleistungL1     SmartmeterDataType = "1-0:36.7.0*255"
	SmartmeterMomentaneWirkleistungL2     SmartmeterDataType = "1-0:56.7.0*255"
	SmartmeterMomentaneWirkleistungL3     SmartmeterDataType = "1-0:76.7.0*255"
	SmartmeterMomentaneSpannungL1         SmartmeterDataType = "1-0:32.7.0*255"
	SmartmeterMomentaneSpannungL2         SmartmeterDataType = "1-0:52.7.0*255"
	SmartmeterMomentaneSpannungL3         SmartmeterDataType = "1-0:72.7.0*255"
	SmartmeterServerID                    SmartmeterDataType = "1-0:96.1.0*255"
)

type SmartmeterValueHandler func(string, SmartmeterDataType, float64)

type SmartmeterReader struct {
	serial   string
	handlers []SmartmeterValueHandler
	serverID string
}

func NewSmartmeterReader(serial string) *SmartmeterReader {
	return &SmartmeterReader{
		serial: serial,
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

func (s *SmartmeterReader) obisCallback(msg *sml.ListEntry) {
	log.Printf("Got message: %#v %#v", msg.ObjectName(), msg.ValueString())
	switch SmartmeterDataType(msg.ObjectName()) {
	case SmartmeterServerID:
		s.serverID = msg.ValueString()
	case SmartmeterPositiveWirkenergieTariflos:
		s.callHandlers(SmartmeterPositiveWirkenergieTariflos, msg.Float())
	case SmartmeterNegativeWirkenergieTariflos:
		s.callHandlers(SmartmeterNegativeWirkenergieTariflos, msg.Float())
	case SmartmeterMomentaneWirkleistung:
		s.callHandlers(SmartmeterMomentaneWirkleistung, msg.Float())
	}
}

func (s *SmartmeterReader) callHandlers(t SmartmeterDataType, value float64) {
	for _, handler := range s.handlers {
		go handler(s.serverID, t, value)
	}
}
