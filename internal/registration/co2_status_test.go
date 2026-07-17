package registration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hexablock/vivaldi"
	"github.com/serverledge-faas/serverledge/internal/node"
)

func setupVivaldiForTest(t *testing.T) {
	t.Helper()
	cfg := vivaldi.DefaultConfig()
	cfg.Dimensionality = 3
	client, err := vivaldi.NewClient(cfg)
	if err != nil {
		t.Fatalf("vivaldi.NewClient() error = %v", err)
	}
	VivaldiClient = client
}

func TestCurrentStatusInformationIncludesCO2AndExistingFields(t *testing.T) {
	setupVivaldiForTest(t)
	node.LocalResources.Init()
	node.LocalResources.Co2Footprint.Set(time.Unix(0, 0), 42.5, 1)

	payload, err := getCurrentStatusInformation()
	if err != nil {
		t.Fatalf("getCurrentStatusInformation() error = %v", err)
	}

	var status StatusInformation
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if status.CO2Intensity != 42.5 {
		t.Fatalf("CO2Intensity = %v, want 42.5", status.CO2Intensity)
	}
	if status.TotalMemory == 0 {
		t.Fatal("TotalMemory was not preserved")
	}
	if status.AvailableMemory == 0 {
		t.Fatal("AvailableMemory was not preserved")
	}
	if status.FreeMemory == 0 {
		t.Fatal("FreeMemory was not preserved")
	}
}

func TestNeighborInfoUpdatePreservesAdvertisedCO2AndTimestamp(t *testing.T) {
	neighborInfo = make(map[string]*StatusInformation)
	updateNeighborInfoLocked("node-a", &StatusInformation{
		CO2Intensity:    123.4,
		TotalMemory:     2048,
		AvailableMemory: 1024,
		FreeMemory:      512,
	}, 99)

	got := neighborInfo["node-a"]
	if got == nil {
		t.Fatal("neighbor info was not stored")
	}
	if got.CO2Intensity != 123.4 {
		t.Fatalf("CO2Intensity = %v, want 123.4", got.CO2Intensity)
	}
	if got.LastUpdateTime != 99 {
		t.Fatalf("LastUpdateTime = %v, want 99", got.LastUpdateTime)
	}
	if got.AvailableMemory != 1024 || got.FreeMemory != 512 {
		t.Fatalf("memory fields not preserved: %+v", got)
	}
}

func TestParseEtcdRegisteredNodePreservesArchitecture(t *testing.T) {
	reg, err := parseEtcdRegisteredNode("area-a", "node-a", []byte("127.0.0.1;1323;9876;arm64"))
	if err != nil {
		t.Fatalf("parseEtcdRegisteredNode() error = %v", err)
	}
	if reg.Area != "area-a" || reg.Key != "node-a" || reg.Arch != "arm64" {
		t.Fatalf("NodeID = %+v, want area/key/arm64", reg.NodeID)
	}
	if reg.APIPort != 1323 || reg.UDPPort != 9876 {
		t.Fatalf("ports = %d/%d, want 1323/9876", reg.APIPort, reg.UDPPort)
	}
}
