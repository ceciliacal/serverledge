package scheduling

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/container"
	"github.com/serverledge-faas/serverledge/internal/function"
	"github.com/serverledge-faas/serverledge/internal/metrics"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/regions"
	"github.com/serverledge-faas/serverledge/internal/registration"
	"github.com/spf13/viper"
)

func testPolicyFunction(name string) *function.Function {
	return &function.Function{
		Name:           name,
		Runtime:        "python314",
		MemoryMB:       128,
		CPUDemand:      0.1,
		MaxConcurrency: 1,
		Handler:        name + ".handler",
		SupportedArchs: []string{"amd64"},
		IsDefault:      true,
		SpeedUp:        1,
	}
}

func testPolicy(t *testing.T, fun *function.Function) *Co2QosAwarePolicy {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set(config.POOL_MEMORY_MB, 2048)
	viper.Set(config.POOL_CPUS, 2.0)
	viper.Set(config.PROCESSING_POWER_CONSUMPTION, 100.0)
	viper.Set(config.TX_ENERGY_CONSUMPTION, 51200.0)
	viper.Set(config.RX_ENERGY_CONSUMPTION, 30720.0)
	node.LocalNode = node.NodeID{Area: "edge", Key: "local", Arch: "amd64"}
	node.LocalResources.Init()
	node.LocalResources.Co2Footprint.Set(time.Unix(0, 0), 50, 1)
	qosClasses = map[int64]QoSClass{0: {Id: 0, Name: "standard", MaxRespTime: 1, Utility: 1}}

	p := &Co2QosAwarePolicy{
		updateInterval: 10 * time.Second,
		arrivalAlpha:   1,
		AlphaWeight:    1,
		BetaWeight:     1,
		httpClient:     &http.Client{Timeout: time.Second},
		Config:         Co2QosPolicyConfig{OptHost: "127.0.0.1", OptPort: 1, CO2GreenThreshold: 10, Alpha: 1, Beta: 1},
		listFunctions:  func() ([]string, error) { return []string{fun.Name}, nil },
		getFunction: func(name string) (*function.Function, bool) {
			if name == fun.Name {
				return fun, true
			}
			return nil, false
		},
		getMetrics: func() metrics.RetrievedMetrics { return metrics.RetrievedMetrics{} },
		getNeighborInfo: func() map[string]*registration.StatusInformation {
			return map[string]*registration.StatusInformation{}
		},
		listConfiguredAreas: func() ([]regions.AreaInfo, error) {
			return []regions.AreaInfo{
				{Name: "cloud_a", CO2TraceFile: "a.csv", Cost: 0.2},
				{Name: "cloud_b", CO2TraceFile: "b.csv", Cost: 0.3},
				{Name: "cloud_empty", CO2TraceFile: "empty.csv"},
				{Name: "cloud_arm", CO2TraceFile: "arm.csv"},
			}, nil
		},
		getLBInArea: func(area string) (map[string]registration.NodeRegistration, error) {
			switch area {
			case "cloud_a":
				return map[string]registration.NodeRegistration{
					"lb-a": {NodeID: node.NodeID{Area: area, Key: "lb-a", Arch: "amd64"}, IPAddress: "10.0.0.1", APIPort: 1323, IsLoadBalancer: true},
				}, nil
			case "cloud_b":
				return map[string]registration.NodeRegistration{
					"lb-b": {NodeID: node.NodeID{Area: area, Key: "lb-b", Arch: "amd64"}, IPAddress: "10.0.0.2", APIPort: 1324, IsLoadBalancer: true},
				}, nil
			case "cloud_arm":
				return map[string]registration.NodeRegistration{
					"lb-arm": {NodeID: node.NodeID{Area: area, Key: "lb-arm", Arch: "arm64"}, IPAddress: "10.0.0.3", APIPort: 1325, IsLoadBalancer: true},
				}, nil
			default:
				return map[string]registration.NodeRegistration{}, nil
			}
		},
		getNodeStatus: func(lb registration.NodeRegistration) (*registration.StatusInformation, error) {
			switch lb.Area {
			case "cloud_a":
				return &registration.StatusInformation{
					AvailableMemory:            4096,
					CO2Intensity:               20,
					ProcessingPowerConsumption: 800,
					TxEnergyConsumption:        0.00000512,
					RxEnergyConsumption:        0.000003072,
				}, nil
			case "cloud_b":
				return &registration.StatusInformation{
					AvailableMemory:            8192,
					CO2Intensity:               80,
					ProcessingPowerConsumption: 900,
					TxEnergyConsumption:        0.00000612,
					RxEnergyConsumption:        0.000004072,
				}, nil
			default:
				return nil, errors.New("no status")
			}
		},
		getCloudLatencySec: func(registration.NodeRegistration) (float64, error) { return 0.02, nil },
		acquireContainer: func(*function.Function, bool) (*container.Container, bool, error) {
			return &container.Container{ID: "container"}, true, nil
		},
		edgeTarget:  func(*scheduledRequest) (string, error) { return "http://edge-peer:1323", nil },
		randFloat64: func() float64 { return 0 },
	}
	p.setDefaultHooks()
	return p
}

