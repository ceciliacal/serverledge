package scheduling

//lambdaonly_policy.go can be used by the edge nodes to always offload the request to AWS Lambda.
import (
	"github.com/serverledge-faas/serverledge/internal/function"
	"log"
)

type LambdaPolicy struct{}

func (p *LambdaPolicy) Init() {}

func (p *LambdaPolicy) OnCompletion(_ *function.Function, _ *function.ExecutionReport) {}

func (p *LambdaPolicy) OnArrival(r *scheduledRequest) {

	if r.CanDoOffloading {
		log.Printf("[LambdaPolicy] Offloading to AWS Lambda...")
		handleLambdaOffload(r)
	} else {
		log.Printf("[LambdaPolicy] Offloading not allowed, dropping.")
		dropRequest(r)
	}
}
