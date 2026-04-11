package main

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"

	sml "github.com/databus23/go-sml"
)

// listenTestData parses sml.bin and dispatches each reading to the reader's obisCallback.
func listenTestData(t *testing.T, reader *SmartmeterReader) {
	t.Helper()
	data, err := os.ReadFile("sml.bin")
	if err != nil {
		t.Fatalf("Failed to read test data: %s", err)
	}
	err = sml.Listen(context.Background(), bytes.NewReader(data), func(f *sml.File) error {
		for _, entry := range f.Readings() {
			reader.obisCallback(entry)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sml.Listen failed: %s", err)
	}
}

// countFloatReadings counts how many float-typed OBIS readings the test data produces for the given mappings.
func countFloatReadings(t *testing.T, mappings map[string]ObisConfig) int {
	t.Helper()
	data, err := os.ReadFile("sml.bin")
	if err != nil {
		t.Fatalf("Failed to read test data: %s", err)
	}
	count := 0
	sml.Listen(context.Background(), bytes.NewReader(data), func(f *sml.File) error {
		for _, entry := range f.Readings() {
			cfg, ok := mappings[entry.OBISString()]
			if !ok {
				continue
			}
			if cfg.Type == "float" || cfg.Type == "" {
				if _, ok := entry.ScaledValue(); ok {
					count++
				}
			}
		}
		return nil
	})
	return count
}

func TestObisCallbackWithWireData(t *testing.T) {
	mappings := map[string]ObisConfig{
		"1-0:96.1.0*255": {Type: "string", Var: "server_id"},
		"1-0:1.8.0*255":  {Type: "float"},
		"1-0:16.7.0*255": {Type: "float"},
	}

	reader := NewSmartmeterReader("", mappings)

	type reading struct {
		code  string
		value float64
		unit  string
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var received []reading

	wg.Add(countFloatReadings(t, mappings))
	reader.RegisterHandler(func(code string, _ ObisConfig, value float64, unit string) {
		mu.Lock()
		received = append(received, reading{code, value, unit})
		mu.Unlock()
		wg.Done()
	})

	listenTestData(t, reader)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("Expected handler to be called, got no calls")
	}

	foundEnergy := false
	foundPower := false
	for _, r := range received {
		switch r.code {
		case "1-0:1.8.0*255":
			foundEnergy = true
			if r.value < 21000 || r.value > 22000 {
				t.Errorf("Unexpected energy value: %f (expected ~21570)", r.value)
			}
			if r.unit != "Wh" {
				t.Errorf("Expected energy unit 'Wh', got %q", r.unit)
			}
		case "1-0:16.7.0*255":
			foundPower = true
			if r.value < 1000 || r.value > 1200 {
				t.Errorf("Unexpected power value: %f (expected ~1100)", r.value)
			}
			if r.unit != "W" {
				t.Errorf("Expected power unit 'W', got %q", r.unit)
			}
		}
	}
	if !foundEnergy {
		t.Error("No energy reading (1-0:1.8.0*255) received")
	}
	if !foundPower {
		t.Error("No power reading (1-0:16.7.0*255) received")
	}

	// String variables are stored synchronously in obisCallback (no goroutine)
	// OctetString values are hex-encoded to ensure valid UTF-8 for Prometheus labels
	serverID := reader.Var("server_id")
	if serverID == "" {
		t.Error("Expected server_id variable to be set")
	}
	if serverID != "0a0149534b00051e809c" {
		t.Errorf("Expected server_id to be hex-encoded, got %q", serverID)
	}
}

func TestObisCallbackSkipsUnmappedCodes(t *testing.T) {
	mappings := map[string]ObisConfig{
		"1-0:16.7.0*255": {Type: "float"},
	}

	reader := NewSmartmeterReader("", mappings)

	expectedCalls := countFloatReadings(t, mappings)

	var wg sync.WaitGroup
	var mu sync.Mutex
	callCount := 0

	wg.Add(expectedCalls)
	reader.RegisterHandler(func(_ string, _ ObisConfig, _ float64, _ string) {
		mu.Lock()
		callCount++
		mu.Unlock()
		wg.Done()
	})

	listenTestData(t, reader)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if callCount != expectedCalls {
		t.Errorf("Expected %d handler calls (one per frame), got %d", expectedCalls, callCount)
	}
}

func TestObisCallbackDefaultTypeIsFloat(t *testing.T) {
	mappings := map[string]ObisConfig{
		"1-0:16.7.0*255": {}, // no Type set — should default to float behavior
	}

	reader := NewSmartmeterReader("", mappings)

	expectedCalls := countFloatReadings(t, mappings)

	var wg sync.WaitGroup
	var mu sync.Mutex
	called := false

	wg.Add(expectedCalls)
	reader.RegisterHandler(func(_ string, _ ObisConfig, v float64, _ string) {
		mu.Lock()
		called = true
		mu.Unlock()
		if v < 1000 || v > 1200 {
			t.Errorf("Unexpected power value: %f", v)
		}
		wg.Done()
	})

	listenTestData(t, reader)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Error("Expected handler to be called for default (empty) type")
	}
}
