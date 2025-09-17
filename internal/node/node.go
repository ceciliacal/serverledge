package node

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/lithammer/shortuuid"
)

var OutOfResourcesErr = errors.New("not enough resources for function execution")

type NodeID struct {
	Area string
	Key  string
}

var LocalNode NodeID

func (n NodeID) String() string {
	return fmt.Sprintf("(%s)%s", n.Area, n.Key)
}

func NewIdentifier(area string) NodeID {
	id := shortuuid.New() + strconv.FormatInt(time.Now().UnixNano(), 10)
	return NodeID{Area: area, Key: id}
}

type NodeResources struct {
	sync.RWMutex
	AvailableMemMB             int64
	UsedMemMB                  int64 // memory occupied by busy containers
	AvailableCPUs              float64
	DropCount                  int64
	ContainerPools             map[string]*ContainerPool
	Co2Footprint               CarbonFootprint
	ProcessingPowerConsumption float64
	TxEnergyConsumption        float64
	RxEnergyConsumption        float64
	GCo2Emissions              float64
}

func (n *NodeResources) String() string {
	return fmt.Sprintf("[CPUs: %f - Mem: %d]", n.AvailableCPUs, n.AvailableMemMB)
}

var Resources NodeResources