func testScheduledRequest(fun *function.Function) *scheduledRequest {
	return &scheduledRequest{
		Request: &function.Request{
			Fun:             fun,
			Arrival:         time.Now(),
			CanDoOffloading: true,
			RequestQoS:      function.RequestQoS{Class: 0},
		},
		ExecutionReport: &function.ExecutionReport{},
		decisionChannel: make(chan schedDecision, 1),
	}
}

func TestCo2QosEvaluateLocalExec(t *testing.T) {
	fun := testPolicyFunction("local")
	p := testPolicy(t, fun)
	storeTestSnapshot(p, map[string]MultiRegionProbs{"local|standard": {PLocal: 1, PCloud: map[string]float64{}}}, nil)

	decision, err := p.evaluate(testScheduledRequest(fun), "local|standard")
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if decision.action != EXEC_LOCAL {
		t.Fatalf("action = %v, want EXEC_LOCAL", decision.action)
	}
}

func TestCo2QosEvaluateOffloadEdge(t *testing.T) {
	fun := testPolicyFunction("edge")
	p := testPolicy(t, fun)
	storeTestSnapshot(p, map[string]MultiRegionProbs{"edge|standard": {PEdge: 1, PCloud: map[string]float64{}}}, nil)

	decision, err := p.evaluate(testScheduledRequest(fun), "edge|standard")
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if decision.action != EXEC_REMOTE || decision.remoteHost != edgeDecisionTarget {
		t.Fatalf("decision = %#v, want edge EXEC_REMOTE", decision)
	}
}

func TestCo2QosEvaluateOffloadCloud(t *testing.T) {
	fun := testPolicyFunction("cloud")
	p := testPolicy(t, fun)
	storeTestSnapshot(p, map[string]MultiRegionProbs{"cloud|standard": {PCloud: map[string]float64{"cloud_a": 1}}}, nil)

	decision, err := p.evaluate(testScheduledRequest(fun), "cloud|standard")
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if decision.action != EXEC_REMOTE || decision.remoteHost != "http://10.0.0.1:1323" {
		t.Fatalf("decision = %#v, want cloud_a EXEC_REMOTE", decision)
	}
}

func TestCo2QosDiscoverCloudCandidates(t *testing.T) {
	fun := testPolicyFunction("candidates")
	p := testPolicy(t, fun)

	candidates, _ := p.discoverCloudCandidates(fun)
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want two compatible LB areas", candidates)
	}
	if _, ok := candidates["cloud_a"]; !ok {
		t.Fatal("cloud_a missing")
	}
	if _, ok := candidates["cloud_b"]; !ok {
		t.Fatal("cloud_b missing")
	}
	if _, ok := candidates["cloud_empty"]; ok {
		t.Fatal("cloud_empty included without LB")
	}
	if _, ok := candidates["cloud_arm"]; ok {
		t.Fatal("cloud_arm included despite incompatible LB architecture")
	}
}

func TestCo2QosEvaluateDrop(t *testing.T) {
	fun := testPolicyFunction("drop")
	p := testPolicy(t, fun)
	storeTestSnapshot(p, map[string]MultiRegionProbs{"drop|standard": {PDrop: 1, PCloud: map[string]float64{}}}, nil)

	decision, err := p.evaluate(testScheduledRequest(fun), "drop|standard")
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if decision.action != DROP {
		t.Fatalf("action = %v, want DROP", decision.action)
	}
}

func TestParseOptimizerResponseErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{`},
		{name: "unknown decision", body: `{"f":{"standard":{"SIDEWAYS":1}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseOptimizerResponse([]byte(tt.body)); err == nil {
				t.Fatal("parseOptimizerResponse() error = nil")
			}
		})
	}
}

func TestParseOptimizerResponseSuccessHasNoExecVar(t *testing.T) {
	got, err := parseOptimizerResponse([]byte(`{"f":{"standard":{"EXEC":0.25,"OFFLOAD_EDGE":0.25,"OFFLOAD_CLOUD_cloud_a":0.25,"DROP":0.25}}}`))
	if err != nil {
		t.Fatalf("parseOptimizerResponse() error = %v", err)
	}
	probs := got.Probabilities["f|standard"]
	if probs.PLocal != 0.25 || probs.PEdge != 0.25 || probs.PDrop != 0.25 || probs.PCloud["cloud_a"] != 0.25 {
		t.Fatalf("probs = %#v", probs)
	}
}

