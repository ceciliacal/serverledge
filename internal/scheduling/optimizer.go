package scheduling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/externalprovider/lambda"
	"github.com/serverledge-faas/serverledge/internal/function"
	"github.com/serverledge-faas/serverledge/internal/metrics"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/registration"
	"golang.org/x/exp/slices"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

type Probs struct {
	PLocal float64 `json:"PLocal"`
	PCloud float64 `json:"PCloud"`
	PEdge  float64 `json:"PEdge"`
	PDrop  float64 `json:"PDrop"`
}

type optimizerPayload struct {
	Functions []FunctionDef `json:"functions"` //OK
	Classes   []ClassDef    `json:"classes"`   //OK

	Budget                float64            `json:"budget"`                   //OK
	CloudComputeCostGbSec float64            `json:"cloud_compute_cost_gbsec"` //OK
	CloudRequestCost      float64            `json:"cloud_request_cost"`       //OK
	Cost                  map[string]float64 `json:"cost"`                     //OK

	HandlingNode         string             `json:"handling_node"`          //OK
	EdgeNodes            []string           `json:"edge_nodes"`             //OK
	CloudNodes           []string           `json:"cloud_nodes"`            //OK
	NodeMemory           map[string]float64 `json:"node_memory"`            // Memoria DISPONIBILE OK
	NodeLatency          map[string]float64 `json:"node_latency"`           // Latenza RTT OK
	DSBandwidth          map[string]float64 `json:"ds_bandwidth"`           //OK
	ArrivalRates         map[string]float64 `json:"arrival_rates"`          // Chiave: "funzione|classe" //MANCA
	ExecTime             map[string]float64 `json:"exec_time"`              // Chiave: "[\"funzione\",\"nodo\"]" OK
	InitTime             map[string]float64 `json:"init_time"`              // Chiave: "[\"funzione\",\"nodo\"]" OK
	AggregatedEdgeMemory float64            `json:"aggregated_edge_memory"` //OK
}

type FunctionDef struct {
	Name      string `json:"name"`
	MemoryMB  int64  `json:"memory"`
	InputSize int64  `json:"input_size"`
}

type ClassDef struct {
	Id              int64   `json:"id"`
	Name            string  `json:"name"`
	MaxRespTime     float64 `json:"max_resp_time"`
	Utility         float64 `json:"utility"`
	DeadlinePenalty float64 `json:"deadline_penalty"`
	DropPenalty     float64 `json:"drop_penalty"`
}

type QoSClassMetadata struct {
	Id              int64   `yaml:"id" json:"id"`
	Name            string  `yaml:"name" json:"name"`
	MaxRespTime     float64 `yaml:"max_resp_time" json:"max_resp_time"`
	Utility         float64 `yaml:"utility" json:"utility"`
	DeadlinePenalty float64 `yaml:"deadline_penalty" json:"deadline_penalty"`
	DropPenalty     float64 `yaml:"drop_penalty" json:"drop_penalty"`
}

func initOptimizerParams() optimizerPayload {
	return optimizerPayload{
		NodeMemory:   make(map[string]float64),
		NodeLatency:  make(map[string]float64),
		DSBandwidth:  make(map[string]float64),
		ArrivalRates: make(map[string]float64),
		ExecTime:     make(map[string]float64),
		InitTime:     make(map[string]float64),
		Cost:         make(map[string]float64),
	}
}

var qosRegistry = make(map[int64]QoSClassMetadata)

