package scheduling

import (
	"fmt"
	"github.com/grussorusso/serverledge/internal/energy"
	"time"

	"github.com/grussorusso/serverledge/internal/container"
	"github.com/grussorusso/serverledge/internal/executor"
)

const HANDLER_DIR = "/app"

// Execute serves a request on the specified container.
func Execute(contID container.ContainerID, r *scheduledRequest) error {
	//log.Printf("[%s] Executing on container: %v", r, contID)

	context := make(map[string]interface{}) //crea il channel / dictionary
	context["SoC"] = energy.ReadBattery()
	context["isLightVariant"] = energy.IsLightVariant()

	var req executor.InvocationRequest
	if r.Fun.Runtime == container.CUSTOM_RUNTIME {
		req = executor.InvocationRequest{
			Params: r.Params,
		}
	} else {
		cmd := container.RuntimeToInfo[r.Fun.Runtime].InvocationCmd
		req = executor.InvocationRequest{
			cmd,
			r.Params,
			r.Fun.Handler,
			HANDLER_DIR,
			context,
		}
	}

	t0 := time.Now()

	response, invocationWait, err := container.Execute(contID, &req)
	if err != nil {
		// notify scheduler
		completions <- &completion{scheduledRequest: r, contID: contID}
		return fmt.Errorf("[%s] Execution failed: %v", r, err)
	}

	if !response.Success {
		// notify scheduler
		completions <- &completion{scheduledRequest: r, contID: contID}
		return fmt.Errorf("Function execution failed")
	}

	r.ExecReport.Result = response.Result
	r.ExecReport.Duration = time.Now().Sub(t0).Seconds() - invocationWait.Seconds()
	r.ExecReport.ResponseTime = time.Now().Sub(r.Arrival).Seconds()

	// initializing containers may require invocation retries, adding
	// latency
	r.ExecReport.InitTime += invocationWait.Seconds()

	r.ExecReport.SoC = energy.ReadBattery()

	// notify scheduler
	completions <- &completion{scheduledRequest: r, contID: contID}

	return nil
}
