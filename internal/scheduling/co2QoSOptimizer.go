package scheduling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/serverledge-faas/serverledge/internal/function"
	"github.com/serverledge-faas/serverledge/internal/metrics"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/regions"
	"github.com/serverledge-faas/serverledge/internal/registration"
	"gopkg.in/yaml.v3"
)

type MultiRegionProbs struct {
	PLocal    float64
	PEdge     float64
	PDrop     float64
	PCloud    map[string]float64 // region -> prob
	PLocalVar float64            // probability mass for EXEC_VAR
}

type OptCarbonAwareParams struct {
	CloudRegions      map[string][]float64 `json:"cloud_regions"`      // region -> [mem, co2, procW, txJ/B, rxJ/B, cost]
	PossibleDecisions []string             `json:"possible_decisions"` // e.g., "LOCAL_EXEC", "OFFLOAD_EDGE", "OFFLOAD_CLOUD_<region>", "DROP"
	Functions         []string             `json:"functions"`          // function names
	Classes           []string             `json:"classes"`            // class names
	Variants          map[string][]string  `json:"variants"`           // base function -> list of variant names
	// [function][class] -> λ
	ArrivalRates map[string]map[string]float64 `json:"arrival_rates"`

	//LOCAL Per-function
	ServTimeLocal   map[string]float64 `json:"serv_time_local"`    // f -> seconds
	InitTimeLocal   map[string]float64 `json:"init_time_local"`    // f -> seconds
	ColdStartPLocal map[string]float64 `json:"cold_start_p_local"` // f -> seconds
	CpuUsageLocal   map[string]float64 `json:"cpu_usage_local"`    // f -> seconds

	// --- Flattened [region][function] -> value ---
	ServTimeCloud   map[string]map[string]float64 `json:"serv_time_cloud"`
	InitTimeCloud   map[string]map[string]float64 `json:"init_time_cloud"`
	ColdStartPCloud map[string]map[string]float64 `json:"cold_start_p_cloud"`
	CpuUsageCloud   map[string]map[string]float64 `json:"cpu_usage_cloud"`

	// Region-only maps
	OffloadTimeCloud map[string]float64 `json:"offload_time_cloud"` // region -> RTT seconds
	BandwidthCloud   map[string]float64 `json:"bandwidth_cloud"`    // region -> bytes/sec

	// Edge
	AggregatedEdgeMemory float64            `json:"aggregated_edge_memory"`
	ServTimeEdge         map[string]float64 `json:"serv_time_edge"`    // f -> seconds
	ColdStartPEdge       map[string]float64 `json:"cold_start_p_edge"` // f -> probability
	InitTimeEdge         map[string]float64 `json:"init_time_edge"`    // f -> seconds
	CpuUsageEdge         map[string]float64 `json:"cpu_usage_edge"`    // f -> seconds
	BandwidthEdge        float64            `json:"bandwidth_edge"`    // bytes/sec
	OffloadTimeEdge      float64            `json:"offload_time_edge"` // seconds

	AggregatedEdgePowerConsumption float64 `json:"aggregated_edge_power_consumption"` // W
	AggregatedEdgeEnergyTx         float64 `json:"aggregated_edge_energy_tx"`         // J/byte
	AggregatedEdgeEnergyRx         float64 `json:"aggregated_edge_energy_rx"`         // J/byte

	CO2GreenThreshold float64 `json:"co2_green_threshold"`
	Alpha             float64 `json:"alpha"`
	Beta              float64 `json:"beta"`

	// Local node
	NodeMemory                     float64 `json:"node_memory"`
	NodeCO2Footprint               float64 `json:"node_co2_footprint"`
	NodeProcessingPowerConsumption float64 `json:"node_processing_power_consumption"`
	NodeTxEnergyConsumption        float64 `json:"node_tx_energy_consumption"`
	NodeRxEnergyConsumption        float64 `json:"node_rx_energy_consumption"`

	Budget float64 `json:"budget"`

	// Per-function & per-class
	FunctionMemory         map[string]int64   `json:"function_memory"`           // f -> MB
	FunctionInputSizeMean  map[string]float64 `json:"function_input_size_mean"`  // f -> bytes (or chosen unit)
	FunctionOutputSizeMean map[string]float64 `json:"function_output_size_mean"` // f -> bytes (or chosen unit)

	ClassMaxRt           map[string]float64 `json:"class_maxRt"`            // class -> seconds
	ClassUtility         map[string]float64 `json:"class_utility"`          // class -> scalar
	VariantUtility       map[string]float64 `json:"variant_utility"`        // class -> scalar
	ClassDeadlinePenalty map[string]float64 `json:"class_deadline_penalty"` // class -> scalar
	ClassDropPenalty     map[string]float64 `json:"class_drop_penalty"`     // class -> scalar

}

