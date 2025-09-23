package scheduling

import (
	"net/http"
	"time"
)

type remotePolicyParams struct {
	CloudNodes   []string            `json:"cloud_nodes"`  // Set of Cloud nodes
	EdgeNodes    []string            `json:"edge_nodes"`   // Set of Edge nodes
	NodeMemory   map[string]float64  `json:"node_memory"`  // Memory per node
	DSLatency    map[string]float64  `json:"ds_latency"`   // Latency per node
	DSBandwidth  map[string]float64  `json:"ds_bandwidth"` // Bandwidth per node
	NodeLatency  map[string]float64  `json:"node_latency"` // map[json.dumps((src_node, dst_node))] = latency
	HandlingNode string              `json:"handling_node"`
	T            []string            `json:"T"`           // Set of tasks
	Adj          map[string][]string `json:"adj"`         // Task adjacency list
	TaskMemory   map[string]float64  `json:"task_memory"` // Memory per task
	Deadline     float64             `json:"deadline"`    // Global deadline
	OutputSize   map[string]float64  `json:"output_size"` // Output size per task
	InputSize    float64             `json:"input_size"`  // Input data size

	ObjWeights []float64           `json:"obj_weights"` // Objective terms weights
	Cost       map[string]float64  `json:"cost"`        // Computation cost
	NodeLabels map[string][]string `json:"node_labels"` // Labels per node
	TaskLabels map[string][]string `json:"task_labels"` // Labels per task

	ExecTime map[string]float64 `json:"exectime"`  // map[json.dumps((task, node))] = time
	InitTime map[string]float64 `json:"init_time"` // map[json.dumps((task, node))] = time
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

type Probs struct {
	PLocal float64 `json:"p_local"`
	PCloud float64 `json:"p_cloud"`
	PEdge  float64 `json:"p_edge"`
	PDrop  float64 `json:"p_drop"`
}

func prepareParameters(r *scheduledRequest) *remotePolicyParams {
	return nil
}
