package scheduling

import "github.com/serverledge-faas/serverledge/internal/function"

type OffloadingDecision struct {
	Offload    bool   `json:"offload"`
	RemoteHost string `json:"remote_host"`
}

type OffloadingPolicy interface {
	Evaluate(r *function.Request) (OffloadingDecision, error)
}
