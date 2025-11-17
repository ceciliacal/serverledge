package emissions

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/regions"
)

func SetupFromConfig(myArea string) (stop func(), err error) {
	regionConfigFile := config.GetString(config.REGIONS_FILE_PATH, "")
	area, pollInterval, err := regions.ReadRegionConfiguration(regionConfigFile, myArea)
	if err != nil {
		return nil, err
	}
	if area.CO2TracesFile == "" {
		return nil, fmt.Errorf("CO2: empty CSV path")
	}

	timeCol := config.GetString("CO2_TIME_COLUMN", "Datetime (UTC)")
	intCol := config.GetString("CO2_INTENSITY_COLUMN", "Carbon Intensity gCO₂eq/kWh (direct)")
	tzName := config.GetString("CO2_TIMEZONE", "UTC")
	loc, tzErr := time.LoadLocation(tzName)
	if tzErr != nil {
		log.Printf("CO2: bad timezone %q: %v; using UTC", tzName, tzErr)
		loc = time.UTC
	}

	ctx, cancel := context.WithCancel(context.Background())

	if _, err := node.StartCO2FromCSV(ctx, area.CO2TracesFile, timeCol, intCol, pollInterval, loc); err != nil {
		cancel()
		return nil, err
	}

	//go func() {
	//	ticker := time.NewTicker(5 * time.Second)
	//	defer ticker.Stop()
	//	for range ticker.C {
	//		t, v, i := node.LocalResources.Co2Footprint.Snapshot()
	//		log.Printf("[NODE] CO2: t=%s intensity=%.3f gCO2/kWh idx=%d",
	//			t.Format(time.RFC3339), v, i)
	//	}
	//}()

	return cancel, nil
}
