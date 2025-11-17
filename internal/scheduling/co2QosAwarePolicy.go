package scheduling

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/emissions"
	"github.com/serverledge-faas/serverledge/internal/function"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/regions"
	"github.com/serverledge-faas/serverledge/internal/registration"
)

type Co2QosAwarePolicy struct {
	probabilityCache sync.Map
	updateInterval   time.Duration
	httpClient       *http.Client

	arrivalCounts      map[string]int64            // Contatore thread-safe per le richieste in arrivo
	arrivalCountsMutex sync.Mutex                  // Mutex per proteggere arrivalCounts
	arrivalRates       map[string]float64          // Mappa dei tassi di arrivo "smussati" (req/sec)
	arrivalRatesMutex  sync.RWMutex                // RWMutex per proteggere arrivalRates
	arrivalAlpha       float64                     // Fattore di smoothing per la Media Mobile Esponenziale (EMA)
	cloudRegions       map[string]regions.AreaInfo //key: areaName, val: AreaInfo
	AlphaWeight        float64                     //weight qos
	BetaWeight         float64                     //weight co2
	Config             Co2QosPolicyConfig
}

func (policy *Co2QosAwarePolicy) Init() {
	// Policy general configuration
	policyConfigPath := config.GetString(config.CO2_QOS_POLICY_GENERAL_CONFIG_PATH, "")
	cfg, err := LoadPolicyConfig(policyConfigPath)
	if err != nil {
		log.Printf("Impossible to load co2&qos general config: %v", err)
		return
	}
	policy.Config = cfg
	policy.arrivalAlpha = cfg.ArrivalRate
	policy.updateInterval = time.Duration(cfg.UpdateIntervalSec) * time.Second
	policy.httpClient = &http.Client{Timeout: 10 * time.Second}
	policy.AlphaWeight = cfg.Alpha
	policy.BetaWeight = cfg.Beta

	// Load QoS class definitions
	qosPath := config.GetString(config.FUNCTION_OFFLOADING_QOS_CLASSES_PATH, "")
	err = loadQosClasses(qosPath)
	if err != nil {
		log.Printf("Impossible to load QoS classes definitions: %v", err)
		return
	}

	policy.arrivalCounts = make(map[string]int64)
	policy.arrivalRates = make(map[string]float64)

	log.Println("Starting policy polling process...")
	go policy.optimizerLoop()
}

func (policy *Co2QosAwarePolicy) OnArrival(r *scheduledRequest) {
	allAreas, _ := registration.ListAreas()
	fmt.Print("=== regions: ", allAreas)

	qosName, ok := getClassNameByID(r.Class)
	if !ok {
		log.Printf("QoS class name not registered, plese add it, error: %v", r.Class)
	}
	key := r.Fun.Name + "|" + qosName
	policy.arrivalCountsMutex.Lock()
	policy.arrivalCounts[key]++
	policy.arrivalCountsMutex.Unlock()

	decision, err := policy.evaluate(r, key)
	if err != nil {
		log.Printf("Error calling Evaluate request: %v. Dropping it...", err)
		dropRequest(r)
	}
	log.Printf("decision: ", decision)

	var actionChoice string
	if decision.action == 0 {
		actionChoice = "Drop"
	} else if decision.action == 1 {
		actionChoice = "Execute locally"
	} else if decision.action == 2 {
		actionChoice = "Execute remotely on " + decision.remoteHost
	}

	log.Printf("Action choiced by evaluator: %s\n", actionChoice)

	if decision.action == 0 {
		dropRequest(r)
	} else if decision.action == 1 { //Local execution
		containerID, warm, err := node.AcquireContainer(r.Fun, false)
		if err == nil {
			execLocally(r, containerID, warm)
		} else {
			log.Printf("Error in choosing container: %v", err)
		}
	} else if decision.action == 2 && decision.remoteHost == edgeUrl { //Offload on Edge node
		// We want to choose the node with more memory available
		nearbyServers := registration.GetFullNeighborInfo()
		edgeNodes := make([]string, 0)
		nodeMemory := make(map[string]float64)
		for k, v := range nearbyServers {
			availableCPU := v.TotalCPU - v.UsedCPU
			availableMemory := v.TotalMemory - v.UsedMemory
			if availableCPU > 0 && availableMemory > r.Fun.MemoryMB {
				edgeNodes = append(edgeNodes, k)
				nodeMemory[k] = float64(availableMemory)
			}
		}
		selectedEdge, err := selectEdgePeer(edgeNodes, nodeMemory)
		if err != nil || selectedEdge == "" { //Here we can send to Cloud or Drop, for now we drop
			log.Printf("No edge peer available, dropping request...")
			dropRequest(r)
			return
		}
		chosenPeerInfo := registration.GetPeerFromKey(selectedEdge)
		targetURL := chosenPeerInfo.APIUrl()
		log.Printf("In OnArrival - offloading to EDGE - target node: %s\n", targetURL)

		handleOffload(r, targetURL)

	} else if decision.action == 2 && decision.remoteHost != edgeUrl { //cloud region
		if decision.remoteHost == "" {
			log.Printf("No LB configured for cloud region in ", decision.regionName, ", dropping request...")
			dropRequest(r)
			return
		}
		log.Printf("In OnArrival - offloading to CLOUD REGION: %s - target node: %s\n", decision.regionName, decision.remoteHost)

		handleOffload(r, decision.remoteHost)
	}

	log.Printf("Execution of function: %s  with action: %s done\n.", r.Fun.Name, actionChoice)

}

