package regions

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ErrAreaNotFound returns a deterministic lookup error for a missing area name.
func ErrAreaNotFound(areaName string) error {
	return fmt.Errorf("regions: area %q not found in config", areaName)
}

type rawConfig struct {
	Regions []AreaInfo `mapstructure:"regions"`
}

// Load reads and validates a region scenario YAML file.
func Load(filename string) (*Config, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, fmt.Errorf("regions: empty config file path")
	}

	v := viper.New()
	v.SetConfigFile(filename)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("regions: read config: %w", err)
	}

	var raw rawConfig
	if err := v.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("regions: parse config: %w", err)
	}
	if len(raw.Regions) == 0 {
		return nil, fmt.Errorf("regions: no regions configured")
	}

	byName := make(map[string]AreaInfo, len(raw.Regions))
	areas := make([]AreaInfo, 0, len(raw.Regions))
	for i, area := range raw.Regions {
		area.Name = strings.TrimSpace(area.Name)
		area.CO2TraceFile = strings.TrimSpace(area.CO2TraceFile)
		if area.Name == "" {
			return nil, fmt.Errorf("regions: regions[%d].name is required", i)
		}
		if area.CO2TraceFile == "" {
			return nil, fmt.Errorf("regions: co2.traces.file is required for area %q", area.Name)
		}
		if _, exists := byName[area.Name]; exists {
			return nil, fmt.Errorf("regions: duplicate area %q", area.Name)
		}
		byName[area.Name] = area
		areas = append(areas, area)
	}

	cfg := &Config{
		areas:      areas,
		byName:     byName,
		sourcePath: filename,
		header: HeaderConfig{
			TimestampColumn: strings.TrimSpace(v.GetString("header.timestamp")),
			IntensityColumn: strings.TrimSpace(v.GetString("header.intensity")),
		},
	}
	if v.IsSet("poll.interval.sec") {
		cfg.pollEvery = time.Duration(v.GetInt("poll.interval.sec")) * time.Second
	}
	return cfg, nil
}

// ReadRegionConfiguration loads a region file and returns the selected area
// plus the configured poll interval. It does not retain package-global state.
func ReadRegionConfiguration(filename, areaName string) (AreaInfo, time.Duration, error) {
	cfg, err := Load(filename)
	if err != nil {
		return AreaInfo{}, 0, err
	}
	area, ok := cfg.Area(areaName)
	if !ok {
		return AreaInfo{}, 0, ErrAreaNotFound(areaName)
	}
	return area, cfg.PollInterval(), nil
}
