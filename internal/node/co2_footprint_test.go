package node

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testTimeColumn = "Datetime (UTC)"
const testIntensityColumn = "Carbon Intensity gCO2eq/kWh (direct)"

func writeTrace(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.csv")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write trace: %v", err)
	}
	return path
}

func TestLoadCO2TraceFromCSVValid(t *testing.T) {
	path := writeTrace(t, testTimeColumn+","+testIntensityColumn+"\n"+
		"2023-01-01 00:00:00,35.34\n"+
		"2023-01-01 01:00:00,36.53\n")

	trace, err := LoadCO2TraceFromCSV(path, testTimeColumn, testIntensityColumn, time.UTC)
	if err != nil {
		t.Fatalf("LoadCO2TraceFromCSV() error = %v", err)
	}
	samples := trace.Samples()
	if len(samples) != 2 {
		t.Fatalf("len(samples) = %d, want 2", len(samples))
	}
	if samples[0].Intensity != 35.34 {
		t.Fatalf("first intensity = %v, want 35.34", samples[0].Intensity)
	}
	if samples[0].Time.Location() != time.UTC {
		t.Fatalf("location = %v, want UTC", samples[0].Time.Location())
	}
}

func TestLoadCO2TraceFromCSVMalformed(t *testing.T) {
	missingColumn := writeTrace(t, testTimeColumn+",other\n2023-01-01 00:00:00,35.34\n")
	if _, err := LoadCO2TraceFromCSV(missingColumn, testTimeColumn, testIntensityColumn, time.UTC); err == nil {
		t.Fatal("LoadCO2TraceFromCSV() missing intensity column error = nil")
	}

	badIntensity := writeTrace(t, testTimeColumn+","+testIntensityColumn+"\n2023-01-01 00:00:00,not-a-number\n")
	if _, err := LoadCO2TraceFromCSV(badIntensity, testTimeColumn, testIntensityColumn, time.UTC); err == nil {
		t.Fatal("LoadCO2TraceFromCSV() bad intensity error = nil")
	}
}

func TestCO2TraceApplyNextBeginningAndEnd(t *testing.T) {
	path := writeTrace(t, testTimeColumn+","+testIntensityColumn+"\n"+
		"2023-01-01 00:00:00,10\n"+
		"2023-01-01 01:00:00,20\n")
	trace, err := LoadCO2TraceFromCSV(path, testTimeColumn, testIntensityColumn, time.UTC)
	if err != nil {
		t.Fatalf("LoadCO2TraceFromCSV() error = %v", err)
	}

	var footprint CarbonFootprint
	if !trace.ApplyNext(&footprint) {
		t.Fatal("first ApplyNext() = false")
	}
	_, intensity, idx := footprint.Snapshot()
	if intensity != 10 || idx != 1 {
		t.Fatalf("first snapshot intensity=%v idx=%d, want 10/1", intensity, idx)
	}

	if !trace.ApplyNext(&footprint) {
		t.Fatal("second ApplyNext() = false")
	}
	if trace.ApplyNext(&footprint) {
		t.Fatal("third ApplyNext() = true, want exhausted")
	}
	_, intensity, idx = footprint.Snapshot()
	if intensity != 20 || idx != 2 {
		t.Fatalf("final snapshot intensity=%v idx=%d, want 20/2", intensity, idx)
	}
}

func TestStartCO2FromCSVPeriodicUpdate(t *testing.T) {
	path := writeTrace(t, testTimeColumn+","+testIntensityColumn+"\n"+
		"2023-01-01 00:00:00,10\n"+
		"2023-01-01 01:00:00,20\n")
	LocalResources.Co2Footprint = CarbonFootprint{}

	stop, err := StartCO2FromCSV(context.Background(), path, testTimeColumn, testIntensityColumn, time.Millisecond, time.UTC)
	if err != nil {
		t.Fatalf("StartCO2FromCSV() error = %v", err)
	}
	defer stop()

	if got := LocalResources.CO2Intensity(); got != 10 {
		t.Fatalf("initial intensity = %v, want 10", got)
	}

	deadline := time.After(100 * time.Millisecond)
	for LocalResources.CO2Intensity() != 20 {
		select {
		case <-deadline:
			t.Fatalf("intensity did not advance, got %v", LocalResources.CO2Intensity())
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
