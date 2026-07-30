package scheduling

import (
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/container"
	"github.com/serverledge-faas/serverledge/internal/function"
	"github.com/serverledge-faas/serverledge/internal/metrics"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/regions"
	"github.com/serverledge-faas/serverledge/internal/registration"
)

const edgeDecisionTarget = "__edge__"

type Co2QosAwarePolicy struct {
	probabilityCache sync.Map
	updateInterval   time.Duration
	httpClient       *http.Client

	arrivalCounts      map[string]int64
	arrivalCountsMutex sync.Mutex
	arrivalRates       map[string]float64
	arrivalRatesMutex  sync.RWMutex
	arrivalAlpha       float64
	AlphaWeight        float64
	BetaWeight         float64
	Config             Co2QosPolicyConfig

	cloudCandidateAreas map[string]regions.AreaInfo

	listFunctions       func() ([]string, error)
	getFunction         func(string) (*function.Function, bool)
	getMetrics          func() metrics.RetrievedMetrics
	getNeighborInfo     func() map[string]*registration.StatusInformation
	listConfiguredAreas func() ([]regions.AreaInfo, error)
	getLBInArea         func(string) (map[string]registration.NodeRegistration, error)
	getNodeStatus       func(registration.NodeRegistration) (*registration.StatusInformation, error)
	getCloudLatencySec  func(registration.NodeRegistration) (float64, error)
	acquireContainer    func(*function.Function, bool) (*container.Container, bool, error)
	edgeTarget          func(*scheduledRequest) (string, error)
	execLocal           func(*scheduledRequest, *container.Container, bool)
	offload             func(*scheduledRequest, string)
	drop                func(*scheduledRequest)
	randFloat64         func() float64
}

func (policy *Co2QosAwarePolicy) Init() {
	policy.setDefaultHooks()
	policyConfigPath := config.GetString(config.CO2_QOS_POLICY_GENERAL_CONFIG_PATH, "")
	cfg, err := LoadPolicyConfig(policyConfigPath)
	if err != nil {
		log.Printf("could not load CO2/QoS policy config: %v", err)
		return
	}
	policy.Config = cfg
	policy.arrivalAlpha = cfg.ArrivalRate
	policy.updateInterval = time.Duration(cfg.UpdateIntervalSec) * time.Second
	policy.httpClient = &http.Client{Timeout: 10 * time.Second}
	policy.AlphaWeight = cfg.Alpha
	policy.BetaWeight = cfg.Beta
	policy.arrivalCounts = make(map[string]int64)
	policy.arrivalRates = make(map[string]float64)
	policy.cloudCandidateAreas = make(map[string]regions.AreaInfo)

	qosPath := config.GetString(config.FUNCTION_OFFLOADING_QOS_CLASSES_PATH, "")
	if err := loadQosClasses(qosPath); err != nil {
		log.Printf("could not load QoS classes: %v", err)
		return
	}

	go policy.optimizerLoop()
}

