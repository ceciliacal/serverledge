package emissions

const MU = 2048.0 //m_u

// Inputs contains everything needed to compute CO2 for one invocation
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
}

// Compute returns emitted grams of CO2 for the given inputs
func Compute(in Inputs) float64 {
	if in.InitialNodeTxEnergy == 0 && in.InitialNodeRxEnergy == 0 {
		energyTerm := in.CurrentNodePowerCons * (in.FunctionMemory / MU) * in.DurationSec
		return (energyTerm / (3600.0 * 1000.0)) * in.CurrentNodeCO2Intensity
	}
	if in.AggrInitialNodeMemory <= 0 {
		return 0
	} else {
		energyTerm :=
			(in.CurrentNodePowerCons * (in.FunctionMemory / MU) * in.DurationSec) +
				in.InputSizeMean*(in.LocalNodeTxEnergy+in.InitialNodeRxEnergy) +
				in.OutputSizeMean*(in.InitialNodeTxEnergy+in.LocalNodeRxEnergy)

		return (energyTerm / (3600.0 * 1000.0)) * in.CurrentNodeCO2Intensity
	}

}
