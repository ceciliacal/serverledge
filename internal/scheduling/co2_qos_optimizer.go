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

const (
	defaultServiceTimeSec = 0.01
	defaultInitTimeSec    = 0.1
	defaultInputSize      = 100.0
	defaultOutputSize     = 10.0
	defaultBandwidthEdge  = 5000.0
	defaultBandwidthCloud = 10000.0
	defaultCPUUsage       = 100.0
)

type MultiRegionProbs struct {
	PLocal float64
	PEdge  float64
	PDrop  float64
	PCloud map[string]float64
}

type OptCarbonAwareParams struct {
	CloudRegions      map[string][]float64          `json:"cloud_regions"`
	PossibleDecisions []string                      `json:"possible_decisions"`
	Functions         []string                      `json:"functions"`
	Classes           []string                      `json:"classes"`
	ArrivalRates      map[string]map[string]float64 `json:"arrival_rates"`

	ServTimeLocal   map[string]float64 `json:"serv_time_local"`
	InitTimeLocal   map[string]float64 `json:"init_time_local"`
	ColdStartPLocal map[string]float64 `json:"cold_start_p_local"`
	CPUUsageLocal   map[string]float64 `json:"cpu_usage_local"`

	ServTimeCloud    map[string]map[string]float64 `json:"serv_time_cloud"`
	InitTimeCloud    map[string]map[string]float64 `json:"init_time_cloud"`
	ColdStartPCloud  map[string]map[string]float64 `json:"cold_start_p_cloud"`
	CPUUsageCloud    map[string]map[string]float64 `json:"cpu_usage_cloud"`
	OffloadTimeCloud map[string]float64            `json:"offload_time_cloud"`
	BandwidthCloud   map[string]float64            `json:"bandwidth_cloud"`

	AggregatedEdgeMemory float64            `json:"aggregated_edge_memory"`
	ServTimeEdge         map[string]float64 `json:"serv_time_edge"`
	ColdStartPEdge       map[string]float64 `json:"cold_start_p_edge"`
	InitTimeEdge         map[string]float64 `json:"init_time_edge"`
	CPUUsageEdge         map[string]float64 `json:"cpu_usage_edge"`
	BandwidthEdge        float64            `json:"bandwidth_edge"`
	OffloadTimeEdge      float64            `json:"offload_time_edge"`

	AggregatedEdgePowerConsumption float64 `json:"aggregated_edge_power_consumption"`
	AggregatedEdgeEnergyTx         float64 `json:"aggregated_edge_energy_tx"`
	AggregatedEdgeEnergyRx         float64 `json:"aggregated_edge_energy_rx"`

	CO2GreenThreshold float64 `json:"co2_green_threshold"`
	Alpha             float64 `json:"alpha"`
	Beta              float64 `json:"beta"`

	NodeMemory                     float64 `json:"node_memory"`
	NodeCO2Footprint               float64 `json:"node_co2_footprint"`
	NodeProcessingPowerConsumption float64 `json:"node_processing_power_consumption"`
	NodeTxEnergyConsumption        float64 `json:"node_tx_energy_consumption"`
	NodeRxEnergyConsumption        float64 `json:"node_rx_energy_consumption"`
	Budget                         float64 `json:"budget"`

	FunctionMemory         map[string]int64   `json:"function_memory"`
	FunctionInputSizeMean  map[string]float64 `json:"function_input_size_mean"`
	FunctionOutputSizeMean map[string]float64 `json:"function_output_size_mean"`

	ClassMaxRt           map[string]float64 `json:"class_maxRt"`
	ClassUtility         map[string]float64 `json:"class_utility"`
	ClassDeadlinePenalty map[string]float64 `json:"class_deadline_penalty"`
	ClassDropPenalty     map[string]float64 `json:"class_drop_penalty"`
}