func initOptCarbonAwareParams() OptCarbonAwareParams {
	return OptCarbonAwareParams{
		CloudRegions: make(map[string][]float64),
		Variants:     make(map[string][]string),
		ArrivalRates: make(map[string]map[string]float64),
		// Per-function
		FunctionMemory:         make(map[string]int64),
		FunctionInputSizeMean:  make(map[string]float64),
		FunctionOutputSizeMean: make(map[string]float64),
		// Flattened ([region][function] maps)
		ServTimeCloud:   make(map[string]map[string]float64),
		InitTimeCloud:   make(map[string]map[string]float64),
		ColdStartPCloud: make(map[string]map[string]float64),
		CpuUsageCloud:   make(map[string]map[string]float64),
		// Local  (per function)
		InitTimeLocal:   make(map[string]float64),
		ServTimeLocal:   make(map[string]float64),
		ColdStartPLocal: make(map[string]float64),
		CpuUsageLocal:   make(map[string]float64),
		// Region-only maps
		OffloadTimeCloud: make(map[string]float64),
		BandwidthCloud:   make(map[string]float64),
		// Edge
		ServTimeEdge:   make(map[string]float64),
		ColdStartPEdge: make(map[string]float64),
		InitTimeEdge:   make(map[string]float64),
		CpuUsageEdge:   make(map[string]float64),
		// Per-class
		ClassMaxRt:           make(map[string]float64),
		ClassUtility:         make(map[string]float64),
		VariantUtility:       make(map[string]float64),
		ClassDeadlinePenalty: make(map[string]float64),
		ClassDropPenalty:     make(map[string]float64),
	}
}

type QoSClass struct {
	Id              int64   `yaml:"id" json:"id"`
	Name            string  `yaml:"name" json:"name"`
	MaxRespTime     float64 `yaml:"max_resp_time" json:"max_resp_time"`
	Utility         float64 `yaml:"utility" json:"utility"`
	DeadlinePenalty float64 `yaml:"deadline_penalty" json:"deadline_penalty"`
	DropPenalty     float64 `yaml:"drop_penalty" json:"drop_penalty"`
}

var qosClasses = make(map[int64]QoSClass)

type Co2QosPolicyConfig struct {
	ArrivalRate       float64 `yaml:"policy.arrival.rate.alpha" json:"policy.arrival.rate.alpha"`
	UpdateIntervalSec int     `yaml:"policy.update.interval" json:"policy.update.interval"`
	Budget            float64 `yaml:"budget"`
	Alpha             float64 `yaml:"policy.alpha"`
	Beta              float64 `yaml:"policy.beta"`
	OptHost           string  `yaml:"optimizer.host"`
	OptPort           int     `yaml:"optimizer.port"`
	CO2GreenThreshold float64 `yaml:"policy.threshold.greenness"`
}

func LoadPolicyConfig(path string) (Co2QosPolicyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Co2QosPolicyConfig{}, fmt.Errorf("read file: %w", err)
	}
	var c Co2QosPolicyConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Co2QosPolicyConfig{}, fmt.Errorf("unmarshal yaml: %w", err)
	}
	return c, nil
}