func (policy *IlpOffloadingPolicy) optimizerLoop() {
	ticker := time.NewTicker(time.Duration(15) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Polling: Begin strategic update...")

		policy.calculateArrivalRates()
		params, err := policy.prepareOptimizerParams()

		if err != nil {
			log.Printf("Error in the preparation of parameters, skipping optimization...: %v", err)
			continue
		}

		PrintOptimizerPayload(params)

		jsonData, err := json.Marshal(params)
		if err != nil {
			log.Printf("Polling: Error in marshalling: %v", err)
			continue
		}

		ilpOptimizerHost := config.GetString(config.FUNCTION_OFFLOADING_POLICY_OPTIMIZER_HOST, "localhost")
		ilpOptimizerPort := config.GetInt(config.FUNCTION_OFFLOADING_POLICY_OPTIMIZER_PORT, 8080)
		url := fmt.Sprintf("http://%s:%d/optimize", ilpOptimizerHost, ilpOptimizerPort)

		resp, err := policy.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Polling: Errore nella chiamata all'ottimizzatore: %v", err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var newProbs map[string]Probs
			if err := json.NewDecoder(resp.Body).Decode(&newProbs); err == nil {
				for key, value := range newProbs {
					policy.probabilityCache.Store(key, value)
				}
				log.Printf("Polling: Cache of probability update done")
			} else {
				log.Printf("Polling: Error in decoding the response: %v", err)
			}
		} else {
			log.Printf("Polling: Response status: %s", resp.Status)
		}
		resp.Body.Close()
	}
}

