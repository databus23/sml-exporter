package main

import (
	"bufio"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	energyCounterTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "smartmeter",
		Subsystem: "wirkarbeit",
		Name:      "verbrauch_wh_total",
		Help:      "Summe Wirkarbeit Verbrauch über alle Phasen",
	})

	powerTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "smartmeter",
		Subsystem: "wirkleistung",
		Name:      "gesamt_w",
		Help:      "gelieferte Leistung ueber alle Phasen",
	})
)

func main() {

	//we don't want to export the go garble
	registry := prometheus.NewRegistry()
	registry.Register(energyCounterTotal)
	registry.Register(powerTotal)

	var serialDev, smlServerBinary, listen string
	flag.StringVar(&serialDev, "serial", "", "Serial device to read from")
	flag.StringVar(&smlServerBinary, "sml-server", "sml_server", "sml server binary to run")
	flag.StringVar(&listen, "metrics-address", ":9761", "The address to listen on for HTTP requests.")
	flag.Parse()

	if _, err := os.Stat(serialDev); err != nil {
		log.Fatalf("No or invalid serial device given: %s", serialDev)
	}

	go readInput(smlServerBinary, serialDev)

	log.Printf("Listening on %s", listen)
	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	log.Fatal(http.ListenAndServe(listen, nil))

}

func readInput(binary, serial string) {
	for {
		cmd := exec.Command(binary, serial)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Fatalf("Failed to create pipe for sub process: %s", err)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Fatalf("Failed to start: %v: %s", strings.Join(cmd.Args, " "), err)
		}
		log.Printf("Started %v", strings.Join(cmd.Args, " "))

		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				processLine(scanner.Text())
			}
		}()

		log.Printf("%s exited: %s", binary, cmd.Wait())

	}
}

var smlServerRE = regexp.MustCompile(`^([^:]+:[^*]+\*\d+)#([^#]+)#(.*)`)

func processLine(line string) {
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

	switch obis {
	case "1-0:1.8.0*255":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			log.Printf("Failed to parse value %v for %s: %s", value, obis, err)
			return
		}
		energyCounterTotal.Set(v)
	case "1-0:16.7.0*255":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			log.Printf("Failed to parse value %v for %s: %s", value, obis, err)
			return
		}
		powerTotal.Set(v)
	}
}
