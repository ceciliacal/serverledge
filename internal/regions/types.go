package regions

import (
	"strings"

	"github.com/serverledge-faas/serverledge/internal/node"
)

type AreaInfo struct {
	AreaName                   string  `mapstructure:"name"`
	CO2TracesFile              string  `mapstructure:"co2.traces.file"`
	Cost                       float64 `mapstructure:"cost"`
	ProcessingPowerConsumption float64 `mapstructure:"consumption.processing.power"`
	TxEnergyConsumption        float64 `mapstructure:"consumption.energy.tx"`
	RxEnergyConsumption        float64 `mapstructure:"consumption.energy.rx"`
	LoadBalancerNode           node.NodeID
}

// region -> [mem, co2, procW, txJ/B, rxJ/B, cost]
func BuildCloudRegionsAndDecisions(areas map[string]AreaInfo) ([]string, map[string][]float64) {
	decisions := []string{"LOCAL_EXEC", "OFFLOAD_EDGE", "DROP"}
	cloudRegions := make(map[string][]float64)

	for _, a := range areas {

		decisions = append(decisions, "OFFLOAD_CLOUD_"+strings.ToUpper(a.AreaName))
		regionFeatures := make([]float64, 0)
		regionFeatures = append(regionFeatures, 800.0) //todo leggi mem disp da qualche parte
		regionFeatures = append(regionFeatures, 650.0) //todo leggi co2 corrente da qualche parte
		regionFeatures = append(regionFeatures, a.ProcessingPowerConsumption)
		regionFeatures = append(regionFeatures, a.TxEnergyConsumption)
		regionFeatures = append(regionFeatures, a.RxEnergyConsumption)
		regionFeatures = append(regionFeatures, a.Cost)

		cloudRegions[a.AreaName] = regionFeatures
	}

	return decisions, cloudRegions
}