func (policy *Co2QosAwarePolicy) OnCompletion(fun *function.Function, executionReport *function.ExecutionReport) {
	//todo: co2 metric update
}

func (policy *Co2QosAwarePolicy) evaluate(r *scheduledRequest, cacheKey string) (schedDecision, error) {
	if !r.CanDoOffloading {
		if node.CanExecuteLocally(r.Fun.CPUDemand, r.Fun.MemoryMB) {
			return schedDecision{action: EXEC_LOCAL}, nil
		}
		return schedDecision{action: DROP}, nil
	}

	val, ok := policy.probabilityCache.Load(cacheKey)
	log.Printf("In EVALUATE - loaded probs (val): %s\n", val)

	var probs MultiRegionProbs
	if ok {
		probs = val.(MultiRegionProbs)
		log.Printf("probs[%s]: PLocal=%.4f PEdge=%.4f PDrop=%.4f PCloud=%v",
			cacheKey, probs.PLocal, probs.PEdge, probs.PDrop, probs.PCloud)
	} else {
		probs = MultiRegionProbs{
			PLocal: 0.25,
			PEdge:  0.25,
			PDrop:  0.25,
			PCloud: make(map[string]float64),
		}
		regCount := len(registration.CloudRegions)
		if regCount > 0 {
			per := 0.25 / float64(regCount)
			for region := range registration.CloudRegions {
				probs.PCloud[region] = per
			}
		}
	}

	// If local cannot run it, zero local probability
	if !node.CanExecuteLocally(r.Fun.CPUDemand, r.Fun.MemoryMB) {
		probs.PLocal = 0
	}
	return randomizedChoiceV2(probs)
}

func randomizedChoiceV2(probs MultiRegionProbs) (schedDecision, error) {
	regionNames := make([]string, 0, len(registration.CloudRegions))
	for region := range registration.CloudRegions {
		regionNames = append(regionNames, region)
	}
	sort.Strings(regionNames)

	// Sum probabilities
	sum := probs.PLocal + probs.PEdge + probs.PDrop
	for _, region := range regionNames {
		sum += probs.PCloud[region]
	}
	if sum <= 0 {
		return schedDecision{action: DROP}, nil
	}

	// Local exec
	randomValue := rand.Float64()
	cumulative := probs.PLocal / sum
	if randomValue < cumulative {
		return schedDecision{action: EXEC_LOCAL}, nil
	}

	// Cloud regions
	for _, region := range regionNames {
		p := probs.PCloud[region] / sum
		cumulative += p
		if randomValue < cumulative {
			remoteLBUrl := regionToRemoteHost(region)
			log.Printf("In randomizedChoice - cloud offloading to %s\n", region)

			return schedDecision{action: EXEC_REMOTE, remoteHost: remoteLBUrl, regionName: region}, nil
		}
	}

	// Edge
	cumulative += probs.PEdge / sum
	if randomValue < cumulative {
		return schedDecision{action: EXEC_REMOTE, remoteHost: edgeUrl}, nil
	}

	return schedDecision{action: DROP}, nil
}

// return LB url of the input region
func regionToRemoteHost(region string) string {
	if ai, ok := registration.CloudRegions[region]; ok && ai.LoadBalancerNode.Key != "" {
		log.Printf("ai.LoadBalancerNode.Key ", ai.LoadBalancerNode.Key)
		log.Printf("registration.GetPeerFromKey(ai.LoadBalancerNode.Key)", registration.GetPeerFromKey(ai.LoadBalancerNode.Key))

		lb, err := registration.GetLBByKey(region, ai.LoadBalancerNode.Key)
		if err != nil {
			log.Printf("LB lookup failed for area=%s key=%s: %v", region, ai.LoadBalancerNode.Key, err)
			return ""
		}
		log.Printf("===region to remote host: ", lb.APIUrl())

		return lb.APIUrl()
	}
	return ""
}

func prepareEnergyInputs(r *scheduledRequest) emissions.Inputs {
	return emissions.Inputs{
		DurationSec:             r.ExecutionReport.Duration,
		FunctionMemory:          float64(r.Fun.MemoryMB), //TODO: fixa unita misura
		CurrentNodePowerCons:    node.LocalResources.ProcessingPower(),
		CurrentNodeCO2Intensity: node.LocalResources.Co2Footprint.Intensity(),
		InputSizeMean:           100.0, //todo: da fixare
		OutputSizeMean:          10.0,

		InitialNodeTxEnergy:   r.initialNodeTxEnergy,
		InitialNodeRxEnergy:   r.initialNodeRxEnergy,
		AggrInitialNodeMemory: r.initialNodeMemory,

		// executor (this node)
		LocalNodeRxEnergy: node.LocalResources.RxEnergyPerByte(),
		LocalNodeTxEnergy: node.LocalResources.TxEnergyPerByte(),
	}
}
