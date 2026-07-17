package regions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadValidRegionConfiguration(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := cfg.PollInterval(); got != 3*time.Second {
		t.Fatalf("PollInterval() = %v, want 3s", got)
	}
	header := cfg.Header()
	if header.TimestampColumn != "Datetime (UTC)" {
		t.Fatalf("TimestampColumn = %q", header.TimestampColumn)
	}
	if header.IntensityColumn != "Carbon Intensity gCO2eq/kWh (direct)" {
		t.Fatalf("IntensityColumn = %q", header.IntensityColumn)
	}

	areas := cfg.Areas()
	if len(areas) != 2 {
		t.Fatalf("len(Areas()) = %d, want 2", len(areas))
	}
	if areas[0].Name != "cloud_region_a" || areas[1].Name != "region_a" {
		t.Fatalf("areas in file order = %#v", areas)
	}
}

func TestAreaLookup(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	area, ok := cfg.Area("region_a")
	if !ok {
		t.Fatal("Area(region_a) not found")
	}
	if area.CO2TraceFile != "./co2_traces/FR_2023_hourly.csv" {
		t.Fatalf("CO2TraceFile = %q", area.CO2TraceFile)
	}
	if _, ok := cfg.Area("missing"); ok {
		t.Fatal("Area(missing) found, want absent")
	}
}

func TestEdgeAreaClassification(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	edges := cfg.EdgeAreas("region_a")
	if len(edges) != 1 || edges[0].Name != "region_a" {
		t.Fatalf("EdgeAreas(region_a) = %#v", edges)
	}
	if got := cfg.EdgeAreas("unknown"); len(got) != 0 {
		t.Fatalf("EdgeAreas(unknown) = %#v, want empty", got)
	}
}

func TestReadRegionConfigurationUnknownArea(t *testing.T) {
	_, _, err := ReadRegionConfiguration(filepath.Join("testdata", "valid.yaml"), "unknown")
	if err == nil {
		t.Fatal("ReadRegionConfiguration() error = nil")
	}
	if !strings.Contains(err.Error(), `area "unknown" not found`) {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsDuplicateAreaNames(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "duplicate.yaml"))
	if err == nil {
		t.Fatal("Load() duplicate error = nil")
	}
	if !strings.Contains(err.Error(), "duplicate area") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsMissingRequiredFields(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "missing_trace.yaml"))
	if err == nil {
		t.Fatal("Load() missing field error = nil")
	}
	if !strings.Contains(err.Error(), "co2.traces.file is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "malformed.yaml"))
	if err == nil {
		t.Fatal("Load() malformed error = nil")
	}
}

func TestRepeatedLoadsDoNotKeepStaleState(t *testing.T) {
	tmp := t.TempDir()
	first := filepath.Join(tmp, "first.yaml")
	second := filepath.Join(tmp, "second.yaml")
	if err := os.WriteFile(first, []byte("regions:\n- name: first\n  co2.traces.file: ./first.csv\n"), 0o600); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte("regions:\n- name: second\n  co2.traces.file: ./second.csv\n"), 0o600); err != nil {
		t.Fatalf("write second: %v", err)
	}

	firstCfg, err := Load(first)
	if err != nil {
		t.Fatalf("Load(first) error = %v", err)
	}
	secondCfg, err := Load(second)
	if err != nil {
		t.Fatalf("Load(second) error = %v", err)
	}

	if _, ok := firstCfg.Area("first"); !ok {
		t.Fatal("first config lost first area")
	}
	if _, ok := secondCfg.Area("first"); ok {
		t.Fatal("second config contains stale first area")
	}
	if _, ok := secondCfg.Area("second"); !ok {
		t.Fatal("second config missing second area")
	}
}
