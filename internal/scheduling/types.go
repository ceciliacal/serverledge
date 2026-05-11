package scheduling

import (
	"github.com/serverledge-faas/serverledge/internal/container"
	"github.com/serverledge-faas/serverledge/internal/function"
)

// scheduledRequest represents a Request within the scheduling subsystem
type scheduledRequest struct {
	*function.Request
	*function.ExecutionReport
	offloaded          bool
	onExternalProvider bool
	decisionChannel    chan schedDecision

	// For gCO2 calculation after execution: initial (origin) node profile
	initialNodeTxEnergy float64
	initialNodeRxEnergy float64
	initialNodeMemory   float64
	QoSClass
}

type completionNotification struct {
	failed bool
	r      *scheduledRequest
	cont   *container.Container
}

// schedDecision wraps a action made by the scheduler.
// Possible decisions are 1) drop, 2) execute locally or 3) execute on a remote
// Node (offloading).
type schedDecision struct {
	action     action
	cont       *container.Container
	remoteHost string
	useWarm    bool
	regionName string

	variantName string
}

type action int64

const (
	DROP               action = 0
	EXEC_LOCAL                = 1
	EXEC_REMOTE               = 2
	EXEC_LOCAL_VARIANT        = 3
)