func (policy *IlpOffloadingPolicy) prepareOptimizerParams() (optimizerPayload, error) {
	var LOCAL = registration.SelfRegistration.Key //local
	var EXTERNAL = "external"

	params := initOptimizerParams()
	params.HandlingNode = LOCAL
	params.EdgeNodes = []string{LOCAL}
	params.NodeMemory[LOCAL] = (float64)(node.Resources.AvailableMemMB)

	regionCost := config.GetStringMapFloat64(config.FUNCTION_OFFLOADING_POLICY_REGION_COST)

	localCost, ok := regionCost[strings.ToLower(registration.SelfRegistration.Area)]
	if !ok {
		localCost = 0.0
	}
	params.Cost[LOCAL] = localCost

	// Add available Edge peers
	nearbyServers := registration.GetFullNeighborInfo()

	if nearbyServers != nil {
		for k, v := range nearbyServers {
			if v.AvailableMemMB > 0 && v.AvailableCPUs > 0 {
				params.EdgeNodes = append(params.EdgeNodes, k)
				params.NodeMemory[k] = float64(v.AvailableMemMB)
				params.AggregatedEdgeMemory += float64(v.AvailableMemMB)
				// Cost (assuming that Edge nodes are all in the same area)
				params.Cost[k] = localCost

			}
		}
	}

	// Compute distances
	for key1, v1 := range nearbyServers {
		var distance float64
		if !slices.Contains(params.EdgeNodes, key1) {
			continue
		}
		distance = registration.VivaldiClient.DistanceTo(&v1.Coordinates).Seconds()
		params.NodeLatency[tupleKey(LOCAL, key1)] = distance
		params.NodeLatency[tupleKey(key1, LOCAL)] = distance

		for key2, v2 := range nearbyServers {
			if !slices.Contains(params.EdgeNodes, key2) {
				continue
			}
			if key1 == key2 {
				distance = 0.0
			} else {
				distance = v1.Coordinates.DistanceTo(&v2.Coordinates).Seconds()
			}
			params.NodeLatency[tupleKey(key1, key2)] = distance
			params.NodeLatency[tupleKey(key2, key1)] = distance
		}
	}

	//Bandwidth
	dsBandwidth := config.GetFloat(
		config.FUNCTION_OFFLOADING_POLICY_NODE_TO_DATA_STORE_BANDWIDTH,
		100.0, // default Mbps per edge
	)

	for _, n := range params.EdgeNodes {
		params.DSBandwidth[n] = dsBandwidth
	}

	isExternalProviderEnabled := config.GetBool(config.EXTERNAL_PROVIDER_ENABLED, true)

	if isExternalProviderEnabled {
		params.CloudNodes = []string{EXTERNAL}
		costCloudGbSec := config.GetStringMapFloat64(config.FUNCTION_OFFLOADING_COMPUTE_REGION_COST_GB_SEC)
		costCloudReq := config.GetStringMapFloat64(config.FUNCTION_OFFLOADING_COMPUTE_REGION_REQUEST_COST)
		budget := config.GetFloat(config.FUNCTION_OFFLOADING_BUDGET, 0.0)

		params.Budget = budget

		cloudRegion, err := lambda.GetRegion()

		if err != nil {
			return optimizerPayload{}, fmt.Errorf("impossible obtain cloudRegion: %w", err)
		}

		cloudRegionGbSec := costCloudGbSec[strings.ToLower(cloudRegion)]
		cloudRegionReq := costCloudReq[strings.ToLower(cloudRegion)]

		params.CloudComputeCostGbSec = cloudRegionGbSec
		params.CloudRequestCost = cloudRegionReq

		lambdaBw := config.GetFloat(
			config.FUNCTION_OFFLOADING_POLICY_LAMBDA_TO_DATA_STORE_BANDWIDTH,
			dsBandwidth*10, // default: 10× edge
		)

		params.DSBandwidth[EXTERNAL] = lambdaBw

		//Now we need to measure latency to Lambda
		//Latency locale misurata come: TCP 3-way handshake

		distanceToCloudDuration := lambda.GetLambdaRtt()
		distanceToCloudSec := distanceToCloudDuration.Seconds()

		for _, n := range params.EdgeNodes {
			params.NodeLatency[tupleKey(n, EXTERNAL)] = distanceToCloudSec
			params.NodeLatency[tupleKey(EXTERNAL, n)] = distanceToCloudSec

		}
		params.NodeLatency[tupleKey(EXTERNAL, EXTERNAL)] = 0.0
	}

	//QoS
	allClasses := getAllQoSClasses()
	params.Classes = make([]ClassDef, 0, len(allClasses))

	for _, metadata := range allClasses {
		newQos := ClassDef{
			Id:              metadata.Id,
			Name:            metadata.Name,
			MaxRespTime:     metadata.MaxRespTime,
			Utility:         metadata.Utility,
			DeadlinePenalty: metadata.DeadlinePenalty,
			DropPenalty:     metadata.DropPenalty,
		}
		params.Classes = append(params.Classes, newQos)
	}

	functionNames, err := function.GetAll()

	if err != nil {
		return optimizerPayload{}, fmt.Errorf("impossible obtain functionNames: %w", err)
	}

	retrievedMetrics := metrics.GetMetrics()

	for _, functionName := range functionNames {
		realFunc, ok := function.GetFunction(functionName)
		if !ok {
			log.Printf("Impossible get the function, skipping...")
			continue
		}

		var avgInputSize = 100.0

		if size, ok := retrievedMetrics.AvgInputSize[functionName]; ok && size > 0 {
			avgInputSize = size
		}
		newFuncDef := FunctionDef{
			Name:      functionName,
			MemoryMB:  realFunc.MemoryMB,
			InputSize: int64(avgInputSize),
		}

		params.Functions = append(params.Functions, newFuncDef)

		for _, n := range params.EdgeNodes {
			nId := node.NodeID{Area: registration.SelfRegistration.Area, Key: n}

			execTime := 0.01 // Default
			if nodeTimes, ok := retrievedMetrics.AvgEdgeExecutionTime[nId.String()]; ok {
				if t, ok2 := nodeTimes[functionName]; ok2 {
					execTime = t
				}
			}
			params.ExecTime[tupleKey(functionName, n)] = execTime

			pColdEdge := 1.0 // Default
			if prob, ok := retrievedMetrics.EdgeColdStartProbability[functionName]; ok {
				pColdEdge = prob
			}

			avgInitEdge := 0.1 // Default
			if initTimes, ok := retrievedMetrics.AvgEdgeInitTime[nId.String()]; ok {
				if t, ok2 := initTimes[functionName]; ok2 {
					avgInitEdge = t
				}
			}
			params.InitTime[tupleKey(functionName, n)] = pColdEdge * avgInitEdge
		}

		if len(params.CloudNodes) > 0 {
			cloudNodeID := params.CloudNodes[0] // External

			execTimeCloud := 0.01 // Default
			if t, ok := retrievedMetrics.AvgExtPrvRemoteExecutionTime[functionName]; ok && !math.IsNaN(t) && !math.IsInf(t, 0) {
				execTimeCloud = t
			}
			params.ExecTime[tupleKey(functionName, cloudNodeID)] = execTimeCloud

			pColdCloud := 1.0 // Default
			if p, ok := retrievedMetrics.ExtPrvColdStartProbability[functionName]; ok && !math.IsNaN(p) && !math.IsInf(p, 0) {
				pColdCloud = p
			}

			avgInitCloud := 0.1 // Default
			if t, ok := retrievedMetrics.AvgExtPrvRemoteInitTime[functionName]; ok && !math.IsNaN(t) && !math.IsInf(t, 0) {
				avgInitCloud = t
			}
			params.InitTime[tupleKey(functionName, cloudNodeID)] = pColdCloud * avgInitCloud
		}
	}

	//Arrival Rates from policy struct
	policy.arrivalRatesMutex.RLock()
	ratesCopy := make(map[string]float64)
	for k, v := range policy.arrivalRates {
		ratesCopy[k] = v
	}
	policy.arrivalRatesMutex.RUnlock()
	params.ArrivalRates = ratesCopy

	return params, nil
}

