package emissions

import (
	"context"
	"log"
	"time"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/regions"
)

const (
	defaultCO2TimeColumn      = "Datetime (UTC)"
	defaultCO2IntensityColumn = "Carbon Intensity gCO₂eq/kWh (direct)"
	defaultCO2Timezone        = "UTC"
	defaultCO2UpdateInterval  = time.Hour
)

// SetupCO2FromConfig starts local carbon-intensity tracking when either
// co2.trace.file or regions.file.path is configured. Missing configuration is
// intentionally a no-op so existing deployments keep the previous zero-value
// behavior.
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

	if traceFile := config.GetString(config.CO2_TRACE_FILE, ""); traceFile != "" {
		return traceFile, interval, timeCol, intensityCol, nil
	}

	regionsFile := config.GetString(config.REGIONS_FILE_PATH, "")
	if regionsFile == "" {
		return "", 0, timeCol, intensityCol, nil
	}

	regionConfig, err := regions.Load(regionsFile)
	if err != nil {
		return "", 0, "", "", err
	}
	regionArea, ok := regionConfig.Area(area)
	if !ok {
		return "", 0, "", "", regions.ErrAreaNotFound(area)
	}
	if poll := regionConfig.PollInterval(); poll > 0 {
		interval = poll
	}
	header := regionConfig.Header()
	if header.TimestampColumn != "" {
		timeCol = header.TimestampColumn
	}
	if header.IntensityColumn != "" {
		intensityCol = header.IntensityColumn
	}
	return regionArea.CO2TraceFile, interval, timeCol, intensityCol, nil
}