func TestParseOptimizerResponseAcceptsRealServerWrapper(t *testing.T) {
	got, err := parseOptimizerResponse([]byte(`{"probs":{"f":{"standard":{"EXEC":1}}},"decision_fc_probability_var":{}}`))
	if err != nil {
		t.Fatalf("parseOptimizerResponse() error = %v", err)
	}
	probs := got.Probabilities["f|standard"]
	if probs.PLocal != 1 {
		t.Fatalf("PLocal = %v, want 1", probs.PLocal)
	}

	got, err = parseOptimizerResponse([]byte(`{"probs":{"f":{"standard":{"EXEC":0.5,"EXEC_VAR":0.5}}},"decision_fc_probability_var":{"f":{"f_fast":{"standard":0.5}}}}`))
	if err != nil {
		t.Fatalf("parseOptimizerResponse() rejected variant probabilities: %v", err)
	}
	if got.Probabilities["f|standard"].PLocalVar != 0.5 || got.VariantProbabilities["f|standard"]["f_fast"] != 0.5 {
		t.Fatalf("variant probabilities = %#v", got)
	}
}

func TestOptimizerUnavailableDoesNotPopulateCache(t *testing.T) {
	fun := testPolicyFunction("unavailable")
	p := testPolicy(t, fun)
	p.Config.OptHost = "127.0.0.1"
	p.Config.OptPort = 1

	err := p.invokeOptimizer(initOptCarbonAwareParams())
	if err == nil {
		t.Fatal("invokeOptimizer() error = nil")
	}
	if _, ok := p.loadOptimizerSnapshot().Probabilities["unavailable|standard"]; ok {
		t.Fatal("probability cache populated after optimizer failure")
	}
}

