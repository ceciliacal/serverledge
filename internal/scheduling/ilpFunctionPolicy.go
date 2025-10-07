package scheduling

import (
	"fmt"
	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/externalprovider/lambda"
	"github.com/serverledge-faas/serverledge/internal/function"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/registration"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

const cloudUrl = "external"
const edgeUrl = "edge"

type IlpOffloadingPolicy struct {
	probabilityCache sync.Map
	updateInterval   time.Duration
	httpClient       *http.Client

	arrivalCounts      map[string]int64   // Contatore thread-safe per le richieste in arrivo
	arrivalCountsMutex sync.Mutex         // Mutex per proteggere arrivalCounts
	arrivalRates       map[string]float64 // Mappa dei tassi di arrivo "smussati" (req/sec)
	arrivalRatesMutex  sync.RWMutex       // RWMutex per proteggere arrivalRates
	arrivalAlpha       float64            // Fattore di smoothing per la Media Mobile Esponenziale (EMA)
}

func (policy *IlpOffloadingPolicy) Init() {
	updateIntervalSeconds := config.GetInt(config.FUNCTION_OFFLOADING_POLICY_LAMBDA_PING_INTERVAL, 60)
	policy.updateInterval = time.Duration(updateIntervalSeconds) * time.Second
	policy.httpClient = &http.Client{Timeout: 10 * time.Second}

	//Taking Qos Classes
	qosPath := config.GetString(config.FUNCTION_OFFLOADING_QOS_CLASSES_PATH, "")

	err := loadQoSDefinitions(qosPath)
	if err != nil {
		log.Printf("Impossible to load QoS classes definitions: %v", err)
		return
	}

	alpha := config.GetFloat(config.POLICY_ARRIVAL_RATE_ALPHA, 0.3)
	policy.arrivalAlpha = alpha
	policy.arrivalCounts = make(map[string]int64)
	policy.arrivalRates = make(map[string]float64)

	//Lambda RTT Monitor
	fnPingFunction := config.GetString(config.POLICY_FUNCTION_NAME, "")
	lambda.InitRttMonitor(policy.updateInterval, fnPingFunction)

	log.Println("Starting policy polling process...")
	go policy.optimizerLoop()
}

func (policy *IlpOffloadingPolicy) OnArrival(r *scheduledRequest) {
	qosName, ok := getQoSClassNameByID(r.Class)
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

	var actionChoice string
	if decision.action == 0 {
		actionChoice = "Drop"
	} else if decision.action == 1 {
		actionChoice = "Execute locally"
	} else if decision.action == 2 {
		actionChoice = "Execute remotely on" + decision.remoteHost
	}

	log.Printf("Action choiced by evaluator: %s\n", actionChoice)

	if decision.action == 0 {
		dropRequest(r)
	} else if decision.action == 1 { //Local execution
		containerID, warm, err := node.AcquireContainer(r.Fun)
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
			if v.AvailableCPUs > 0 && v.AvailableMemMB > r.Fun.MemoryMB {
				edgeNodes = append(edgeNodes, k)
				nodeMemory[k] = float64(v.AvailableMemMB)
			}
		}
		selectedEdge, err := selectEdgePeer(edgeNodes, nodeMemory)
		if err != nil || selectedEdge == "" { //Here we can send to Lambda or Drop, for now we drop
			log.Printf("No edge peer available, dropping request...")
			dropRequest(r)
			return
		}
		chosenPeerInfo := registration.GetPeerFromKey(selectedEdge)
		targetURL := chosenPeerInfo.APIUrl()

		handleOffload(r, targetURL)

	} else if decision.action == 2 && decision.remoteHost == cloudUrl {
		handleLambdaOffload(r)
	}

	log.Printf("Execution of function: %s  with action: %s done\n.", r.Fun.Name, actionChoice)
}

func (policy *IlpOffloadingPolicy) OnCompletion(fun *function.Function, executionReport *function.ExecutionReport) {

}

func (policy *IlpOffloadingPolicy) evaluate(r *scheduledRequest, cacheKey string) (schedDecision, error) {

	if !r.CanDoOffloading {
		if node.CanExecuteLocally(r.Fun.CPUDemand, r.Fun.MemoryMB) {
			return schedDecision{action: EXEC_LOCAL}, nil
		} else {
			return schedDecision{action: DROP}, nil
		}
	}

	value, ok := policy.probabilityCache.Load(cacheKey)
	var currentProbs Probs
	if ok {
		currentProbs = value.(Probs)
	} else {
		currentProbs = Probs{
			PLocal: 0.25,
			PCloud: 0.25,
			PEdge:  0.25,
			PDrop:  0.25,
		}
	}

	if !node.CanExecuteLocally(r.Fun.CPUDemand, r.Fun.MemoryMB) {
		currentProbs.PLocal = 0
	}
	return randomizedChoice(currentProbs)
}

func randomizedChoice(probs Probs) (schedDecision, error) {
	if probs.PEdge+probs.PCloud+probs.PLocal <= 0 {
		return schedDecision{action: DROP}, nil
	}

	sum := probs.PLocal + probs.PCloud + probs.PEdge + probs.PDrop

	pLocal := probs.PLocal / sum
	pCloud := probs.PCloud / sum
	pEdge := probs.PEdge / sum

	randValue := rand.Float64()

	if randValue < pLocal {
		return schedDecision{action: EXEC_LOCAL}, nil
	} else if randValue < pLocal+pCloud {
		return schedDecision{action: EXEC_REMOTE, remoteHost: cloudUrl}, nil
	} else if randValue < pLocal+pCloud+pEdge {
		return schedDecision{action: EXEC_REMOTE, remoteHost: edgeUrl}, nil
	} else {
		return schedDecision{action: DROP}, nil
	}
}

func selectEdgePeer(edgeNodes []string, nodeMemory map[string]float64) (string, error) {
	candidates := make(map[string]float64)
	var totalWeight float64 = 0

	for _, nodeID := range edgeNodes {

		if mem, ok := nodeMemory[nodeID]; ok && mem > 0 {
			candidates[nodeID] = mem
			totalWeight += mem
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no Edge node available")
	}

	randValue := rand.Float64() * totalWeight
	for nodeID, memory := range candidates {
		randValue -= memory
		if randValue <= 0 {
			return nodeID, nil
		}
	}

	log.Println("Warning: no node chose.")
	for nodeID := range candidates {
		return nodeID, nil // Fallback
	}

	return "", fmt.Errorf("error choosing node")
}
