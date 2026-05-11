package metrics

import (
	"fmt"
	"log"

	"net/http"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/node"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var Enabled bool
var registry = prometheus.NewRegistry()
var ScrapingHandler http.Handler = nil
var durationBuckets = []float64{0.002, 0.005, 0.010, 0.02, 0.03, 0.05, 0.1, 0.15, 0.3, 0.6, 1.0}
var cpuUsageBuckets = []float64{0.1, 0.25, 0.5, 0.75, 1.0}

const (
	COMPLETIONS              = "completed_count"
	COLD_STARTS              = "cold_starts_count"
	EXECUTION_TIME           = "execution_time"
	INITIALIZATION_TIME      = "init_time"
	INPUT_SIZE               = "input_size"
	OUTPUT_SIZE              = "output_size"
	BRANCH_COUNT             = "branch_count"
	EXECUTION_TIME_AREA      = "execution_time_by_area"
	INITIALIZATION_TIME_AREA = "init_time_by_area"
	CO2_EMITTED_GRAMS_TOTAL  = "co2_emitted_grams_total"
	COMPLETIONS_NODE         = "completed_node_count"
	COLD_STARTS_NODE         = "cold_starts_node_count"
	CPU_USAGE                = "cpu_usage"
	CPU_USAGE_AREA           = "cpu_usage_by_area"
)

var (
	metricCompletions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: COMPLETIONS,
		Help: "Number of completed function invocations per area",
	}, []string{"area", "function"})
	metricColdStarts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: COLD_STARTS,
		Help: "Number of cold starts per function and area",
	}, []string{"area", "function"})
	metricExecutionTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    EXECUTION_TIME,
		Help:    "Function duration",
		Buckets: durationBuckets,
	}, []string{"node", "function"})
	metricInitializationTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: INITIALIZATION_TIME,
		Help: "Function initialization time (cold start duration)",
	}, []string{"node", "function"})
	metricInputSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: INPUT_SIZE,
		Help: "Function input size",
	}, []string{"function"})
	metricOutputSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: OUTPUT_SIZE,
		Help: "Function output size",
	}, []string{"function"})
	metricBranchCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: BRANCH_COUNT,
		Help: "Number of executions of a task among multiple alternatives",
	}, []string{"task", "next_task"})
	metricExecutionTimeByArea = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    EXECUTION_TIME_AREA,
		Help:    "Function duration (area-labelled)",
		Buckets: durationBuckets,
	}, []string{"area", "node", "function"})

	metricInitializationTimeByArea = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    INITIALIZATION_TIME_AREA,
		Help:    "Function initialization time (area-labelled)",
		Buckets: durationBuckets,
	}, []string{"area", "node", "function"})
	metricCO2EmittedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: CO2_EMITTED_GRAMS_TOTAL,
		Help: "Total grams of CO2 emitted by function execution",
	}, []string{"area", "node", "function"})

	//for PCold Start for each node
	metricCompletionsNode = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: COMPLETIONS_NODE,
		Help: "Number of completed function invocations per node",
	}, []string{"node", "function", "class"})

	metricColdStartsNode = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: COLD_STARTS_NODE,
		Help: "Number of cold starts per function and node",
	}, []string{"node", "function", "class"})
	metricCPUUsage = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    CPU_USAGE,
		Help:    "CPU usage (fraction of a core) during function execution",
		Buckets: cpuUsageBuckets,
	}, []string{"node", "function"})

	metricCPUUsageByArea = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    CPU_USAGE_AREA,
		Help:    "CPU usage (fraction of a core) during function execution (area-labelled)",
		Buckets: cpuUsageBuckets,
	}, []string{"area", "node", "function"})
)

type RetrievedMetrics struct {
	ExtPrvColdStartProbability      map[string]float64
	AvgExtPrvRemoteExecutionTime    map[string]float64
	AvgExtPrvRemoteInitTime         map[string]float64
	RemoteColdStartProbability      map[string]float64
	AvgRemoteExecutionTime          map[string]float64
	AvgEdgeExecutionTime            map[string]map[string]float64
	AvgRemoteInitTime               map[string]float64
	AvgEdgeInitTime                 map[string]map[string]float64
	EdgeColdStartProbability        map[string]float64
	AvgInputSize                    map[string]float64
	AvgOutputSize                   map[string]float64
	BranchFrequency                 map[string]map[string]float64
	ArrivalRates                    map[string]float64 // Key: "func|QosClass"
	CloudRegionColdStartProbability map[string]map[string]float64
	AvgCloudRegionExecutionTime     map[string]map[string]float64
	AvgCloudRegionInitTime          map[string]map[string]float64
	EdgeColdStartProbabilityByNode  map[string]map[string]float64 // node -> function -> pCold
	AvgEdgeCPUUsage                 map[string]map[string]float64 // node -> function -> avg cpu
	AvgCloudRegionCPUUsage          map[string]map[string]float64 // area -> function -> avg cpu

}