func TestInvokeOptimizerPopulatesProbabilityCache(t *testing.T) {
	fun := testPolicyFunction("optimizer-success")
	p := testPolicy(t, fun)
	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"optimizer-success":{"standard":{"EXEC":1}}}`)),
		}, nil
	})}

	if err := p.invokeOptimizer(initOptCarbonAwareParams()); err != nil {
		t.Fatalf("invokeOptimizer() error = %v", err)
	}
	got, ok := p.loadOptimizerSnapshot().Probabilities["optimizer-success|standard"]
	if !ok {
		t.Fatal("probability cache missing optimizer entry")
	}
	probs := got
	if probs.PLocal != 1 {
		t.Fatalf("PLocal = %v, want 1", probs.PLocal)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func storeTestSnapshot(p *Co2QosAwarePolicy, probabilities map[string]MultiRegionProbs, variants map[string]map[string]float64) {
	if variants == nil {
		variants = make(map[string]map[string]float64)
	}
	p.storeOptimizerSnapshot(optimizerSnapshot{
		Probabilities:        cloneProbabilityMap(probabilities),
		VariantProbabilities: cloneVariantProbabilityMap(variants),
	})
}

func TestCo2QosApplyDecisionSideEffects(t *testing.T) {
	fun := testPolicyFunction("side-effects")
	p := testPolicy(t, fun)
	req := testScheduledRequest(fun)
	var got string
	p.execLocal = func(*scheduledRequest, *container.Container, bool) { got = "local" }
	p.offload = func(_ *scheduledRequest, target string) { got = target }
	p.drop = func(*scheduledRequest) { got = "drop" }

	p.applyDecision(req, schedDecision{action: EXEC_LOCAL})
	if got != "local" {
		t.Fatalf("local side effect = %q", got)
	}
	p.applyDecision(req, schedDecision{action: EXEC_REMOTE, remoteHost: edgeDecisionTarget})
	if got != "http://edge-peer:1323" {
		t.Fatalf("edge side effect = %q", got)
	}
	p.applyDecision(req, schedDecision{action: EXEC_REMOTE, remoteHost: "http://cloud:1323"})
	if got != "http://cloud:1323" {
		t.Fatalf("cloud side effect = %q", got)
	}
	p.applyDecision(req, schedDecision{action: DROP})
	if got != "drop" {
		t.Fatalf("drop side effect = %q", got)
	}
}

func TestCo2QosApplyDecisionDropsWhenEdgeUnavailable(t *testing.T) {
	fun := testPolicyFunction("edge-unavailable")
	p := testPolicy(t, fun)
	p.edgeTarget = func(*scheduledRequest) (string, error) { return "", errors.New("no edge") }
	var dropped bool
	p.drop = func(*scheduledRequest) { dropped = true }

	p.applyDecision(testScheduledRequest(fun), schedDecision{action: EXEC_REMOTE, remoteHost: edgeDecisionTarget})
	if !dropped {
		t.Fatal("edge failure did not drop request")
	}
}

func TestCo2QosWorkflowStageUsesCurrentSchedulerRequest(t *testing.T) {
	fun := testPolicyFunction("workflow-stage")
	p := testPolicy(t, fun)
	req := testScheduledRequest(fun)
	req.Request.Ctx = nil
	storeTestSnapshot(p, map[string]MultiRegionProbs{"workflow-stage|standard": {PDrop: 1, PCloud: map[string]float64{}}}, nil)

	decision, err := p.evaluate(req, "workflow-stage|standard")
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if decision.action != DROP {
		t.Fatalf("action = %v, want DROP", decision.action)
	}
}

func TestCo2QosPrepareParamsRepresentsVariantsWithoutDoubleCounting(t *testing.T) {
	base := testPolicyFunction("base")
	variant := testPolicyFunction("base-fast")
	variant.IsDefault = false
	variant.DefaultFunction = "base"
	variant.SpeedUp = 10
	variant.Utility = 0.1
	functions := map[string]*function.Function{base.Name: base, variant.Name: variant}
	p := testPolicy(t, base)
	p.listFunctions = func() ([]string, error) { return []string{base.Name, variant.Name}, nil }
	p.getFunction = func(name string) (*function.Function, bool) {
		f, ok := functions[name]
		return f, ok
	}

	params, err := p.prepareOptimizerParams()
	if err != nil {
		t.Fatalf("prepareOptimizerParams() error = %v", err)
	}
	if len(params.Functions) != 1 || params.Functions[0] != "base" {
		t.Fatalf("Functions = %#v", params.Functions)
	}
	if got := params.Variants["base"]; !reflect.DeepEqual(got, []string{"base-fast"}) {
		t.Fatalf("Variants[base] = %#v", got)
	}
	if params.VariantUtility["base-fast"] != 0.1 {
		t.Fatalf("VariantUtility = %#v", params.VariantUtility)
	}
	if params.ServTimeLocal["base-fast"] != params.ServTimeLocal["base"]/10 {
		t.Fatalf("variant service time = %v, want %v", params.ServTimeLocal["base-fast"], params.ServTimeLocal["base"]/10)
	}
	if _, ok := params.ServTimeCloud["cloud_a"]["base-fast"]; ok {
		t.Fatal("variant was added to cloud timing map")
	}
	if !containsDecision(params.PossibleDecisions, "EXEC_VAR") {
		t.Fatal("possible_decisions missing EXEC_VAR")
	}
}

func TestCo2QosVariantMeasuredServiceTimeIsNotDividedBySpeedUp(t *testing.T) {
	base := testPolicyFunction("base")
	variant := testPolicyFunction("base-fast")
	variant.IsDefault = false
	variant.DefaultFunction = base.Name
	variant.SpeedUp = 10
	variant.Utility = 0.9
	functions := map[string]*function.Function{base.Name: base, variant.Name: variant}
	p := testPolicy(t, base)
	p.listFunctions = func() ([]string, error) { return []string{base.Name, variant.Name}, nil }
	p.getFunction = func(name string) (*function.Function, bool) {
		f, ok := functions[name]
		return f, ok
	}
	local := node.LocalNode.String()
	p.getMetrics = func() metrics.RetrievedMetrics {
		return metrics.RetrievedMetrics{
			AvgEdgeExecutionTime: map[string]map[string]float64{
				local: {base.Name: 1.0, variant.Name: 0.4},
			},
		}
	}

	params, err := p.prepareOptimizerParams()
	if err != nil {
		t.Fatalf("prepareOptimizerParams() error = %v", err)
	}
	if params.ServTimeLocal[variant.Name] != 0.4 {
		t.Fatalf("variant service time = %v, want measured 0.4", params.ServTimeLocal[variant.Name])
	}
}

func TestCo2QosVariantServiceTimeFallsBackToDefaultDividedBySpeedUp(t *testing.T) {
	base := testPolicyFunction("base")
	variant := testPolicyFunction("base-fast")
	variant.IsDefault = false
	variant.DefaultFunction = base.Name
	variant.SpeedUp = 4
	variant.Utility = 0.9
	functions := map[string]*function.Function{base.Name: base, variant.Name: variant}
	p := testPolicy(t, base)
	p.listFunctions = func() ([]string, error) { return []string{base.Name, variant.Name}, nil }
	p.getFunction = func(name string) (*function.Function, bool) {
		f, ok := functions[name]
		return f, ok
	}
	local := node.LocalNode.String()
	p.getMetrics = func() metrics.RetrievedMetrics {
		return metrics.RetrievedMetrics{
			AvgEdgeExecutionTime: map[string]map[string]float64{
				local: {base.Name: 1.2},
			},
		}
	}

	params, err := p.prepareOptimizerParams()
	if err != nil {
		t.Fatalf("prepareOptimizerParams() error = %v", err)
	}
	if params.ServTimeLocal[variant.Name] != 0.3 {
		t.Fatalf("variant service time = %v, want 0.3", params.ServTimeLocal[variant.Name])
	}
}

func TestCo2QosPrepareParamsKeepsVariantsSeparateByDefaultFunction(t *testing.T) {
	baseA := testPolicyFunction("base-a")
	baseB := testPolicyFunction("base-b")
	variantA := testPolicyFunction("base-a-fast")
	variantA.IsDefault = false
	variantA.DefaultFunction = baseA.Name
	variantA.SpeedUp = 2
	variantA.Utility = 0.8
	variantB := testPolicyFunction("base-b-fast")
	variantB.IsDefault = false
	variantB.DefaultFunction = baseB.Name
	variantB.SpeedUp = 3
	variantB.Utility = 0.7
	functions := map[string]*function.Function{
		baseA.Name:    baseA,
		baseB.Name:    baseB,
		variantA.Name: variantA,
		variantB.Name: variantB,
	}
	p := testPolicy(t, baseA)
	p.listFunctions = func() ([]string, error) {
		return []string{baseA.Name, baseB.Name, variantA.Name, variantB.Name}, nil
	}
	p.getFunction = func(name string) (*function.Function, bool) {
		f, ok := functions[name]
		return f, ok
	}

	params, err := p.prepareOptimizerParams()
	if err != nil {
		t.Fatalf("prepareOptimizerParams() error = %v", err)
	}
	if !reflect.DeepEqual(params.Functions, []string{baseA.Name, baseB.Name}) {
		t.Fatalf("Functions = %#v", params.Functions)
	}
	if !reflect.DeepEqual(params.Variants[baseA.Name], []string{variantA.Name}) {
		t.Fatalf("Variants[%s] = %#v", baseA.Name, params.Variants[baseA.Name])
	}
	if !reflect.DeepEqual(params.Variants[baseB.Name], []string{variantB.Name}) {
		t.Fatalf("Variants[%s] = %#v", baseB.Name, params.Variants[baseB.Name])
	}
}

func TestCo2QosExecVarSelectsAndExecutesRegisteredVariantLocally(t *testing.T) {
	base := testPolicyFunction("base")
	variant := testPolicyFunction("base-fast")
	variant.IsDefault = false
	variant.DefaultFunction = base.Name
	variant.SpeedUp = 2
	variant.Utility = 0.9
	functions := map[string]*function.Function{base.Name: base, variant.Name: variant}
	p := testPolicy(t, base)
	p.listFunctions = func() ([]string, error) { return []string{base.Name, variant.Name}, nil }
	p.getFunction = func(name string) (*function.Function, bool) {
		f, ok := functions[name]
		return f, ok
	}
	storeTestSnapshot(
		p,
		map[string]MultiRegionProbs{"base|standard": {PLocalVar: 1, PCloud: map[string]float64{}}},
		map[string]map[string]float64{"base|standard": {variant.Name: 1}},
	)

	decision, err := p.evaluate(testScheduledRequest(base), "base|standard")
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if decision.action != EXEC_LOCAL || decision.variant == nil || decision.variant.Name != variant.Name {
		t.Fatalf("decision = %#v, want local variant", decision)
	}
	req := testScheduledRequest(base)
	var acquired string
	var executed string
	p.acquireContainer = func(f *function.Function, _ bool) (*container.Container, bool, error) {
		acquired = f.Name
		return &container.Container{ID: "variant-container"}, true, nil
	}
	p.execLocal = func(r *scheduledRequest, _ *container.Container, _ bool) { executed = r.Fun.Name }
	p.applyDecision(req, decision)
	if acquired != variant.Name {
		t.Fatalf("AcquireContainer got %q, want %q", acquired, variant.Name)
	}
	if executed != base.Name {
		t.Fatalf("logical request function = %q, want %q", executed, base.Name)
	}
	if req.Fun.Name != base.Name {
		t.Fatalf("request function mutated to %q, want %q", req.Fun.Name, base.Name)
	}
	if req.executionFunction().Name != variant.Name {
		t.Fatalf("execution target = %q, want %q", req.executionFunction().Name, variant.Name)
	}
	req.markPhysicalExecutionTarget(req.executionFunction())
	if req.IsDefault {
		t.Fatal("isDefault = true, want false for EXEC_VAR physical variant")
	}
}

func TestCo2QosExplicitVariantInvocationIsLocalOnlyAndDoesNotRecurse(t *testing.T) {
	variant := testPolicyFunction("base-fast")
	variant.IsDefault = false
	variant.DefaultFunction = "base"
	variant.SpeedUp = 2
	variant.Utility = 0.9
	p := testPolicy(t, variant)
	storeTestSnapshot(p, map[string]MultiRegionProbs{"base-fast|standard": {
		PLocalVar: 1,
		PEdge:     1,
		PCloud:    map[string]float64{"cloud_a": 1},
	}}, nil)

	decision, err := p.evaluate(testScheduledRequest(variant), "base-fast|standard")
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if decision.action != DROP {
		t.Fatalf("decision = %#v, want DROP after recursive/offload probabilities are stripped", decision)
	}
	req := testScheduledRequest(variant)
	req.markPhysicalExecutionTarget(req.executionFunction())
	if req.IsDefault {
		t.Fatal("isDefault = true, want false for explicit variant invocation")
	}
}

func TestExecutionReportMarksDefaultPhysicalTarget(t *testing.T) {
	base := testPolicyFunction("base")
	req := testScheduledRequest(base)

	req.markPhysicalExecutionTarget(req.executionFunction())

	if !req.IsDefault {
		t.Fatal("isDefault = false, want true for default local execution")
	}
	if req.Fun.Name != base.Name {
		t.Fatalf("logical function changed to %q", req.Fun.Name)
	}
}

func TestExecutionReportTreatsLegacyFunctionAsDefault(t *testing.T) {
	legacy := testPolicyFunction("legacy")
	legacy.IsDefault = false
	legacy.DefaultFunction = ""
	req := testScheduledRequest(legacy)

	req.markPhysicalExecutionTarget(req.executionFunction())

	if !req.IsDefault {
		t.Fatal("isDefault = false, want true for ordinary legacy function without variant metadata")
	}
}

func TestExecutionReportMarksOffloadedDefaultPhysicalTarget(t *testing.T) {
	base := testPolicyFunction("base")
	req := testScheduledRequest(base)
	req.ExecutionReport = &function.ExecutionReport{Result: "remote"}

	req.markPhysicalExecutionTarget(req.Fun)

	if !req.IsDefault {
		t.Fatal("isDefault = false, want true for offloaded default function")
	}
}

func TestCo2QosRejectsInvalidOptimizerVariantResponses(t *testing.T) {
	base := testPolicyFunction("base")
	valid := testPolicyFunction("base-fast")
	valid.IsDefault = false
	valid.DefaultFunction = base.Name
	valid.SpeedUp = 2
	valid.Utility = 0.9
	wrong := testPolicyFunction("other-fast")
	wrong.IsDefault = false
	wrong.DefaultFunction = "other"
	wrong.SpeedUp = 2
	wrong.Utility = 0.9
	arm := testPolicyFunction("base-arm")
	arm.IsDefault = false
	arm.DefaultFunction = base.Name
	arm.SpeedUp = 2
	arm.Utility = 0.9
	arm.SupportedArchs = []string{"arm64"}
	huge := testPolicyFunction("base-huge")
	huge.IsDefault = false
	huge.DefaultFunction = base.Name
	huge.SpeedUp = 2
	huge.Utility = 0.9
	huge.MemoryMB = 999999
	functions := map[string]*function.Function{
		base.Name:  base,
		valid.Name: valid,
		wrong.Name: wrong,
		arm.Name:   arm,
		huge.Name:  huge,
	}
	p := testPolicy(t, base)
	p.listFunctions = func() ([]string, error) {
		return []string{base.Name, valid.Name, wrong.Name, arm.Name, huge.Name}, nil
	}
	p.getFunction = func(name string) (*function.Function, bool) {
		f, ok := functions[name]
		return f, ok
	}

	tests := []struct {
		name string
		body string
	}{
		{name: "missing variant probabilities", body: `{"probs":{"base":{"standard":{"EXEC_VAR":1}}},"decision_fc_probability_var":{}}`},
		{name: "unknown variant", body: `{"probs":{"base":{"standard":{"EXEC_VAR":1}}},"decision_fc_probability_var":{"base":{"missing":{"standard":1}}}}`},
		{name: "wrong default", body: `{"probs":{"base":{"standard":{"EXEC_VAR":1}}},"decision_fc_probability_var":{"base":{"other-fast":{"standard":1}}}}`},
		{name: "incompatible arch", body: `{"probs":{"base":{"standard":{"EXEC_VAR":1}}},"decision_fc_probability_var":{"base":{"base-arm":{"standard":1}}}}`},
		{name: "insufficient memory", body: `{"probs":{"base":{"standard":{"EXEC_VAR":1}}},"decision_fc_probability_var":{"base":{"base-huge":{"standard":1}}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})}
			if err := p.invokeOptimizer(initOptCarbonAwareParams()); err == nil {
				t.Fatal("invokeOptimizer() error = nil")
			}
		})
	}
}