func (policy *Co2QosAwarePolicy) prepareOptimizerParams() (OptCarbonAwareParams, error) {

	var LOCAL = registration.SelfRegistration.Key //local

	params := initOptCarbonAwareParams()

	allAreas := regions.GetAllAreas()
	fmt.Print("allAreas: ", allAreas)

	// Local node params setting
	params.NodeMemory = (float64)(node.LocalResources.AvailableMemory())
	params.NodeCO2Footprint = (float64)(node.LocalResources.Co2Footprint.Intensity())
	params.NodeProcessingPowerConsumption = float64(node.LocalResources.ProcessingPowerConsumption)
	params.NodeTxEnergyConsumption = float64(node.LocalResources.TxEnergyConsumption)
	params.NodeRxEnergyConsumption = float64(node.LocalResources.RxEnergyConsumption)

	edgeNodes := []string{LOCAL}
	nearbyServers := registration.GetFullNeighborInfo()

	//latency & energy Edge avgs
	if len(nearbyServers) > 0 {

		var (
			sumPower, sumTx, sumRx float64
			countAll               int
			sumDist                float64
			countEdge              int
		)

		distanceLocalToEdge := make(map[string]float64, len(nearbyServers))

		for key, s := range nearbyServers {
			if s == nil {
				continue
			}

			// For averages
			sumPower += s.ProcessingPowerConsumption
			sumTx += s.TxEnergyConsumption
			sumRx += s.RxEnergyConsumption
			countAll++

			// Consider as edge candidate only if it has CPU & memory
			availCPU := s.TotalCPU - s.UsedCPU
			availMem := s.TotalMemory - s.UsedMemory
			if availCPU > 0 && availMem > 0 {
				edgeNodes = append(edgeNodes, key)
				params.AggregatedEdgeMemory += float64(availMem)

				// LOCAL -> edge distance (seconds)
				d := registration.VivaldiClient.DistanceTo(&s.Coordinates).Seconds()
				distanceLocalToEdge[tupleKey(LOCAL, key)] = d

				sumDist += d
				countEdge++
			}
		}

		// Averages across all neighbors
		if countAll > 0 {
			//todo: dovrebbe essere calcolo a runtime xke se nuovo nodo si registra in nuova regione non lo so!
			params.AggregatedEdgePowerConsumption = sumPower / float64(countAll)
			params.AggregatedEdgeEnergyTx = sumTx / float64(countAll)
			params.AggregatedEdgeEnergyRx = sumRx / float64(countAll)
		}

		// Average LOCAL -> edge distance (seconds)
		if countEdge > 0 {
			params.OffloadTimeEdge = sumDist / float64(countEdge)
		} else {
			params.OffloadTimeEdge = 0
		}

	}

	loadBalancers := make(map[string]registration.NodeRegistration)
	if registration.CloudRegions == nil {
		registration.CloudRegions = make(map[string]regions.AreaInfo)
	}

	for i := range allAreas {
		areaName := allAreas[i].AreaName

		lbs, err := registration.GetLBInArea(areaName)
		if err != nil || len(lbs) == 0 {
			continue
		}
		for _, loadBalancer := range lbs {
			allAreas[i].LoadBalancerNode = loadBalancer.NodeID
			loadBalancers[areaName] = loadBalancer
			registration.CloudRegions[areaName] = allAreas[i]
			break
		}
	}

	// Build baseline decisions (EXEC, OFFLOAD_EDGE, DROP, OFFLOAD_CLOUD_*)
	params.PossibleDecisions, params.CloudRegions =
		regions.BuildCloudRegionsAndDecisions(registration.CloudRegions, fetchAreaStat)

	for _, lb := range loadBalancers {
		latSec, err := registration.GetTcpLatencySec(lb.IPAddress, lb.APIPort)
		if err != nil {
			continue
		}
		params.OffloadTimeCloud[lb.NodeID.Area] = latSec
	}

	budget := policy.Config.Budget
	params.Budget = budget

	// FUNCTIONS
	functionNames, err := function.GetAll()

	if err != nil {
		return OptCarbonAwareParams{}, fmt.Errorf("impossible obtain functionNames: %w", err)
	}

	retrievedMetrics := metrics.GetMetrics()
	//todo: continua qui

	for _, functionName := range functionNames {
		realFunc, ok := function.GetFunction(functionName)
		if !ok {
			log.Printf("Impossible get the function, skipping...")
			continue
		}

		// Avg input size (from metrics or defaults)
		var avgInputSize = 100.0
		if size, ok := retrievedMetrics.AvgInputSize[functionName]; ok && size > 0 {
			avgInputSize = size
		}
		realFunc.AvgInputSize = avgInputSize

		// Avg output size
		var avgOutputSize = 10.0
		if outputSize, ok := retrievedMetrics.AvgOutputSize[functionName]; ok && outputSize > 0 {
			avgOutputSize = outputSize
		}
		realFunc.AvgOutputSize = avgOutputSize

		// Basic function info
		params.Functions = append(params.Functions, functionName)
		params.FunctionMemory[functionName] = realFunc.MemoryMB
		params.FunctionInputSizeMean[functionName] = avgInputSize
		params.FunctionOutputSizeMean[functionName] = avgOutputSize

		// Retrieving variants of current functions
		if vs, err := function.GetVariantNamesOf(functionName); err == nil && len(vs) > 0 {
			params.Variants[functionName] = vs

			// variant utility
			for _, vName := range vs {
				vFunc, ok := function.GetFunction(vName)
				if !ok || vFunc == nil {
					log.Printf("prepareOptimizerParams: variant %s of %s not found in function registry", vName, functionName)
					continue
				}
				params.VariantUtility[vName] = vFunc.Utility
			}

		}

		execTimesEdge := make(map[string]float64) //[node]=float
		initTimesEdge := make(map[string]float64)
		coldStartsEdge := make(map[string]float64)
		cpuUsageEdge := make(map[string]float64)

		for _, n := range edgeNodes {
			nId := node.NodeID{Area: registration.SelfRegistration.Area, Key: n}

			// Execution Times
			execTime := 0.01 // Default
			if nodeTimes, ok := retrievedMetrics.AvgEdgeExecutionTime[nId.String()]; ok {
				if t, ok2 := nodeTimes[functionName]; ok2 {
					execTime = t
				}
			}

			// Init Times
			avgInit := 0.1 // Default
			if initTimes, ok := retrievedMetrics.AvgEdgeInitTime[nId.String()]; ok {
				if t, ok2 := initTimes[functionName]; ok2 {
					avgInit = t
				}
			}

			// Cold start
			pCold := 1.0
			if m, ok := retrievedMetrics.EdgeColdStartProbabilityByNode[nId.String()]; ok {
				if v, ok2 := m[functionName]; ok2 && !math.IsNaN(v) && !math.IsInf(v, 0) {
					pCold = v
				}
			}

			cpuUsage := 100.0
			if avgCpu, ok := retrievedMetrics.AvgEdgeCPUUsage[nId.String()]; ok {
				if u, ok2 := avgCpu[functionName]; ok2 {
					cpuUsage = u
				}
			}

			if !realFunc.IsDefault {
				execTime = execTime * (1 / realFunc.SpeedUp)
			}

			if n == LOCAL {
				// Fill LOCAL maps
				params.ServTimeLocal[functionName] = execTime
				params.ColdStartPLocal[functionName] = pCold
				params.InitTimeLocal[functionName] = avgInit
				params.CpuUsageLocal[functionName] = cpuUsage

			} else {
				execTimesEdge[n] = execTime
				initTimesEdge[n] = avgInit
				coldStartsEdge[n] = pCold
				cpuUsageEdge[n] = cpuUsage
			}
		}

		avgEdgeExecTimes := avgMap(execTimesEdge)
		avgEdgeInitTimes := avgMap(initTimesEdge)
		avgColdStartsEdge := avgMap(coldStartsEdge)
		avgCpuUsageEdge := avgMap(cpuUsageEdge)
		params.ServTimeEdge[functionName] = avgEdgeExecTimes
		params.InitTimeEdge[functionName] = avgEdgeInitTimes
		params.ColdStartPEdge[functionName] = avgColdStartsEdge
		params.CpuUsageEdge[functionName] = avgCpuUsageEdge

		params.BandwidthEdge = 0.0050

		for areaName := range registration.CloudRegions { // map[string]regions.AreaInfo

			// ensure inner maps exist
			if _, ok := params.ServTimeCloud[areaName]; !ok {
				params.ServTimeCloud[areaName] = make(map[string]float64)
			}
			if _, ok := params.ColdStartPCloud[areaName]; !ok {
				params.ColdStartPCloud[areaName] = make(map[string]float64)
			}
			if _, ok := params.InitTimeCloud[areaName]; !ok {
				params.InitTimeCloud[areaName] = make(map[string]float64)
			}
			if _, ok := params.CpuUsageCloud[areaName]; !ok {
				params.CpuUsageCloud[areaName] = make(map[string]float64)
			}

			// Exec time
			exec := 0.01
			if m, ok := retrievedMetrics.AvgCloudRegionExecutionTime[areaName]; ok {
				if v, ok := m[functionName]; ok && !math.IsNaN(v) && !math.IsInf(v, 0) {
					exec = v
				}
			}
			params.ServTimeCloud[areaName][functionName] = exec

			// Cold-start prob
			pCold := 1.0
			if m, ok := retrievedMetrics.CloudRegionColdStartProbability[areaName]; ok {
				if v, ok := m[functionName]; ok && !math.IsNaN(v) && !math.IsInf(v, 0) {
					pCold = v
				}
			}
			params.ColdStartPCloud[areaName][functionName] = pCold

			// Init time
			initAvg := 0.1
			if m, ok := retrievedMetrics.AvgCloudRegionInitTime[areaName]; ok {
				if v, ok := m[functionName]; ok && !math.IsNaN(v) && !math.IsInf(v, 0) {
					initAvg = v
				}
			}

			// Cpu Usage
			cpuUsage := 100.0
			if avgCpu, ok := retrievedMetrics.AvgCloudRegionCPUUsage[areaName]; ok {
				if u, ok := avgCpu[functionName]; ok && !math.IsNaN(u) && !math.IsInf(u, 0) {
					cpuUsage = u
				}
			}
			params.ServTimeCloud[areaName][functionName] = exec
			params.InitTimeCloud[areaName][functionName] = initAvg
			params.CpuUsageCloud[areaName][functionName] = cpuUsage

			params.BandwidthCloud[areaName] = 10000.0
		}
	}

	//remove variants from params.function
	variantNames := make(map[string]struct{})

	for _, vs := range params.Variants { // vs is []string of variant names
		for _, v := range vs {
			variantNames[v] = struct{}{}
		}
	}

	// As an extra safety, treat any function with IsDefault == false as a variant,
	// in case for some reason it wasn't added to params.Variants.
	for _, fname := range params.Functions {
		if f, ok := function.GetFunction(fname); ok && f != nil && !f.IsDefault {
			variantNames[fname] = struct{}{}
		}
	}

	// Filter params.Functions to keep ONLY base/default functions:
	// i.e., remove anything that is in variantNames.
	filtered := make([]string, 0, len(params.Functions))
	for _, fname := range params.Functions {
		if _, isVariant := variantNames[fname]; isVariant {
			// Skip this, it's a variant
			continue
		}
		filtered = append(filtered, fname)
	}
	params.Functions = filtered

	// adding EXEC_VAR among possible decisions
	hasAnyVariants := false
	for _, vs := range params.Variants {
		if len(vs) > 0 {
			hasAnyVariants = true
			break
		}
	}

	if hasAnyVariants {
		already := false
		for _, d := range params.PossibleDecisions {
			if d == "EXEC_VAR" {
				already = true
				break
			}
		}
		if !already {
			params.PossibleDecisions = append(params.PossibleDecisions, "EXEC_VAR")
		}
	}

	// CLASSES
	allClasses := getQosClasses()
	for _, class := range allClasses {
		currentClassName := class.Name
		params.Classes = append(params.Classes, currentClassName)
		params.ClassMaxRt[currentClassName] = class.MaxRespTime
		params.ClassUtility[currentClassName] = class.Utility
		params.ClassDeadlinePenalty[currentClassName] = class.DeadlinePenalty
		params.ClassDropPenalty[currentClassName] = class.DropPenalty
	}

	// Arrival Rates from policy struct (flat "f|class" -> λ)
	policy.arrivalRatesMutex.RLock()
	flat := make(map[string]float64)
	for k, v := range policy.arrivalRates {
		flat[k] = v
	}
	policy.arrivalRatesMutex.RUnlock()

	params.ArrivalRates = BuildNestedArrivalRates(flat, allClasses)
	params.Alpha = policy.AlphaWeight
	params.Beta = policy.BetaWeight
	params.CO2GreenThreshold = policy.Config.CO2GreenThreshold
	params.Budget = policy.Config.Budget
	//todo: bandwidth(va letta da conf)

	return params, nil
}

