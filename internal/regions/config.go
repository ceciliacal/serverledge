package regions

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
)

var (
	mu        sync.RWMutex
	allAreas  []AreaInfo
	byName    map[string]AreaInfo
	pollEvery time.Duration
)

// ReadRegionConfiguration loads YAML, populates package state, and returns the selected area + poll interval.
func ReadRegionConfiguration(filename, myArea string) (AreaInfo, time.Duration, error) {
	if filename == "" {
		return AreaInfo{}, 0, fmt.Errorf("no CO2 config file specified")
	}

	v := viper.New()
	v.SetConfigFile(filename)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return AreaInfo{}, 0, fmt.Errorf("could not read CO2 config file: %w", err)
	}

	// Unmarshal regions
	var rs []AreaInfo
	if err := v.UnmarshalKey("regions", &rs); err != nil {
		return AreaInfo{}, 0, fmt.Errorf("unmarshal regions: %w", err)
	}

	// Build lookup (local temps first)
	tmpByName := make(map[string]AreaInfo, len(rs))
	for _, r := range rs {
		if r.AreaName == "" {
			continue
		}
		tmpByName[r.AreaName] = r
	}

	mu.Lock()
	allAreas = rs
	byName = tmpByName
	pollEvery = time.Duration(v.GetInt("poll.interval.sec")) * time.Second
	mu.Unlock()

	// Validate selection
	area, ok := Get(myArea)
	if !ok {
		return AreaInfo{}, 0, fmt.Errorf("area %q not found in config", myArea)
	}
	if area.CO2TracesFile == "" {
		return AreaInfo{}, 0, fmt.Errorf("co2.traces.file is missing for area %q", myArea)
	}

	return area, PollInterval(), nil
}

// Get returns a single AreaInfo by name.
func Get(name string) (AreaInfo, bool) {
	mu.RLock()
	defer mu.RUnlock()
	a, ok := byName[name]
	return a, ok
}

// GetAllAreas returns a copy of all loaded areas.
func GetAllAreas() []AreaInfo {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]AreaInfo, len(allAreas))
	copy(out, allAreas)
	return out
}

// Names returns all area names.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(allAreas))
	for _, a := range allAreas {
		names = append(names, a.AreaName)
	}
	return names
}

// PollInterval returns the configured poll interval.
func PollInterval() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	return pollEvery
}