func TestCo2QosOptimizerSnapshotReplacementClearsStaleVariantProbabilities(t *testing.T) {
	base := testPolicyFunction("base")
	variant := testPolicyFunction("base-fast")
	variant.IsDefault = false
	variant.DefaultFunction = base.Name
	variant.SpeedUp = 2
	variant.Utility = 0.9
	functions := map[string]*function.Function{base.Name: base, variant.Name: variant}
	p := testPolicy(t, base)
	p.listFunctions = func() ([]string, error) { return []string{base.Name, variant.Name}, nil }
	p.getFunction = func(name string) (*function.Function, bool) {
		f, ok := functions[name]
		return f, ok
	}
	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"probs":{"base":{"standard":{"EXEC_VAR":1}}},"decision_fc_probability_var":{"base":{"base-fast":{"standard":1}}}}`)),
		}, nil
	})}
	if err := p.invokeOptimizer(initOptCarbonAwareParams()); err != nil {
		t.Fatalf("first invokeOptimizer() error = %v", err)
	}
	if len(p.loadOptimizerSnapshot().VariantProbabilities["base|standard"]) != 1 {
		t.Fatal("variant probabilities missing after first snapshot")
	}

	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"probs":{"base":{"standard":{"EXEC":1}}},"decision_fc_probability_var":{}}`)),
		}, nil
	})}
	if err := p.invokeOptimizer(initOptCarbonAwareParams()); err != nil {
		t.Fatalf("second invokeOptimizer() error = %v", err)
	}
	snapshot := p.loadOptimizerSnapshot()
	if snapshot.Probabilities["base|standard"].PLocal != 1 {
		t.Fatalf("base probabilities not replaced: %#v", snapshot.Probabilities)
	}
	if _, ok := snapshot.VariantProbabilities["base|standard"]; ok {
		t.Fatalf("stale variant probabilities not cleared: %#v", snapshot.VariantProbabilities)
	}
}

