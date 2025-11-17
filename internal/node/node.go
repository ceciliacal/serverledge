package node

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/lithammer/shortuuid"
	"github.com/serverledge-faas/serverledge/internal/config"
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

type Resources struct {
	sync.RWMutex
	totalMemory                int64
	totalCPUs                  float64
	busyPoolUsedMem            int64   // amount of memory used by functions currently running
	warmPoolUsedMem            int64   // amount of memory used by warm containers
	usedCPUs                   float64 // number of CPU used by functions currently running
	containerPools             map[string]*ContainerPool
	Co2Footprint               CarbonFootprint
	ProcessingPowerConsumption float64
	TxEnergyConsumption        float64
	RxEnergyConsumption        float64
}

func (n *Resources) Init() {
	availableCores := runtime.NumCPU()
	n.totalCPUs = config.GetFloat(config.POOL_CPUS, float64(availableCores))
	n.totalMemory = int64(config.GetInt(config.POOL_MEMORY_MB, 1024))
	n.containerPools = make(map[string]*ContainerPool)

	n.ProcessingPowerConsumption = config.GetFloat(config.PROCESSING_POWER_CONSUMPTION, 400.0)
	n.TxEnergyConsumption = config.GetFloat(config.TX_ENERGY_CONSUMPTION, 100.0) / 1e9
	n.RxEnergyConsumption = config.GetFloat(config.RX_ENERGY_CONSUMPTION, 100.0) / 1e9
}

func (n *Resources) String() string {
	fmt.Sprintf("[CPUs: %f/%f - Mem: %d(+%d warm)/%d]", n.usedCPUs, n.totalCPUs, n.busyPoolUsedMem, n.warmPoolUsedMem, n.totalMemory)
	return fmt.Sprintf("[ProcessingPowerConsumption: %f - TxEnergyConsumption: %f - RxEnergyConsumption: %f\n]", n.ProcessingPowerConsumption, n.TxEnergyConsumption, n.RxEnergyConsumption)
}

func (n *Resources) FreeMemory() int64 {
	return n.totalMemory - n.busyPoolUsedMem - n.warmPoolUsedMem
}

// AvailableMemory returns the amount of memory that is free or reclaimable from warm containers
func (n *Resources) AvailableMemory() int64 {
	return n.totalMemory - n.busyPoolUsedMem
}

func (n *Resources) AvailableCPUs() float64 {
	return n.totalCPUs - n.usedCPUs
}

func (n *Resources) UsedMemory() int64 {
	return n.busyPoolUsedMem
}

func (n *Resources) UsedCPUs() float64 {
	return n.usedCPUs
}

func (n *Resources) TotalCPUs() float64 {
	return n.totalCPUs
}

func (n *Resources) TotalMemory() int64 {
	return n.totalMemory
}

func (n *Resources) ProcessingPower() float64 {
	return n.ProcessingPowerConsumption
}

func (n *Resources) TxEnergyPerByte() float64 {
	return n.TxEnergyConsumption
}

func (n *Resources) RxEnergyPerByte() float64 {
	return n.RxEnergyConsumption
}

var LocalResources Resources
