package emissions

const memoryUnitMB = 2048.0

// Inputs contains the values needed to estimate grams of CO2 emitted by one
// invocation. Power is in watts, carbon intensity is gCO2/kWh, data-transfer
// energy is J/byte, memory is MB, duration is seconds, and CPUUsage is a
// fraction of one core.
type Inputs struct {
	DurationSec             float64
	FunctionMemory          float64
	CurrentNodePowerCons    float64
	CurrentNodeCO2Intensity float64
	InputSizeMean           float64
	OutputSizeMean          float64
	InitialNodeRxEnergy     float64
	InitialNodeTxEnergy     float64
	LocalNodeRxEnergy       float64
	LocalNodeTxEnergy       float64
	AggrInitialNodeMemory   float64
	CPUUsage                float64
}

// Compute returns estimated emitted grams of CO2 for the supplied invocation.
func Compute(in Inputs) float64 {
	computeEnergyJ := in.CurrentNodePowerCons * (in.FunctionMemory / memoryUnitMB) * in.DurationSec * in.CPUUsage
	if in.InitialNodeTxEnergy == 0.0 && in.InitialNodeRxEnergy == 0.0 {
		return joulesToKWh(computeEnergyJ) * in.CurrentNodeCO2Intensity
	}
	if in.AggrInitialNodeMemory <= 0.0 {
		return 0.0
	}

	transferEnergyJ := in.InputSizeMean*(in.LocalNodeTxEnergy+in.InitialNodeRxEnergy) +
		in.OutputSizeMean*(in.InitialNodeTxEnergy+in.LocalNodeRxEnergy)
	return joulesToKWh(computeEnergyJ+transferEnergyJ) * in.CurrentNodeCO2Intensity
}

func joulesToKWh(joules float64) float64 {
	return joules / (3600.0 * 1000.0)
}
