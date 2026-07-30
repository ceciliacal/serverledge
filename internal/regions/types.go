package regions

import "time"

// HeaderConfig describes the CO2 trace CSV columns configured at the top level
// of a region YAML file.
type HeaderConfig struct {
	// TimestampColumn is read from YAML key `header.timestamp`.
	// Type: string. Meaning: CSV column containing sample timestamps.
	// Unit: none. Required: no; CO2 setup falls back to its default column.
	// Current use: CO2 trace loading. Future use: optimizer trace inputs.
	TimestampColumn string

	// IntensityColumn is read from YAML key `header.intensity`.
	// Type: string. Meaning: CSV column containing carbon intensity samples.
	// Unit: gCO2eq/kWh. Required: no; CO2 setup falls back to its default column.
	// Current use: CO2 trace loading. Future use: optimizer trace inputs.
	IntensityColumn string
}

// AreaInfo describes one configured Serverledge area.
type AreaInfo struct {
	// Name is read from YAML key `name`.
	// Type: string. Meaning: stable area identifier matching registry.area or registry.remote.area.
	// Unit: none. Required: yes.
	// Current use: deterministic area lookup for CO2 trace selection.
	// Future use: optimizer area and cloud-region decision identifiers.
	Name string `mapstructure:"name"`

	// CO2TraceFile is read from YAML key `co2.traces.file`.
	// Type: string. Meaning: path to the area's carbon-intensity CSV trace.
	// Unit: file path. Required: yes.
	// Current use: local CO2 trace loading. Future use: per-area CO2 data for optimizer inputs.
	CO2TraceFile string `mapstructure:"co2.traces.file"`

	// Cost is read from YAML key `cost`.
	// Type: float64. Meaning: configured execution cost for the area.
	// Unit: scenario-specific monetary cost. Required: no.
	// Current use: none. Future use: CO2/QoS optimizer budget and cost terms.
	Cost float64 `mapstructure:"cost"`

	// ProcessingPowerConsumption is read from YAML key `consumption.processing.power`.
	// Type: float64. Meaning: processing power consumed by nodes in this area.
	// Unit: watts. Required: no.
	// Current use: none. Future use: emissions estimation in the optimizer.
	ProcessingPowerConsumption float64 `mapstructure:"consumption.processing.power"`

	// TxEnergyConsumption is read from YAML key `consumption.energy.tx`.
	// Type: float64. Meaning: transmit energy per byte for this area.
	// Unit: joules per byte, matching the emissions package inputs.
	// Required: no. Current use: none. Future use: network emissions estimation.
	TxEnergyConsumption float64 `mapstructure:"consumption.energy.tx"`

	// RxEnergyConsumption is read from YAML key `consumption.energy.rx`.
	// Type: float64. Meaning: receive energy per byte for this area.
	// Unit: joules per byte, matching the emissions package inputs.
	// Required: no. Current use: none. Future use: network emissions estimation.
	RxEnergyConsumption float64 `mapstructure:"consumption.energy.rx"`
}

type AreaStat struct {
	MemoryAvailable            float64 `json:"memory_available"`
	CO2Intensity               float64 `json:"co2_intensity"`
	ProcessingPowerConsumption float64 `json:"processing_power_consumption"`
	TxEnergyConsumption        float64 `json:"tx_energy_consumption"`
	RxEnergyConsumption        float64 `json:"rx_energy_consumption"`
}

// Config is an immutable snapshot of one region YAML file.
type Config struct {
	areas      []AreaInfo
	byName     map[string]AreaInfo
	pollEvery  time.Duration
	header     HeaderConfig
	sourcePath string
}

// Area returns a configured area by exact name.
func (c *Config) Area(name string) (AreaInfo, bool) {
	if c == nil {
		return AreaInfo{}, false
	}
	area, ok := c.byName[name]
	return area, ok
}

// Areas returns all configured areas in file order.
func (c *Config) Areas() []AreaInfo {
	if c == nil {
		return nil
	}
	out := make([]AreaInfo, len(c.areas))
	copy(out, c.areas)
	return out
}

// EdgeAreas returns the configured local edge area identified by registry.area.
func (c *Config) EdgeAreas(localArea string) []AreaInfo {
	area, ok := c.Area(localArea)
	if !ok {
		return nil
	}
	return []AreaInfo{area}
}

// PollInterval returns the YAML key `poll.interval.sec` as a duration.
// Type: integer seconds. Required: no. Current use: CO2 trace update cadence.
// Future use: shared area polling cadence for optimizer inputs.
func (c *Config) PollInterval() time.Duration {
	if c == nil {
		return 0
	}
	return c.pollEvery
}

// Header returns the configured CO2 trace header mapping.
func (c *Config) Header() HeaderConfig {
	if c == nil {
		return HeaderConfig{}
	}
	return c.header
}

// SourcePath returns the path used to load this configuration.
func (c *Config) SourcePath() string {
	if c == nil {
		return ""
	}
	return c.sourcePath
}
