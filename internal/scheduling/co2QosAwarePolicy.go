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
	log.Printf("decision: ", decision)
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

	var probs ProbsV2
	if ok {
		probs = val.(ProbsV2)
	} else {
		// Defaults: split evenly; spread cloud share over regions
		probs = ProbsV2{
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
func randomizedChoiceV2(probs ProbsV2) (schedDecision, error) {
	// Deterministic iteration over regions
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

	// Sample
	rv := rand.Float64()
	cum := probs.PLocal / sum
	if rv < cum {
		return schedDecision{action: EXEC_LOCAL}, nil
	}

	// Cloud regions
	for _, region := range regionNames {
		p := probs.PCloud[region] / sum
		cum += p
		if rv < cum {
			remote := regionToRemoteHost(region)
			return schedDecision{action: EXEC_REMOTE, remoteHost: remote}, nil
		}
	}

	// Edge
	cum += probs.PEdge / sum
	if rv < cum {
		return schedDecision{action: EXEC_REMOTE, remoteHost: edgeUrl}, nil
	}

	return schedDecision{action: DROP}, nil
}

// Resolve a region/area to the remote target string your scheduler expects.
func regionToRemoteHost(region string) string {
	//todo (remote host è LB)
	if ai, ok := registration.CloudRegions[region]; ok && ai.LoadBalancerNode.Key != "" {
		return ai.LoadBalancerNode.String() // "(AREA)key"
	}
	// Fallback: return the region and resolve downstream if needed
	return region
}