// LoadQoSDefinitions yaml parsing
func loadQoSDefinitions(filePath string) error {

	// Read yml
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading QoS YAML '%s': %w", filePath, err)
	}

	var configData struct {
		Classes []QoSClassMetadata `yaml:"classes"`
	}

	// 3. Esegue il parsing (Unmarshal) del contenuto del file nella struct.
	if err := yaml.Unmarshal(yamlFile, &configData); err != nil {
		return fmt.Errorf("errore nel parsing del file QoS YAML: %w", err)
	}

	for _, classDef := range configData.Classes {
		qosRegistry[classDef.Id] = classDef
	}

	log.Printf("Recovered %d QoS classes with success.", len(qosRegistry))
	return nil
}

func getQoSClassNameByID(id int64) (string, bool) {
	class, found := qosRegistry[id]
	return class.Name, found
}

// GetAllQoSClasses restituisce tutte le classi definite, utile per il payload dell'ottimizzatore.
func getAllQoSClasses() []QoSClassMetadata {
	classes := make([]QoSClassMetadata, 0, len(qosRegistry))
	for _, classDef := range qosRegistry {
		classes = append(classes, classDef)
	}
	return classes
}

func (policy *IlpOffloadingPolicy) calculateArrivalRates() {

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
	log.Printf("Arrival rates update: %v", policy.arrivalRates)
}

func tupleKey(s1, s2 string) string {
	keyBytes, _ := json.Marshal([]string{s1, s2})
	return string(keyBytes)
}