func initOptCarbonAwareParams() OptCarbonAwareParams {
	return OptCarbonAwareParams{
		CloudRegions:           make(map[string][]float64),
		ArrivalRates:           make(map[string]map[string]float64),
		ServTimeLocal:          make(map[string]float64),
		InitTimeLocal:          make(map[string]float64),
		ColdStartPLocal:        make(map[string]float64),
		CPUUsageLocal:          make(map[string]float64),
		ServTimeCloud:          make(map[string]map[string]float64),
		InitTimeCloud:          make(map[string]map[string]float64),
		ColdStartPCloud:        make(map[string]map[string]float64),
		CPUUsageCloud:          make(map[string]map[string]float64),
		OffloadTimeCloud:       make(map[string]float64),
		BandwidthCloud:         make(map[string]float64),
		ServTimeEdge:           make(map[string]float64),
		ColdStartPEdge:         make(map[string]float64),
		InitTimeEdge:           make(map[string]float64),
		CPUUsageEdge:           make(map[string]float64),
		FunctionMemory:         make(map[string]int64),
		FunctionInputSizeMean:  make(map[string]float64),
		FunctionOutputSizeMean: make(map[string]float64),
		ClassMaxRt:             make(map[string]float64),
		ClassUtility:           make(map[string]float64),
		ClassDeadlinePenalty:   make(map[string]float64),
		ClassDropPenalty:       make(map[string]float64),
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
		return Co2QosPolicyConfig{}, fmt.Errorf("read policy config: %w", err)
	}
	var c Co2QosPolicyConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Co2QosPolicyConfig{}, fmt.Errorf("parse policy config: %w", err)
	}
	if c.UpdateIntervalSec <= 0 {
		c.UpdateIntervalSec = 15
	}
	if c.ArrivalRate <= 0 || c.ArrivalRate > 1 {
		c.ArrivalRate = 0.3
	}
	if c.OptHost == "" {
		c.OptHost = "127.0.0.1"
	}
	if c.OptPort == 0 {
		c.OptPort = 8080
	}
	return c, nil
}

func (policy *Co2QosAwarePolicy) prepareOptimizerParams() (OptCarbonAwareParams, error) {
	params := initOptCarbonAwareParams()
	params.NodeMemory = float64(node.LocalResources.AvailableMemory())
	params.NodeCO2Footprint = node.LocalResources.CO2Intensity()
	params.NodeProcessingPowerConsumption = node.LocalResources.ProcessingPower()
	params.NodeTxEnergyConsumption = node.LocalResources.TxEnergyPerByte()
	params.NodeRxEnergyConsumption = node.LocalResources.RxEnergyPerByte()
	params.Budget = policy.Config.Budget
	params.Alpha = policy.AlphaWeight
	params.Beta = policy.BetaWeight
	params.CO2GreenThreshold = policy.Config.CO2GreenThreshold

	policy.populateEdgeParams(&params)

	functionNames, err := policy.listFunctions()
	if err != nil {
		return OptCarbonAwareParams{}, fmt.Errorf("list functions: %w", err)
	}
	retrievedMetrics := policy.getMetrics()
	allClasses := getQosClasses()
	candidates, stats := policy.discoverCloudCandidates(nil)
	params.PossibleDecisions, params.CloudRegions = regions.BuildCloudRegionsAndDecisions(candidates, stats)
	for area := range candidates {
		params.BandwidthCloud[area] = defaultBandwidthCloud
		if lb, ok := policy.firstLoadBalancer(area, nil); ok {
			if lat, err := policy.getCloudLatencySec(lb); err == nil && lat > 0 {
				params.OffloadTimeCloud[area] = lat
			}
		}
	}

	for _, functionName := range functionNames {
		realFunc, ok := policy.getFunction(functionName)
		if !ok || realFunc == nil {
			continue
		}
		params.Functions = append(params.Functions, functionName)
		params.FunctionMemory[functionName] = realFunc.MemoryMB
		params.FunctionInputSizeMean[functionName] = positiveOrDefault(retrievedMetrics.AvgInputSize[functionName], defaultInputSize)
		params.FunctionOutputSizeMean[functionName] = positiveOrDefault(retrievedMetrics.AvgOutputSize[functionName], defaultOutputSize)
		policy.populateFunctionTiming(&params, retrievedMetrics, realFunc)
	}

	for _, class := range allClasses {
		params.Classes = append(params.Classes, class.Name)
		params.ClassMaxRt[class.Name] = class.MaxRespTime
		params.ClassUtility[class.Name] = class.Utility
		params.ClassDeadlinePenalty[class.Name] = class.DeadlinePenalty
		params.ClassDropPenalty[class.Name] = class.DropPenalty
	}

	policy.arrivalRatesMutex.RLock()
	flat := make(map[string]float64, len(policy.arrivalRates))
	for k, v := range policy.arrivalRates {
		flat[k] = v
	}
	policy.arrivalRatesMutex.RUnlock()
	params.ArrivalRates = BuildNestedArrivalRates(flat, allClasses)

	return params, nil
}