func TestCo2QosMalformedOptimizerResponseDoesNotPartiallyReplaceSnapshot(t *testing.T) {
	base := testPolicyFunction("base")
	p := testPolicy(t, base)
	storeTestSnapshot(p,
		map[string]MultiRegionProbs{"base|standard": {PLocal: 1, PCloud: map[string]float64{}}},
		map[string]map[string]float64{"base|standard": {"base-fast": 1}},
	)
	p.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"probs":{"base":{"standard":{"SIDEWAYS":1}}},"decision_fc_probability_var":{}}`)),
		}, nil
	})}
	if err := p.invokeOptimizer(initOptCarbonAwareParams()); err == nil {
		t.Fatal("invokeOptimizer() error = nil")
	}
	snapshot := p.loadOptimizerSnapshot()
	if snapshot.Probabilities["base|standard"].PLocal != 1 {
		t.Fatalf("valid base snapshot was replaced: %#v", snapshot.Probabilities)
	}
	if snapshot.VariantProbabilities["base|standard"]["base-fast"] != 1 {
		t.Fatalf("valid variant snapshot was replaced: %#v", snapshot.VariantProbabilities)
	}
}

func TestOptimizerRequestContractMatchesRealPythonOptimizer(t *testing.T) {
	pythonOptimizer := "/home/cecilia/workspace/faas-offloading/prototype_impl/qos_co2_aware_optimizer.py"
	if _, err := os.Stat(pythonOptimizer); err != nil {
		t.Skipf("real Python optimizer unavailable: %v", err)
	}

	base := testPolicyFunction("euler_light")
	variant := testPolicyFunction("func_v1")
	variant.IsDefault = false
	variant.DefaultFunction = base.Name
	variant.SpeedUp = 2
	variant.Utility = 0.8
	functions := map[string]*function.Function{
		base.Name:    base,
		variant.Name: variant,
	}
	p := testPolicy(t, base)
	p.listFunctions = func() ([]string, error) { return []string{base.Name, variant.Name}, nil }
	p.getFunction = func(name string) (*function.Function, bool) {
		f, ok := functions[name]
		return f, ok
	}
	p.getNeighborInfo = func() map[string]*registration.StatusInformation {
		return map[string]*registration.StatusInformation{
			"(edge)peer": {
				AvailableMemory:            1024,
				TotalCPU:                   1,
				ProcessingPowerConsumption: 110,
				TxEnergyConsumption:        0.000052,
				RxEnergyConsumption:        0.000031,
			},
		}
	}
	p.getMetrics = func() metrics.RetrievedMetrics {
		local := node.LocalNode.String()
		return metrics.RetrievedMetrics{
			AvgInputSize:  map[string]float64{base.Name: 128, variant.Name: 64},
			AvgOutputSize: map[string]float64{base.Name: 16, variant.Name: 8},
			AvgEdgeCPUUsage: map[string]map[string]float64{
				local: {base.Name: 42},
			},
			AvgCloudRegionCPUUsage: map[string]map[string]float64{
				"cloud_a": {base.Name: 52, variant.Name: 53},
				"cloud_b": {base.Name: 62, variant.Name: 63},
			},
		}
	}
	p.arrivalRates = map[string]float64{
		base.Name + "|standard":    0.2,
		variant.Name + "|standard": 0.1,
	}

	params, err := p.prepareOptimizerParams()
	if err != nil {
		t.Fatalf("prepareOptimizerParams() error = %v", err)
	}
	assertPhase1OptimizerContract(t, params)
	assertVariantOptimizerContract(t, params, base.Name, variant.Name)

	payload, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal optimizer params: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if _, ok := raw["variants"]; !ok {
		t.Fatal("payload missing variants field")
	}
	if _, ok := raw["variant_utility"]; !ok {
		t.Fatal("payload missing variant_utility field")
	}

	script := `
