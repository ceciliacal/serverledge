package regions

import (
	"time"

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
	NumNodes                   int
}

type AreaStat struct {
	AvgAvailableMem            float64   `json:"avg_available_mem"`
	CO2                        float64   `json:"co2_intensity"`
	NodeCount                  int       `json:"node_count"`
	UpdatedAt                  time.Time `json:"updated_at"`
	ProcessingPowerConsumption float64   `mapstructure:"consumption.processing.power"`
	TxEnergyConsumption        float64   `mapstructure:"consumption.energy.tx"`
	RxEnergyConsumption        float64   `mapstructure:"consumption.energy.rx"`
	Cost                       float64   `mapstructure:"cost"`
}