func (policy *Co2QosAwarePolicy) populateEdgeParams(params *OptCarbonAwareParams) {
	nearbyServers := policy.getNeighborInfo()
	if len(nearbyServers) == 0 {
		return
	}
	var sumDist, sumPower, sumTx, sumRx float64
	var countEdge int
	for _, s := range nearbyServers {
		if s == nil {
			continue
		}
		availMem := s.AvailableMemory
		availCPU := s.TotalCPU - s.UsedCPU
		if availCPU > 0 && availMem > 0 {
			params.AggregatedEdgeMemory += float64(availMem)
			if registration.VivaldiClient != nil {
				sumDist += registration.VivaldiClient.DistanceTo(&s.Coordinates).Seconds()
			}
			sumPower += s.ProcessingPowerConsumption
			sumTx += s.TxEnergyConsumption
			sumRx += s.RxEnergyConsumption
			countEdge++
		}
	}
	if countEdge > 0 {
		params.OffloadTimeEdge = sumDist / float64(countEdge)
		params.AggregatedEdgePowerConsumption = sumPower / float64(countEdge)
		params.AggregatedEdgeEnergyTx = sumTx / float64(countEdge)
		params.AggregatedEdgeEnergyRx = sumRx / float64(countEdge)
	}
	params.BandwidthEdge = defaultBandwidthEdge
}

func (policy *Co2QosAwarePolicy) populateFunctionTiming(params *OptCarbonAwareParams, retrieved metrics.RetrievedMetrics, fun *function.Function) {
	functionName := fun.Name
	localNodeName := localNodeMetricName()
	params.ServTimeLocal[functionName] = metricByNode(retrieved.AvgEdgeExecutionTime, localNodeName, functionName, defaultServiceTimeSec)
	params.InitTimeLocal[functionName] = metricByNode(retrieved.AvgEdgeInitTime, localNodeName, functionName, defaultInitTimeSec)
	params.ColdStartPLocal[functionName] = metricByNode(retrieved.EdgeColdStartProbability, localNodeName, functionName, 1.0)
	params.CPUUsageLocal[functionName] = metricByNode(retrieved.AvgEdgeCPUUsage, localNodeName, functionName, defaultCPUUsage)

	params.ServTimeEdge[functionName] = avgNestedMetricExcluding(retrieved.AvgEdgeExecutionTime, localNodeName, functionName, defaultServiceTimeSec)
	params.InitTimeEdge[functionName] = avgNestedMetricExcluding(retrieved.AvgEdgeInitTime, localNodeName, functionName, defaultInitTimeSec)
	params.ColdStartPEdge[functionName] = avgNestedMetricExcluding(retrieved.EdgeColdStartProbability, localNodeName, functionName, 1.0)
	params.CPUUsageEdge[functionName] = avgNestedMetricExcluding(retrieved.AvgEdgeCPUUsage, localNodeName, functionName, defaultCPUUsage)

	for area := range policy.cloudCandidateAreas {
		ensureCloudTimingMaps(params, area)
		params.ServTimeCloud[area][functionName] = metricByNode(retrieved.AvgCloudRegionExecutionTime, area, functionName, defaultServiceTimeSec)
		params.InitTimeCloud[area][functionName] = metricByNode(retrieved.AvgCloudRegionInitTime, area, functionName, defaultInitTimeSec)
		params.ColdStartPCloud[area][functionName] = metricByNode(retrieved.CloudRegionColdStartProbability, area, functionName, 1.0)
		params.CPUUsageCloud[area][functionName] = metricByNode(retrieved.AvgCloudRegionCPUUsage, area, functionName, defaultCPUUsage)
	}
}

func ensureCloudTimingMaps(params *OptCarbonAwareParams, area string) {
	if _, ok := params.ServTimeCloud[area]; !ok {
		params.ServTimeCloud[area] = make(map[string]float64)
	}
	if _, ok := params.InitTimeCloud[area]; !ok {
		params.InitTimeCloud[area] = make(map[string]float64)
	}
	if _, ok := params.ColdStartPCloud[area]; !ok {
		params.ColdStartPCloud[area] = make(map[string]float64)
	}
	if _, ok := params.CPUUsageCloud[area]; !ok {
		params.CPUUsageCloud[area] = make(map[string]float64)
	}
}

func localNodeMetricName() string {
	if registration.SelfRegistration != nil {
		return registration.SelfRegistration.NodeID.String()
	}
	return node.LocalNode.String()
}

func metricByNode(values map[string]map[string]float64, nodeName, functionName string, fallback float64) float64 {
	if byFunction, ok := values[nodeName]; ok {
		return positiveOrDefault(byFunction[functionName], fallback)
	}
	return fallback
}