func fetchAreaStat(area string) (regions.AreaStat, error) {
	lbs, err := registration.GetLBInArea(area)
	if err != nil {
		return regions.AreaStat{}, err
	}
	for _, lb := range lbs {
		url := fmt.Sprintf("http://%s:%d/lb/stats", lb.IPAddress, lb.APIPort)

		client := &http.Client{Timeout: 4 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var st regions.AreaStat
		if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		return st, nil
	}
	return regions.AreaStat{}, fmt.Errorf("no reachable LB in area %q", area)
}

// BuildNestedArrivalRates converts a flat "func|class" -> λ map
// into a nested func -> class -> λ. If the class part is missing
// (e.g., "func|" or "func"), it uses the first class in `classes`
// as the default. Returns an empty map if no classes are provided.
func BuildNestedArrivalRates(
	flat map[string]float64,
	classes []QoSClass,
) map[string]map[string]float64 {

	out := make(map[string]map[string]float64)
	if len(classes) == 0 {
		return out
	}
	defaultClass := classes[0].Name

	for k, v := range flat {
		k = strings.TrimSpace(k)

		var fn, cls string
		if f, c, ok := strings.Cut(k, "|"); ok {
			fn = strings.TrimSpace(f)
			cls = strings.TrimSpace(c)
			if cls == "" {
				cls = defaultClass
			}
		} else {
			// No delimiter -> default class
			fn = k
			cls = defaultClass
		}

		if fn == "" || cls == "" {
			continue
		}
		if _, ok := out[fn]; !ok {
			out[fn] = make(map[string]float64)
		}
		out[fn][cls] = v
	}
	return out
}

func getQosClasses() []QoSClass {
	classes := make([]QoSClass, 0, len(qosClasses))
	for _, class := range qosClasses {
		classes = append(classes, class)
	}
	return classes
}

func getClassByID(id int64) (QoSClass, bool) {
	class, found := qosClasses[id]
	return class, found
}

// Qos Class yaml parsing
func loadQosClasses(filePath string) error {

	// Read yml
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading QoS YAML '%s': %w", filePath, err)
	}

	var configData struct {
		Classes []QoSClass `yaml:"classes"`
	}

	// 3. Esegue il parsing (Unmarshal) del contenuto del file nella struct.
	if err := yaml.Unmarshal(yamlFile, &configData); err != nil {
		return fmt.Errorf("errore nel parsing del file QoS YAML: %w", err)
	}

	for _, classDef := range configData.Classes {
		qosClasses[classDef.Id] = classDef
	}

	log.Printf("Recovered %d QoS classes with success.", len(qosClasses))
	return nil
}

