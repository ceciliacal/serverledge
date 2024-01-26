package scheduling

import (
	"github.com/grussorusso/serverledge/energy"
	"github.com/grussorusso/serverledge/internal/node"
	"log"
)

type EnergyAwarePolicy struct{}

func (p *EnergyAwarePolicy) Init() {
}

func (p *EnergyAwarePolicy) OnCompletion(r *scheduledRequest) {
}

func (p *EnergyAwarePolicy) OnArrival(r *scheduledRequest) {
	myBattery := energy.MyBattery

	myBattery.Mu.Lock()
	batteryValue := myBattery.Value
	myBattery.Mu.Unlock()

	if batteryValue > 20.0 {
		log.Println("Battery > 20% -> executing request locally ")
		containerID, err := node.AcquireWarmContainer(r.Fun)

		if err == nil {
			log.Println("Using a warm container for: ", r)
			execLocally(r, containerID, true)
		} else {
			log.Println("Handling cold start for: ", r)
			if !handleColdStart(r) {
				dropRequest(r)
			}
		}
	} else { //battery is low
		//offload to another edge node
		log.Println("Low battery -> offloading request ")
		if r.CanDoOffloading {
			url := pickEdgeNodeForOffloading(r)
			if url != "" {
				handleOffload(r, url)
				return
			}
		}

		//if the request cannot be offloaded, then it's dropped
		log.Println("Request cannot be offloaded -> dropping request ")
		dropRequest(r)
	}
}
