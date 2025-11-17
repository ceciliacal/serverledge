package scheduling

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/serverledge-faas/serverledge/internal/emissions"
	"github.com/serverledge-faas/serverledge/internal/externalprovider"
	"github.com/serverledge-faas/serverledge/internal/externalprovider/lambda/utils"
	"github.com/serverledge-faas/serverledge/internal/registration"

	"github.com/serverledge-faas/serverledge/internal/container"
	"github.com/serverledge-faas/serverledge/internal/function"
	"github.com/serverledge-faas/serverledge/internal/metrics"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/telemetry"

	"go.opentelemetry.io/otel/trace"
)

var requests chan *scheduledRequest
var completions chan *completionNotification

var offloadingClient *http.Client

func Run(p Policy) {
	requests = make(chan *scheduledRequest, 500)
	completions = make(chan *completionNotification, 500)

	node.LocalResources.Init()
	log.Printf("Current resources: %v\n", &node.LocalResources)

	container.InitDockerContainerFactory()

	//janitor periodically remove expired warm container
	node.GetJanitorInstance()

	tr := &http.Transport{
		MaxIdleConns:        2500,
		MaxIdleConnsPerHost: 2500,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     30 * time.Minute,
	}
	offloadingClient = &http.Client{Transport: tr}

	// initialize scheduling policy
	p.Init()

	log.Println("Scheduler started.")

	var r *scheduledRequest
	var c *completionNotification
	for {
		select {
		case r = <-requests: // receive request
			prepareInitialNodeEnergyProfile(r) //for gCO2 metrics computation
			go p.OnArrival(r)
		case c = <-completions:
			if c.cont != nil {
				node.HandleCompletion(c.cont, c.r.Fun)
			}
			p.OnCompletion(c.r.Fun, c.r.ExecutionReport)

			if metrics.Enabled && !c.failed && c.r.ExecutionReport != nil {

				if c.r.onExternalProvider {
					provider, err := externalprovider.NewOffloader("aws")
					if err != nil {
						log.Printf("Errore nel recupero del provider: %v", err)
					} else {
						extPrvRegion, err := provider.GetRegion()
						if err != nil {
							log.Printf("Errore nel recupero della regione del provider: %v", err)
						} else {
							nodeArea := utils.ExternalProvider + extPrvRegion
							metrics.AddRemoteCompletedInvocation(c.r.Fun.Name, nodeArea, !c.r.ExecutionReport.IsWarmStart)
							metrics.AddRemoteFunctionDurationValue(c.r.Fun.Name, nodeArea, c.r.ExecutionReport.Duration)
							if !c.r.ExecutionReport.IsWarmStart {
								metrics.AddRemoteFunctionInitTimeValue(c.r.Fun.Name, nodeArea, c.r.ExecutionReport.InitTime)
							}
						}
					}
				} else {
					metrics.AddCompletedInvocation(c.r.Fun.Name, !c.r.ExecutionReport.IsWarmStart)

					in := prepareEnergyInputs(c.r)
					co2g := emissions.Compute(in)
					metrics.AddFunctionCO2Emitted(c.r.Fun.Name, co2g)

					if !c.r.offloaded {
						metrics.AddFunctionDurationValue(c.r.Fun.Name, c.r.ExecutionReport.Duration)
						metrics.AddFunctionDurationValueArea(c.r.Fun.Name, c.r.ExecutionReport.Duration) // NEW

						if !c.r.ExecutionReport.IsWarmStart {
							metrics.AddFunctionInitTimeValue(c.r.Fun.Name, c.r.ExecutionReport.InitTime)
							metrics.AddFunctionInitTimeValueArea(c.r.Fun.Name, c.r.ExecutionReport.InitTime) // NEW

						}
					}
				}
				outputSize := len(c.r.ExecutionReport.Result)
				metrics.AddFunctionOutputSizeValue(c.r.Fun.Name, float64(outputSize))

				jsonParams, err := json.Marshal(c.r.Params)
				if err != nil {
					log.Printf("Impossibile serializzare i parametri per la funzione '%s': %v", c.r.Fun.Name, err)
				} else {
					inputSizeBytes := len(jsonParams)
					metrics.AddFunctionInputSizeValue(c.r.Fun.Name, float64(inputSizeBytes))
				}
			}
		}
	}
}