func (r RetrievedMetrics) String() string {
	s := ""
	s += "REMOTE COLD START PROB:\n"
	s += fmt.Sprintf("  %v\n\n", r.RemoteColdStartProbability)
	s += "EXTERNAL PROVIDER COLD START PROB:\n"
	s += fmt.Sprintf("  %v\n\n", r.ExtPrvColdStartProbability)
	s += "EDGE NODE COLD START PROB:\n"
	s += fmt.Sprintf(" %v\n\n", r.EdgeColdStartProbability)
	s += "REMOTE EXEC TIMES:\n"
	s += fmt.Sprintf("  %v\n\n", r.AvgRemoteExecutionTime)
	s += "EXTERNAL PROVIDER EXEC TIMES:\n"
	s += fmt.Sprintf("  %v\n\n", r.AvgExtPrvRemoteExecutionTime)
	s += "EDGE EXEC TIMES:\n"
	s += fmt.Sprintf("  %v\n\n", r.AvgEdgeExecutionTime)
	s += "REMOTE INIT TIMES:\n"
	s += fmt.Sprintf("  %v\n\n", r.AvgRemoteInitTime)
	s += "EXTERNAL PROVIDER INIT TIMES:\n"
	s += fmt.Sprintf("  %v\n\n", r.AvgExtPrvRemoteInitTime)
	s += "EDGE INIT TIMES:\n"
	s += fmt.Sprintf("  %v\n\n", r.AvgEdgeInitTime)
	s += "INPUT SIZE:\n"
	s += fmt.Sprintf(" %v\n\n", r.AvgInputSize)
	s += "OUTPUT SIZE:\n"
	s += fmt.Sprintf("  %v\n\n", r.AvgOutputSize)
	s += "BRANCH FREQ:\n"
	s += fmt.Sprintf("  %v\n\n", r.BranchFrequency)
	s += "CLOUD REGION REMOTE COLD START PROB:\n"
	s += fmt.Sprintf("  %v\n\n", r.CloudRegionColdStartProbability)
	s += "CLOUD REGION EXEC TIMES:\n"
	s += fmt.Sprintf("  %v\n\n", r.AvgCloudRegionExecutionTime)
	s += "CLOUD REGION INIT TIMES:\n"
	s += fmt.Sprintf(" %v\n\n", r.AvgCloudRegionInitTime)

	return s
}

func Init() {
	if config.GetBool(config.METRICS_ENABLED, false) {
		log.Println("Metrics enabled.")
		Enabled = true
	} else {
		Enabled = false
		return
	}

	registry.MustRegister(metricCompletions)
	registry.MustRegister(metricColdStarts)
	registry.MustRegister(metricExecutionTime)
	registry.MustRegister(metricInitializationTime)
	registry.MustRegister(metricInputSize)
	registry.MustRegister(metricOutputSize)
	registry.MustRegister(metricBranchCount)

	registry.MustRegister(metricExecutionTimeByArea)
	registry.MustRegister(metricInitializationTimeByArea)
	registry.MustRegister(metricCO2EmittedTotal)

	registry.MustRegister(metricCompletionsNode)
	registry.MustRegister(metricColdStartsNode)

	registry.MustRegister(metricCPUUsage)
	registry.MustRegister(metricCPUUsageByArea)

	ScrapingHandler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true})

	go MetricsRetriever()
}

func AddCompletedInvocation(funcName string, coldStart bool) {
	log.Printf("[METRICS] CompletedInvocation area=%s function=%s coldStart=%t",
		node.LocalNode.Area, funcName, coldStart)
	metricCompletions.With(prometheus.Labels{"function": funcName, "area": node.LocalNode.Area}).Inc()
	if coldStart {
		metricColdStarts.With(prometheus.Labels{"function": funcName, "area": node.LocalNode.Area}).Inc()
	}
}

func AddCompletion(funcName string, className string, coldStart bool) {
	if className == "" {
		return
	}
	log.Printf("[METRICS] New Completion area=%s node=%s function=%s class=%s coldStart=%t",
		node.LocalNode.Area, node.LocalNode.String(), funcName, className, coldStart)

	// per-node, per-class counters
	n := node.LocalNode.String()
	metricCompletionsNode.With(prometheus.Labels{
		"function": funcName,
		"node":     n,
		"class":    className,
	}).Inc()
	if coldStart {
		metricColdStartsNode.With(prometheus.Labels{
			"function": funcName,
			"node":     n,
			"class":    className,
		}).Inc()
	}
}