// OptimizerResp: func -> class/policy -> action -> prob
type OptimizerResp map[string]map[string]map[string]float64

func (policy *Co2QosAwarePolicy) optimizerLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Polling: Begin strategic update...")

		policy.calculateArrivalRates()

		params, err := policy.prepareOptimizerParams()

		if err != nil {
			log.Printf("Error preparing parameters, skipping optimization: %v", err)
			continue
		}

		jsonData, err := json.Marshal(params)
		if err != nil {
			log.Printf("Polling: marshal error: %v", err)
			continue
		}

		url := fmt.Sprintf("http://%s:%d/", policy.Config.OptHost, policy.Config.OptPort)
		resp, err := policy.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Polling: optimizer call error: %v", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Polling: response status: %s; body=%s", resp.Status, string(body))
			continue
		}

		// parse nested JSON -> map["<func>|<class>"]MultiRegionProbs
		probs, varProbs, err := parseOptimizerResponse(body)
		if err != nil {
			log.Printf("Polling: decode error: %v; body=%s", err, string(body))
			continue
		}

		log.Printf("========probs from optimizer (probs): %v", probs)
		log.Printf("========probs from optimizer (varProbs): %v", varProbs)

		for k, v := range probs {
			policy.probabilityCache.Store(k, v)
		}
		for k, v := range varProbs {
			policy.variantProbCache.Store(k, v)
		}

	}
}