func (policy *Co2QosAwarePolicy) setDefaultHooks() {
	if policy.listFunctions == nil {
		policy.listFunctions = function.GetAll
	}
	if policy.getFunction == nil {
		policy.getFunction = function.GetFunction
	}
	if policy.getMetrics == nil {
		policy.getMetrics = metrics.GetMetrics
	}
	if policy.getNeighborInfo == nil {
		policy.getNeighborInfo = registration.GetFullNeighborInfo
	}
	if policy.listConfiguredAreas == nil {
		policy.listConfiguredAreas = configuredAreasFromRegionsFile
	}
	if policy.getLBInArea == nil {
		policy.getLBInArea = registration.GetLBInArea
	}
	if policy.getNodeStatus == nil {
		policy.getNodeStatus = fetchNodeStatus
	}
	if policy.getCloudLatencySec == nil {
		policy.getCloudLatencySec = func(lb registration.NodeRegistration) (float64, error) {
			return tcpLatencySec(lb.IPAddress, lb.APIPort)
		}
	}
	if policy.acquireContainer == nil {
		policy.acquireContainer = node.AcquireContainer
	}
	if policy.edgeTarget == nil {
		policy.edgeTarget = pickEdgeNodeForOffloading
	}
	if policy.execLocal == nil {
		policy.execLocal = execLocally
	}
	if policy.offload == nil {
		policy.offload = handleOffload
	}
	if policy.drop == nil {
		policy.drop = dropRequest
	}
	if policy.randFloat64 == nil {
		policy.randFloat64 = rand.Float64
	}
	if policy.arrivalCounts == nil {
		policy.arrivalCounts = make(map[string]int64)
	}
	if policy.arrivalRates == nil {
		policy.arrivalRates = make(map[string]float64)
	}
	if policy.httpClient == nil {
		policy.httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if policy.updateInterval <= 0 {
		policy.updateInterval = 15 * time.Second
	}
	if policy.cloudCandidateAreas == nil {
		policy.cloudCandidateAreas = make(map[string]regions.AreaInfo)
	}
}

func configuredAreasFromRegionsFile() ([]regions.AreaInfo, error) {
	path := config.GetString(config.REGIONS_FILE_PATH, "")
	if path == "" {
		return nil, nil
	}
	cfg, err := regions.Load(path)
	if err != nil {
		return nil, err
	}
	return cfg.Areas(), nil
}

func tcpLatencySec(host string, port int) (float64, error) {
	start := time.Now()
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get((&registration.NodeRegistration{IPAddress: host, APIPort: port}).APIUrl() + "/status")
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return time.Since(start).Seconds(), nil
}

func fetchNodeStatus(lb registration.NodeRegistration) (*registration.StatusInformation, error) {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(lb.APIUrl() + "/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	var status registration.StatusInformation
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (policy *Co2QosAwarePolicy) OnArrival(r *scheduledRequest) {
	policy.setDefaultHooks()
	qosName, ok := getClassNameByID(r.Class)
	if !ok {
		qosName = "default"
	}
	cacheKey := r.Fun.Name + "|" + qosName

	policy.arrivalCountsMutex.Lock()
	policy.arrivalCounts[cacheKey]++
	policy.arrivalCountsMutex.Unlock()

	decision, err := policy.evaluate(r, cacheKey)
	if err != nil {
		log.Printf("CO2/QoS evaluation failed: %v", err)
		policy.drop(r)
		return
	}
	policy.applyDecision(r, decision)
}

func (policy *Co2QosAwarePolicy) OnCompletion(_ *function.Function, _ *function.ExecutionReport) {
}

func (policy *Co2QosAwarePolicy) evaluate(r *scheduledRequest, cacheKey string) (schedDecision, error) {
	policy.setDefaultHooks()
	val, ok := policy.probabilityCache.Load(cacheKey)
	var probs MultiRegionProbs
	if ok {
		var typeOK bool
		probs, typeOK = val.(MultiRegionProbs)
		if !typeOK {
			return schedDecision{}, errors.New("invalid probability cache entry")
		}
	} else {
		probs = policy.defaultProbabilities(r.Fun)
	}
	if !r.CanDoOffloading {
		probs.PEdge = 0
		for region := range probs.PCloud {
			probs.PCloud[region] = 0
		}
	}
	if !r.Fun.SupportsArch(node.LocalNode.Arch) || !canExecuteLocally(r.Fun) {
		probs.PLocal = 0
	}
	return policy.randomizedChoice(probs)
}

func (policy *Co2QosAwarePolicy) defaultProbabilities(fun *function.Function) MultiRegionProbs {
	candidates, _ := policy.discoverCloudCandidates(fun)
	probs := MultiRegionProbs{
		PLocal: 0.25,
		PEdge:  0.25,
		PDrop:  0.25,
		PCloud: make(map[string]float64, len(candidates)),
	}
	if len(candidates) > 0 {
		per := 0.25 / float64(len(candidates))
		for area := range candidates {
			probs.PCloud[area] = per
		}
	}
	return probs
}

func (policy *Co2QosAwarePolicy) randomizedChoice(probs MultiRegionProbs) (schedDecision, error) {
	regionNames := make([]string, 0, len(probs.PCloud))
	for region := range probs.PCloud {
		regionNames = append(regionNames, region)
	}
	sort.Strings(regionNames)

	sum := probs.PLocal + probs.PEdge + probs.PDrop
	for _, region := range regionNames {
		sum += probs.PCloud[region]
	}
	if sum <= 0 {
		return schedDecision{action: DROP}, nil
	}

	randomValue := policy.randFloat64()
	cumulative := probs.PLocal / sum
	if randomValue < cumulative {
		return schedDecision{action: EXEC_LOCAL}, nil
	}

	for _, region := range regionNames {
		cumulative += probs.PCloud[region] / sum
		if randomValue < cumulative {
			return schedDecision{action: EXEC_REMOTE, remoteHost: policy.regionToRemoteHost(region)}, nil
		}
	}

	cumulative += probs.PEdge / sum
	if randomValue < cumulative {
		return schedDecision{action: EXEC_REMOTE, remoteHost: edgeDecisionTarget}, nil
	}
	return schedDecision{action: DROP}, nil
}

func canExecuteLocally(fun *function.Function) bool {
	node.LocalResources.RLock()
	defer node.LocalResources.RUnlock()
	return node.LocalResources.AvailableCPUs() >= fun.CPUDemand &&
		node.LocalResources.AvailableMemory() >= fun.MemoryMB
}

func (policy *Co2QosAwarePolicy) applyDecision(r *scheduledRequest, decision schedDecision) {
	switch decision.action {
	case DROP:
		policy.drop(r)
	case EXEC_LOCAL:
		cont, warm, err := policy.acquireContainer(r.Fun, false)
		if err != nil {
			log.Printf("CO2/QoS local acquire failed: %v", err)
			policy.drop(r)
			return
		}
		policy.execLocal(r, cont, warm)
	case EXEC_REMOTE:
		if decision.remoteHost == edgeDecisionTarget {
			target, err := policy.edgeTarget(r)
			if err != nil {
				log.Printf("CO2/QoS edge target failed: %v", err)
				policy.drop(r)
				return
			}
			policy.offload(r, target)
			return
		}
		if decision.remoteHost == "" {
			policy.drop(r)
			return
		}
		policy.offload(r, decision.remoteHost)
	default:
		policy.drop(r)
	}
}

func (policy *Co2QosAwarePolicy) discoverCloudCandidates(fun *function.Function) (map[string]regions.AreaInfo, map[string]regions.AreaStat) {
	policy.setDefaultHooks()
	areas, err := policy.listConfiguredAreas()
	if err != nil {
		log.Printf("could not list configured areas: %v", err)
		return nil, nil
	}
	candidates := make(map[string]regions.AreaInfo)
	stats := make(map[string]regions.AreaStat)
	for _, area := range areas {
		lb, ok := policy.firstLoadBalancer(area.Name, fun)
		if !ok {
			continue
		}
		candidates[area.Name] = area
		stats[area.Name] = regions.AreaStat{
			MemoryAvailable:            float64(node.LocalResources.AvailableMemory()),
			CO2Intensity:               node.LocalResources.CO2Intensity(),
			ProcessingPowerConsumption: area.ProcessingPowerConsumption,
			TxEnergyConsumption:        area.TxEnergyConsumption,
			RxEnergyConsumption:        area.RxEnergyConsumption,
		}
		if status, err := policy.getNodeStatus(lb); err == nil && status != nil {
			stats[area.Name] = regions.AreaStat{
				MemoryAvailable:            float64(status.AvailableMemory),
				CO2Intensity:               status.CO2Intensity,
				ProcessingPowerConsumption: status.ProcessingPowerConsumption,
				TxEnergyConsumption:        status.TxEnergyConsumption,
				RxEnergyConsumption:        status.RxEnergyConsumption,
			}
		}
		if lat, err := policy.getCloudLatencySec(lb); err == nil && lat > 0 {
			policy.ensureCloudCandidateMaps()
			policy.cloudCandidateAreas[area.Name] = area
		}
	}
	policy.cloudCandidateAreas = candidates
	return candidates, stats
}

func (policy *Co2QosAwarePolicy) firstLoadBalancer(area string, fun *function.Function) (registration.NodeRegistration, bool) {
	lbs, err := policy.getLBInArea(area)
	if err != nil || len(lbs) == 0 {
		return registration.NodeRegistration{}, false
	}
	keys := make([]string, 0, len(lbs))
	for key := range lbs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lb := lbs[key]
		if fun != nil && !fun.SupportsArch(lb.Arch) {
			continue
		}
		return lb, true
	}
	return registration.NodeRegistration{}, false
}

func (policy *Co2QosAwarePolicy) ensureCloudCandidateMaps() {
	if policy.cloudCandidateAreas == nil {
		policy.cloudCandidateAreas = make(map[string]regions.AreaInfo)
	}
}

func (policy *Co2QosAwarePolicy) regionToRemoteHost(region string) string {
	lb, ok := policy.firstLoadBalancer(region, nil)
	if ok {
		return lb.APIUrl()
	}
	return ""
}

func (policy *Co2QosAwarePolicy) calculateArrivalRates() {
	policy.arrivalCountsMutex.Lock()
	countsSnapshot := policy.arrivalCounts
	policy.arrivalCounts = make(map[string]int64)
	policy.arrivalCountsMutex.Unlock()

	elapsedSeconds := policy.updateInterval.Seconds()
	if elapsedSeconds <= 0 {
		return
	}

	policy.arrivalRatesMutex.Lock()
	defer policy.arrivalRatesMutex.Unlock()
	for key, count := range countsSnapshot {
		measuredRate := float64(count) / elapsedSeconds
		oldRate := policy.arrivalRates[key]
		policy.arrivalRates[key] = policy.arrivalAlpha*measuredRate + (1.0-policy.arrivalAlpha)*oldRate
	}
}