func AddFunctionDurationValue(funcName string, duration float64) {
	log.Printf("[METRICS] FunctionDuration node=%s function=%s duration=%.6f",
		node.LocalNode.String(), funcName, duration)
	metricExecutionTime.With(prometheus.Labels{"function": funcName, "node": node.LocalNode.String()}).Observe(duration)
}
func AddFunctionInitTimeValue(funcName string, initTime float64) {
	log.Printf("[METRICS] FunctionInitTime node=%s function=%s initTime=%.6f",
		node.LocalNode.String(), funcName, initTime)
	metricInitializationTime.With(prometheus.Labels{"function": funcName, "node": node.LocalNode.String()}).Observe(initTime)
}
func AddFunctionOutputSizeValue(funcName string, size float64) {
	log.Printf("[METRICS] FunctionOutputSize function=%s size=%.0f",
		funcName, size)
	metricOutputSize.With(prometheus.Labels{"function": funcName}).Observe(size)
}
func AddFunctionInputSizeValue(funcName string, size float64) {
	log.Printf("[METRICS] FunctionInputSize function=%s size=%.0f", funcName, size)
	metricInputSize.With(prometheus.Labels{"function": funcName}).Observe(size)
}
func AddBranchCount(taskId string, nextTaskId string) {
	metricBranchCount.With(prometheus.Labels{"task": taskId, "next_task": nextTaskId}).Inc()
}

// For AWS Lambda

func AddRemoteFunctionDurationValue(funcName string, nodeLabel string, duration float64) {
	log.Printf("[METRICS] RemoteFunctionDuration node=%s function=%s duration=%.6f",
		nodeLabel, funcName, duration)
	metricExecutionTime.With(prometheus.Labels{"function": funcName, "node": nodeLabel}).Observe(duration)
}

func AddRemoteFunctionInitTimeValue(funcName string, nodeLabel string, initTime float64) {
	log.Printf("[METRICS] RemoteFunctionInitTime node=%s function=%s initTime=%.6f",
		nodeLabel, funcName, initTime)

	metricInitializationTime.With(prometheus.Labels{"function": funcName, "node": nodeLabel}).Observe(initTime)
}

func AddRemoteCompletedInvocation(funcName string, nodeLabel string, coldStart bool) {
	log.Printf("[METRICS] RemoteCompletedInvocation area=%s function=%s coldStart=%t",
		nodeLabel, funcName, coldStart)
	metricCompletions.With(prometheus.Labels{"function": funcName, "area": nodeLabel}).Inc()
	if coldStart {
		metricColdStarts.With(prometheus.Labels{"function": funcName, "area": nodeLabel}).Inc()
	}
}

func AddFunctionCpuUsageValue(funcName string, cpuUsage float64) {
	log.Printf("[METRICS] FunctionCpuUsage node=%s function=%s cpuUsage=%.6f",
		node.LocalNode.String(), funcName, cpuUsage)

	metricCPUUsage.With(prometheus.Labels{
		"function": funcName,
		"node":     node.LocalNode.String(),
	}).Observe(cpuUsage)
}

// BY AREA
func AddFunctionCpuUsageValueArea(funcName string, cpuUsage float64) {
	log.Printf("[METRICS] FunctionCpuUsageArea node=%s area=%s function=%s cpuUsage=%.6f",
		node.LocalNode.String(), node.LocalNode.Area, funcName, cpuUsage)

	metricCPUUsageByArea.With(prometheus.Labels{
		"function": funcName,
		"node":     node.LocalNode.String(),
		"area":     node.LocalNode.Area,
	}).Observe(cpuUsage)
}

// BY AREA
func AddFunctionDurationValueArea(funcName string, duration float64) {
	log.Printf("[METRICS] FunctionDurationArea node=%s area=%s function=%s duration=%.6f",
		node.LocalNode.String(), node.LocalNode.Area, funcName, duration)
	metricExecutionTimeByArea.With(prometheus.Labels{
		"function": funcName,
		"node":     node.LocalNode.String(),
		"area":     node.LocalNode.Area,
	}).Observe(duration)
}

func AddFunctionInitTimeValueArea(funcName string, initTime float64) {
	log.Printf("[METRICS] FunctionInitTimeArea node=%s area=%s function=%s initTime=%.6f",
		node.LocalNode.String(), node.LocalNode.Area, funcName, initTime)
	metricInitializationTimeByArea.With(prometheus.Labels{
		"function": funcName,
		"node":     node.LocalNode.String(),
		"area":     node.LocalNode.Area,
	}).Observe(initTime)
}

func AddFunctionCO2Emitted(funcName string, grams float64) {
	metricCO2EmittedTotal.With(prometheus.Labels{
		"function": funcName,
		"node":     node.LocalNode.String(),
		"area":     node.LocalNode.Area,
	}).Add(grams)
}

//self.g_co2_emissions = {}   #key: <function, schedulerDecision>, value: g CO2