import sys
from prototype_impl.qos_co2_aware_optimizer import OptCarbonAwareProblemParams, MyLPOptimizer
params = OptCarbonAwareProblemParams.from_json(sys.stdin.read())
for f in params.functions:
    params.cpu_usage_local[f]
    params.cpu_usage_edge[f]
    for region in params.cloud_regions:
        params.cpu_usage_cloud[region][f]
    for v in params.variants.get(f, []):
        params.variant_utility[v]
        params.function_memory[v]
        params.serv_time_local[v]
        params.init_time_local[v]
        params.cold_start_p_local[v]
        params.cpu_usage_local[v]
result, variants = MyLPOptimizer(params).solve()
if isinstance(result, str):
    raise SystemExit(result)
if not variants:
    raise SystemExit("variant probabilities missing for phase 2")
print("ok")
`
	python := "/home/cecilia/workspace/faas-offloading/venv/bin/python"
	if _, err := os.Stat(python); err != nil {
		python = "python3"
	}
	cmd := exec.Command(python, "-c", script)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"PULP_SOLVER=PULP_CBC_CMD",
		"PYTHONPATH=/home/cecilia/workspace/faas-offloading",
	)
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real Python optimizer rejected payload: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("real Python optimizer output missing ok:\n%s", out)
	}
}

func assertVariantOptimizerContract(t *testing.T, params OptCarbonAwareParams, baseName, variantName string) {
	t.Helper()
	for _, fn := range params.Functions {
		if fn == variantName {
			t.Fatalf("variant %q included as top-level function", variantName)
		}
	}
	if got := params.Variants[baseName]; !reflect.DeepEqual(got, []string{variantName}) {
		t.Fatalf("Variants[%s] = %#v", baseName, got)
	}
	if params.VariantUtility[variantName] != 0.8 {
		t.Fatalf("VariantUtility[%s] = %v", variantName, params.VariantUtility[variantName])
	}
	for name, ok := range map[string]bool{
		"function_memory":    params.FunctionMemory[variantName] > 0,
		"serv_time_local":    params.ServTimeLocal[variantName] > 0,
		"init_time_local":    params.InitTimeLocal[variantName] > 0,
		"cold_start_p_local": params.ColdStartPLocal[variantName] > 0,
		"cpu_usage_local":    params.CPUUsageLocal[variantName] > 0,
		"input_size_mean":    params.FunctionInputSizeMean[variantName] > 0,
		"output_size_mean":   params.FunctionOutputSizeMean[variantName] > 0,
	} {
		if !ok {
			t.Fatalf("%s missing or invalid for variant %s", name, variantName)
		}
	}
	if _, ok := params.ServTimeCloud["cloud_a"][variantName]; ok {
		t.Fatalf("variant %s included in cloud timing map", variantName)
	}
}

func assertPhase1OptimizerContract(t *testing.T, params OptCarbonAwareParams) {
	t.Helper()
	requiredFunctionMaps := map[string]int{
		"serv_time_local":           len(params.ServTimeLocal),
		"init_time_local":           len(params.InitTimeLocal),
		"cold_start_p_local":        len(params.ColdStartPLocal),
		"cpu_usage_local":           len(params.CPUUsageLocal),
		"serv_time_edge":            len(params.ServTimeEdge),
		"init_time_edge":            len(params.InitTimeEdge),
		"cold_start_p_edge":         len(params.ColdStartPEdge),
		"cpu_usage_edge":            len(params.CPUUsageEdge),
		"function_memory":           len(params.FunctionMemory),
		"function_input_size_mean":  len(params.FunctionInputSizeMean),
		"function_output_size_mean": len(params.FunctionOutputSizeMean),
	}
	for name, got := range requiredFunctionMaps {
		if got < len(params.Functions) {
			t.Fatalf("%s has %d functions, want at least %d", name, got, len(params.Functions))
		}
	}
	for _, fn := range params.Functions {
		if _, ok := params.CPUUsageLocal[fn]; !ok {
			t.Fatalf("cpu_usage_local missing %s", fn)
		}
		if _, ok := params.CPUUsageEdge[fn]; !ok {
			t.Fatalf("cpu_usage_edge missing %s", fn)
		}
		for _, class := range params.Classes {
			if _, ok := params.ArrivalRates[fn][class]; !ok {
				t.Fatalf("arrival_rates missing %s/%s", fn, class)
			}
		}
	}
	for region := range params.CloudRegions {
		if _, ok := params.BandwidthCloud[region]; !ok {
			t.Fatalf("bandwidth_cloud missing %s", region)
		}
		if _, ok := params.OffloadTimeCloud[region]; !ok {
			t.Fatalf("offload_time_cloud missing %s", region)
		}
		cloudMaps := map[string]map[string]float64{
			"serv_time_cloud":    params.ServTimeCloud[region],
			"init_time_cloud":    params.InitTimeCloud[region],
			"cold_start_p_cloud": params.ColdStartPCloud[region],
			"cpu_usage_cloud":    params.CPUUsageCloud[region],
		}
		for name, byFunction := range cloudMaps {
			if len(byFunction) != len(params.Functions) {
				t.Fatalf("%s[%s] has %d functions, want %d", name, region, len(byFunction), len(params.Functions))
			}
			for _, fn := range params.Functions {
				if _, ok := byFunction[fn]; !ok {
					t.Fatalf("%s[%s] missing %s", name, region, fn)
				}
			}
		}
	}
}

func TestCo2QosTypesHaveNoAWSExternalProviderFields(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(OptCarbonAwareParams{}), reflect.TypeOf(schedDecision{})} {
		if _, ok := typ.FieldByName("ExternalProvider"); ok {
			t.Fatalf("%s has ExternalProvider field", typ.Name())
		}
		if _, ok := typ.FieldByName("ArnCode"); ok {
			t.Fatalf("%s has ArnCode field", typ.Name())
		}
	}
}