// SubmitRequest submits a newly arrived request for scheduling and execution
func SubmitRequest(r *function.Request) (*function.ExecutionReport, error) {
	schedRequest := scheduledRequest{
		Request:         r,
		ExecutionReport: &function.ExecutionReport{},
		decisionChannel: make(chan schedDecision, 1)}
	requests <- &schedRequest

	if telemetry.DefaultTracer != nil {
		trace.SpanFromContext(r.Ctx).AddEvent("Scheduling start")
	}

	// wait on channel for scheduling action
	schedDecision, ok := <-schedRequest.decisionChannel
	if !ok {
		return nil, fmt.Errorf("could not schedule the request")
	}
	//log.Printf("[%s] Scheduling decision: %v", r, schedDecision)

	if telemetry.DefaultTracer != nil {
		trace.SpanFromContext(r.Ctx).AddEvent("Scheduling complete")
	}

	if schedDecision.action == DROP {
		//log.Printf("[%s] Dropping request", r)
		return nil, node.OutOfResourcesErr
	} else if schedDecision.action == EXEC_REMOTE {
		//log.Printf("Offloading request")
		err := Offload(&schedRequest, schedDecision.remoteHost)
		return schedRequest.ExecutionReport, err
	} else {
		err := Execute(schedDecision.cont, &schedRequest, schedDecision.useWarm)
		return schedRequest.ExecutionReport, err
	}
}

// SubmitAsyncRequest submits a newly arrived async request for scheduling and execution
func SubmitAsyncRequest(r *function.Request) {
	schedRequest := scheduledRequest{
		Request:         r,
		ExecutionReport: &function.ExecutionReport{},
		decisionChannel: make(chan schedDecision, 1)}
	requests <- &schedRequest // send async request

	// wait on channel for scheduling action
	schedDecision, ok := <-schedRequest.decisionChannel
	if !ok {
		publishAsyncResponse(r.Id(), function.Response{Success: false})
		return
	}

	var err error
	if schedDecision.action == DROP {
		publishAsyncResponse(r.Id(), function.Response{Success: false})
	} else if schedDecision.action == EXEC_REMOTE {
		//log.Printf("Offloading request\n")
		err = OffloadAsync(r, schedDecision.remoteHost)
		if err != nil {
			publishAsyncResponse(r.Id(), function.Response{Success: false})
		}
	} else {
		err = Execute(schedDecision.cont, &schedRequest, schedDecision.useWarm)
		if err != nil {
			publishAsyncResponse(r.Id(), function.Response{Success: false})
			return
		}
		publishAsyncResponse(r.Id(), function.Response{Success: true, ExecutionReport: *schedRequest.ExecutionReport})
	}
}

func dropRequest(r *scheduledRequest) {
	r.decisionChannel <- schedDecision{action: DROP}
}

func execLocally(r *scheduledRequest, c *container.Container, warmStart bool) {
	decision := schedDecision{action: EXEC_LOCAL, cont: c, useWarm: warmStart}
	r.decisionChannel <- decision
}

func handleOffload(r *scheduledRequest, serverHost string) {
	r.CanDoOffloading = false // the next server can't offload this request
	r.decisionChannel <- schedDecision{
		action:     EXEC_REMOTE,
		cont:       nil,
		remoteHost: serverHost,
	}
}

func handleCloudOffload(r *scheduledRequest) {
	offloadingTarget := registration.GetRemoteOffloadingTarget()
	if offloadingTarget == nil {
		log.Printf("No remote offloading target available; dropping request")
		r.decisionChannel <- schedDecision{action: DROP}
	} else {
		handleOffload(r, offloadingTarget.APIUrl())
	}
}

// Func for handling requests to AWS Lambda
func handleLambdaOffload(r *scheduledRequest) {
	cloudAddress := "aws:externalprovider"
	handleOffload(r, cloudAddress)
}

// parsing initial node's offloading path infos to calculate gCO2 for function execution
func parseInitialProfile(s string) (tx float64, rx float64, mem float64, err error) {
	parts := strings.Split(s, ";")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid profile: %q", s)
	}
	tx, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid tx: %w", err)
	}
	rx, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid rx: %w", err)
	}
	mem, err = strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid mem: %w", err)
	}
	return
}

func prepareInitialNodeEnergyProfile(r *scheduledRequest) {
	if r.offloaded == false {
		//no initial node profile, cause the function has never been offloaded
		r.initialNodeTxEnergy = 0.0
		r.initialNodeRxEnergy = 0.0
		r.initialNodeMemory = 0.0
		return
	}
	v, ok := r.Params[metaProfile].(string)
	if ok && v != "" {
		if tx, rx, mem, err := parseInitialProfile(v); err == nil {
			r.initialNodeTxEnergy = tx
			r.initialNodeRxEnergy = rx
			r.initialNodeMemory = mem
			return
		} else {
			log.Printf("Error parsing initial node energy profile %q: %v", v, err)
		}
	}

}