func PrintOptimizerPayload(params optimizerPayload) {
	var sb strings.Builder

	sb.WriteString("\n\n==========================================================\n")
	sb.WriteString("---   Snapshot del Payload per l'Ottimizzatore   ---\n")
	sb.WriteString("==========================================================\n")

	// --- 1. Definizioni Statiche ---
	sb.WriteString("\n=== 1. Definizioni Statiche (da Config/YAML) ===\n")
	fmt.Fprintf(&sb, "Funzioni definite (%d):\n", len(params.Functions))
	if len(params.Functions) == 0 {
		sb.WriteString("  (Nessuna funzione trovata)\n")
	}
	for _, f := range params.Functions {
		fmt.Fprintf(&sb, "  - Nome: %-25s | Memoria Richiesta: %d MB\n", f.Name, f.MemoryMB)
	}

	fmt.Fprintf(&sb, "\nClassi QoS definite (%d):\n", len(params.Classes))
	if len(params.Classes) == 0 {
		sb.WriteString("  (Nessuna classe QoS trovata)\n")
	}
	for _, c := range params.Classes {
		fmt.Fprintf(&sb, "  - ID: %-2d | Nome: %-15s | MaxResp: %.2fs | Utility: %.2f | DeadlinePenalty: %.2f | DropPenalty: %.2f\n",
			c.Id, c.Name, c.MaxRespTime, c.Utility, c.DeadlinePenalty, c.DropPenalty)
	}

	// --- 2. Parametri della Policy ---
	sb.WriteString("\n=== 2. Parametri della Policy (da Config/INI) ===\n")
	fmt.Fprintf(&sb, "Budget:                 %.4f $/ora\n", params.Budget)
	fmt.Fprintf(&sb, "Costo Cloud (GB/sec):   %.8f\n", params.CloudComputeCostGbSec)
	fmt.Fprintf(&sb, "Costo Cloud (richiesta): %.8f\n", params.CloudRequestCost)
	fmt.Fprintf(&sb, "Costi per Nodo (%d voci):\n", len(params.Cost))
	if len(params.Cost) == 0 {
		sb.WriteString("  (Nessun costo per nodo definito)\n")
	}
	for node, cost := range params.Cost {
		fmt.Fprintf(&sb, "  - %-30s: %.6f\n", node, cost)
	}

	// --- 3. Dati Dinamici "Live" ---
	sb.WriteString("\n=== 3. Dati Dinamici \"Live\" (da Stato del Sistema e Prometheus) ===\n")
	fmt.Fprintf(&sb, "Nodo Gestore:          %s\n", params.HandlingNode)
	fmt.Fprintf(&sb, "Nodi Edge Attivi:      %v\n", params.EdgeNodes)
	fmt.Fprintf(&sb, "Nodi Cloud:            %v\n", params.CloudNodes)
	fmt.Fprintf(&sb, "Memoria Aggregata Edge: %.2f MB\n", params.AggregatedEdgeMemory)

	fmt.Fprintf(&sb, "\nMemoria Disponibile per Nodo (%d voci):\n", len(params.NodeMemory))
	for node, mem := range params.NodeMemory {
		fmt.Fprintf(&sb, "  - %-30s: %.2f MB\n", node, mem)
	}

	fmt.Fprintf(&sb, "\nLatenza RTT tra Nodi (%d voci):\n", len(params.NodeLatency))
	for nodes, lat := range params.NodeLatency {
		fmt.Fprintf(&sb, "  - %-30s: %.4f s\n", nodes, lat)
	}

	fmt.Fprintf(&sb, "\nBanda verso il Datastore (%d voci):\n", len(params.DSBandwidth))
	for node, bw := range params.DSBandwidth {
		fmt.Fprintf(&sb, "  - %-30s: %.2f Mbps\n", node, bw)
	}

	fmt.Fprintf(&sb, "\nTempi di Esecuzione Medi (ExecTime) (%d voci):\n", len(params.ExecTime))
	for key, val := range params.ExecTime {
		fmt.Fprintf(&sb, "  - %-40s: %.6f s\n", key, val)
	}

	fmt.Fprintf(&sb, "\nTempi di Inizializzazione Attesi (InitTime) (%d voci):\n", len(params.InitTime))
	for key, val := range params.InitTime {
		fmt.Fprintf(&sb, "  - %-40s: %.6f s\n", key, val)
	}

	fmt.Fprintf(&sb, "\nTassi di Arrivo (ArrivalRates) (%d voci):\n", len(params.ArrivalRates))
	if len(params.ArrivalRates) == 0 {
		sb.WriteString("  (Nessun tasso di arrivo misurato)\n")
	}
	for key, val := range params.ArrivalRates {
		fmt.Fprintf(&sb, "  - %-30s: %.6f req/s\n", key, val)
	}

	sb.WriteString("\n==========================================================\n\n")

	// Stampa l'intera stringa costruita nel log
	log.Println(sb.String())
}
