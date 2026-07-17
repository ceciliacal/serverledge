package emissions

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/spf13/viper"
)

func TestComputeLocalEmissions(t *testing.T) {
	got := Compute(Inputs{
		DurationSec:             10,
		FunctionMemory:          1024,
		CurrentNodePowerCons:    100,
		CurrentNodeCO2Intensity: 360,
		CPUUsage:                0.5,
	})
	want := 0.025
	if got != want {
		t.Fatalf("Compute() = %v, want %v", got, want)
	}
}

func TestComputeOffloadedEmissions(t *testing.T) {
	got := Compute(Inputs{
		DurationSec:             10,
		FunctionMemory:          1024,
		CurrentNodePowerCons:    100,
		CurrentNodeCO2Intensity: 360,
		InputSizeMean:           1000,
		OutputSizeMean:          500,
		InitialNodeRxEnergy:     0.001,
		InitialNodeTxEnergy:     0.002,
		LocalNodeRxEnergy:       0.003,
		LocalNodeTxEnergy:       0.004,
		AggrInitialNodeMemory:   1024,
		CPUUsage:                0.5,
	})
	want := 0.02575
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("Compute() = %v, want %v", got, want)
	}
}

func TestComputeOffloadedWithoutInitialMemory(t *testing.T) {
	got := Compute(Inputs{
		InitialNodeRxEnergy:     0.001,
		CurrentNodeCO2Intensity: 360,
	})
	if got != 0 {
		t.Fatalf("Compute() = %v, want 0", got)
	}
}

func TestSetupCO2FromConfigDisabled(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	stop, err := SetupCO2FromConfig("ROME")
	if err != nil {
		t.Fatalf("SetupCO2FromConfig() error = %v", err)
	}
	if stop == nil {
		t.Fatal("SetupCO2FromConfig() returned nil stop function")
	}
	stop()
}

func TestSetupCO2FromConfigRegionFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	tmp := t.TempDir()
	tracePath := filepath.Join(tmp, "trace.csv")
	err := os.WriteFile(tracePath, []byte("ts,intensity\n2023-01-01 00:00:00,77\n"), 0o600)
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	regionsPath := filepath.Join(tmp, "regions.yaml")
	err = os.WriteFile(regionsPath, []byte("regions:\n"+
		"- name: ROME\n"+
		"  co2.traces.file: "+tracePath+"\n"+
		"poll.interval.sec: 3600\n"+
		"header.timestamp: ts\n"+
		"header.intensity: intensity\n"), 0o600)
	if err != nil {
		t.Fatalf("write regions: %v", err)
	}

	viper.Set(config.REGIONS_FILE_PATH, regionsPath)
	viper.Set(config.CO2_TRACE_UPDATE_INTERVAL_SEC, int(time.Hour.Seconds()))

	node.LocalResources.Co2Footprint = node.CarbonFootprint{}
	stop, err := SetupCO2FromConfig("ROME")
	if err != nil {
		t.Fatalf("SetupCO2FromConfig() error = %v", err)
	}
	defer stop()

	if got := node.LocalResources.CO2Intensity(); got != 77 {
		t.Fatalf("CO2Intensity = %v, want 77", got)
	}
}
