package emissions

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/spf13/viper"
)

const (
	defaultCO2TimeColumn      = "Datetime (UTC)"
	defaultCO2IntensityColumn = "Carbon Intensity gCO₂eq/kWh (direct)"
	defaultCO2Timezone        = "UTC"
	defaultCO2UpdateInterval  = time.Hour
)

type regionTraceConfig struct {
	Name         string `mapstructure:"name"`
	CO2TraceFile string `mapstructure:"co2.traces.file"`
}

// SetupCO2FromConfig starts local carbon-intensity tracking when regions.file.path
// is configured. Missing configuration is intentionally a no-op so existing
// deployments keep the previous zero-value behavior.
func SetupCO2FromConfig(area string) (func(), error) {
	traceFile, interval, timeCol, intensityCol, err := traceConfig(area)
	if err != nil {
		return nil, err
	}
	if traceFile == "" {
		return func() {}, nil
	}
	tzName := config.GetString(config.CO2_TRACE_TIMEZONE, defaultCO2Timezone)
	loc, tzErr := time.LoadLocation(tzName)
	if tzErr != nil {
		log.Printf("CO2: bad timezone %q: %v; using UTC", tzName, tzErr)
		loc = time.UTC
	}

	return node.StartCO2FromCSV(context.Background(), traceFile, timeCol, intensityCol, interval, loc)
}

func traceConfig(area string) (string, time.Duration, string, string, error) {
	interval := time.Duration(config.GetInt(config.CO2_TRACE_UPDATE_INTERVAL_SEC, int(defaultCO2UpdateInterval.Seconds()))) * time.Second
	if interval <= 0 {
		interval = defaultCO2UpdateInterval
	}
	timeCol := config.GetString(config.CO2_TRACE_TIME_COLUMN, defaultCO2TimeColumn)
	intensityCol := config.GetString(config.CO2_TRACE_INTENSITY_COLUMN, defaultCO2IntensityColumn)

	regionsFile := config.GetString(config.REGIONS_FILE_PATH, "")
	if regionsFile == "" {
		return "", 0, timeCol, intensityCol, nil
	}

	v := viper.New()
	v.SetConfigFile(regionsFile)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return "", 0, "", "", fmt.Errorf("CO2: could not read regions config: %w", err)
	}

	var regions []regionTraceConfig
	if err := v.UnmarshalKey("regions", &regions); err != nil {
		return "", 0, "", "", fmt.Errorf("CO2: could not parse regions config: %w", err)
	}
	for _, r := range regions {
		if r.Name == area {
			if r.CO2TraceFile == "" {
				return "", 0, "", "", fmt.Errorf("CO2: empty trace file for area %q", area)
			}
			if v.IsSet("poll.interval.sec") {
				if poll := time.Duration(v.GetInt("poll.interval.sec")) * time.Second; poll > 0 {
					interval = poll
				}
			}
			if v.IsSet("header.timestamp") {
				timeCol = v.GetString("header.timestamp")
			}
			if v.IsSet("header.intensity") {
				intensityCol = v.GetString("header.intensity")
			}
			return r.CO2TraceFile, interval, timeCol, intensityCol, nil
		}
	}

	return "", 0, "", "", fmt.Errorf("CO2: area %q not found in regions config", area)
}