// helper: parse the optimizer's nested response
func parseOptimizerResponse(body []byte) (map[string]MultiRegionProbs, map[string]map[string]float64, error) {
	// expected shape:
	// {
	//   "probs": { func -> class -> decision -> prob },
	//   "decision_fc_probability_var": { func -> variant -> class -> prob }
	// }
	var raw struct {
		Probs    map[string]map[string]map[string]float64 `json:"probs"`
		VarProbs map[string]map[string]map[string]float64 `json:"decision_fc_probability_var"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, fmt.Errorf("unmarshal optimizer response: %w", err)
	}

	out := make(map[string]MultiRegionProbs, 8)

	for fn, classes := range raw.Probs {
		for class, decisions := range classes {
			probs := MultiRegionProbs{PCloud: make(map[string]float64, 4)}

			for dec, v := range decisions {
				switch {
				case dec == "DROP":
					probs.PDrop = v
				case dec == "EXEC":
					probs.PLocal = v
				case dec == "EXEC_VAR":
					probs.PLocalVar = v
				case dec == "OFFLOAD_EDGE":
					probs.PEdge = v
				case strings.HasPrefix(dec, "OFFLOAD_CLOUD_"):
					region := strings.TrimPrefix(dec, "OFFLOAD_CLOUD_")
					probs.PCloud[strings.ToLower(region)] = v
				default:
					// ignore unknown keys
				}
			}

			key := fn + "|" + class
			out[key] = probs
		}
	}

	// Build per-(func,class) variant distributions:
	// outVar["f1|critical"]["f1_var_0.8"] = p
	variantOut := make(map[string]map[string]float64)

	for fn, byVariant := range raw.VarProbs {
		for variantName, byClass := range byVariant {
			for class, p := range byClass {
				key := fn + "|" + class
				if _, ok := variantOut[key]; !ok {
					variantOut[key] = make(map[string]float64)
				}
				variantOut[key][variantName] = p
			}
		}
	}

	return out, variantOut, nil
}

// helper: parse the optimizer's nested response
func parseOptimizerResponseOld(body []byte) (map[string]MultiRegionProbs, error) {
	// expected shape: map[function]map[class]map[decision]float64
	var nested map[string]map[string]map[string]float64
	if err := json.Unmarshal(body, &nested); err != nil {
		return nil, fmt.Errorf("unmarshal optimizer response: %w", err)
	}

	log.Printf("========probs from optimizer (nested) : %v", nested)

	out := make(map[string]MultiRegionProbs, 8)

	for fn, classes := range nested {
		for class, decisions := range classes {
			probs := MultiRegionProbs{PCloud: make(map[string]float64, 4)}
			for dec, v := range decisions {
				switch {
				case dec == "DROP":
					probs.PDrop = v
				case dec == "EXEC":
					probs.PLocal = v
				case dec == "OFFLOAD_EDGE":
					probs.PEdge = v
				case strings.HasPrefix(dec, "OFFLOAD_CLOUD_"):
					region := strings.TrimPrefix(dec, "OFFLOAD_CLOUD_")
					// normalize to lowercase to match your other code (optional)
					probs.PCloud[strings.ToLower(region)] = v
				default:
					// ignore unknown keys
				}
			}

			key := fn + "|" + class

			out[key] = probs
		}
	}
	return out, nil
}

func (policy *Co2QosAwarePolicy) calculateArrivalRates() {

	policy.arrivalCountsMutex.Lock()
	countsSnapshot := policy.arrivalCounts
	policy.arrivalCounts = make(map[string]int64) // Reset for the next interval
	policy.arrivalCountsMutex.Unlock()

	elapsedSeconds := policy.updateInterval.Seconds()
	if elapsedSeconds == 0 {
		return // Evita divisione per zero
	}

	policy.arrivalRatesMutex.Lock()
	defer policy.arrivalRatesMutex.Unlock()

	for key, count := range countsSnapshot {
		measuredRate := float64(count) / elapsedSeconds
		oldRate := policy.arrivalRates[key] // Default: 0.0

		smoothedRate := policy.arrivalAlpha*measuredRate + (1.0-policy.arrivalAlpha)*oldRate

		policy.arrivalRates[key] = smoothedRate
	}
	fmt.Printf("Arrival rates update: %v", policy.arrivalRates)
}

func avgMap(m map[string]float64) float64 {
	if len(m) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range m {
		sum += v
	}
	return sum / float64(len(m))
}