func avgNestedMetricExcluding(values map[string]map[string]float64, excludedNode, functionName string, fallback float64) float64 {
	var sum float64
	var count int
	for nodeName, byFunction := range values {
		if nodeName == excludedNode {
			continue
		}
		v, ok := byFunction[functionName]
		if ok && validPositive(v) {
			sum += v
			count++
		}
	}
	if count == 0 {
		return fallback
	}
	return sum / float64(count)
}

func positiveOrDefault(v, fallback float64) float64 {
	if validPositive(v) {
		return v
	}
	return fallback
}

func validPositive(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (policy *Co2QosAwarePolicy) optimizerLoop() {
	ticker := time.NewTicker(policy.updateInterval)
	defer ticker.Stop()

	for range ticker.C {
		policy.calculateArrivalRates()
		params, err := policy.prepareOptimizerParams()
		if err != nil {
			log.Printf("CO2/QoS optimizer params error: %v", err)
			continue
		}
		if err := policy.invokeOptimizer(params); err != nil {
			log.Printf("CO2/QoS optimizer call error: %v", err)
		}
	}
}

func (policy *Co2QosAwarePolicy) invokeOptimizer(params OptCarbonAwareParams) error {
	jsonData, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal optimizer params: %w", err)
	}
	url := fmt.Sprintf("http://%s:%d/", policy.Config.OptHost, policy.Config.OptPort)
	resp, err := policy.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("optimizer status %s: %s", resp.Status, string(body))
	}
	parsed, err := parseOptimizerResponse(body)
	if err != nil {
		return err
	}
	for k, v := range parsed {
		policy.probabilityCache.Store(k, v)
	}
	return nil
}

func parseOptimizerResponse(body []byte) (map[string]MultiRegionProbs, error) {
	var wrapped struct {
		Probs    map[string]map[string]map[string]float64 `json:"probs"`
		VarProbs map[string]map[string]map[string]float64 `json:"decision_fc_probability_var"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Probs != nil {
		if len(wrapped.VarProbs) > 0 {
			return nil, fmt.Errorf("optimizer returned variant probabilities during phase 1")
		}
		return parseOptimizerProbabilityMap(wrapped.Probs)
	}

	var nested map[string]map[string]map[string]float64
	if err := json.Unmarshal(body, &nested); err != nil {
		return nil, fmt.Errorf("unmarshal optimizer response: %w", err)
	}
	return parseOptimizerProbabilityMap(nested)
}

func parseOptimizerProbabilityMap(nested map[string]map[string]map[string]float64) (map[string]MultiRegionProbs, error) {
	out := make(map[string]MultiRegionProbs, len(nested))
	for fn, classes := range nested {
		for class, decisions := range classes {
			probs := MultiRegionProbs{PCloud: make(map[string]float64)}
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
					if region == "" {
						return nil, fmt.Errorf("empty cloud region in optimizer decision %q", dec)
					}
					probs.PCloud[region] = v
				case dec == "EXEC_VAR":
					return nil, fmt.Errorf("unsupported optimizer decision in phase 1: %s", dec)
				default:
					return nil, fmt.Errorf("unknown optimizer decision: %s", dec)
				}
			}
			out[fn+"|"+class] = probs
		}
	}
	return out, nil
}

func BuildNestedArrivalRates(flat map[string]float64, classes []QoSClass) map[string]map[string]float64 {
	out := make(map[string]map[string]float64)
	if len(classes) == 0 {
		return out
	}
	defaultClass := classes[0].Name
	for k, v := range flat {
		fn, cls, ok := strings.Cut(strings.TrimSpace(k), "|")
		if !ok {
			fn = strings.TrimSpace(k)
			cls = defaultClass
		}
		fn = strings.TrimSpace(fn)
		cls = strings.TrimSpace(cls)
		if cls == "" {
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

func getClassNameByID(id int64) (string, bool) {
	class, found := qosClasses[id]
	return class.Name, found
}

func loadQosClasses(filePath string) error {
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read QoS classes: %w", err)
	}
	var configData struct {
		Classes []QoSClass `yaml:"classes"`
	}
	if err := yaml.Unmarshal(yamlFile, &configData); err != nil {
		return fmt.Errorf("parse QoS classes: %w", err)
	}
	qosClasses = make(map[int64]QoSClass, len(configData.Classes))
	for _, classDef := range configData.Classes {
		qosClasses[classDef.Id] = classDef
	}
	return nil
}
