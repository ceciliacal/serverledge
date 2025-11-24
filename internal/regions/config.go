package regions

import (
	"fmt"
	"log"
	"sort"
	"strings"
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

// ReadRegionConfiguration loads region scenario YAML, populates package state, and returns the selected area + poll interval.
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

// PollInterval returns the configured poll interval.
func PollInterval() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	return pollEvery
}

// vec layout:
//
//	[0]=avg mem (bytes, LB) [1]=CO2 (LB)
//	[2]=proc power W (static) [3]=tx J/B (static)
//	[4]=rx J/B (static)       [5]=cost (static)
func BuildCloudRegionsAndDecisionsEnriched(
	cloudRegionsWithLB map[string]AreaInfo,
	fetch func(area string) (AreaStat, error)) ([]string, map[string][]float64) {
	decisions := []string{"EXEC", "OFFLOAD_EDGE", "DROP"}

	paramsCloudRegions := make(map[string][]float64)

	// stable ordering
	areaNames := make([]string, 0)

	//se num nodi cloudRegionsWithLB = 0 non deve stare in cloudRegionsWithLB regions
	for name, _ := range cloudRegionsWithLB {
		areaNames = append(areaNames, name)
	}
	sort.Strings(areaNames)

	for _, areaName := range areaNames {
		ai := cloudRegionsWithLB[areaName]

		// start from static defaults
		vec := make([]float64, 6)

		// optionally enrich from LB stats
		if fetch != nil {
			if st, err := fetch(areaName); err == nil {
				vec[0] = st.AvgAvailableMem
				vec[1] = st.CO2
				ai.NumNodes = st.NodeCount

				vec[2] = st.ProcessingPowerConsumption
				vec[3] = st.TxEnergyConsumption /// 1e9
				vec[4] = st.RxEnergyConsumption /// 1e9
				vec[5] = st.Cost
			}
		}

		if ai.NumNodes > 0 {
			decisions = append(decisions, "OFFLOAD_CLOUD_"+strings.ToUpper(areaName))
			paramsCloudRegions[areaName] = vec
			cloudRegionsWithLB[areaName] = ai
		} else {
			log.Printf("Removing %s among cloud regions as its stats report numNodes: %f", areaName)
		}

	}

	return decisions, paramsCloudRegions
}
