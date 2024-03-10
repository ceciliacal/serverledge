package scheduling

import (
	"github.com/grussorusso/serverledge/internal/energy"
	"github.com/grussorusso/serverledge/internal/node"
	"log"
)

type EnergyAwarePolicy struct{}

func (p *EnergyAwarePolicy) Init() {
}

func (p *EnergyAwarePolicy) OnCompletion(r *scheduledRequest) {
}

func (p *EnergyAwarePolicy) OnArrival(r *scheduledRequest) {

	batteryValue := energy.ReadBattery()
	r.Params["SoC"] = batteryValue

	if batteryValue <= 1.0 {
		//shut down
		log.Println("Battery <= 1.0 > -> dropping request ")
		nodeShutdown(r)
	} else if batteryValue > 20.0 {
		//log.Println("Battery > 20% -> executing request locally ")
		containerID, err := node.AcquireWarmContainer(r.Fun)

		if err == nil {
			//log.Println("Using a warm container for: ", r)
			execLocally(r, containerID, true)
		} else {
			//log.Println("Handling cold start for: ", r)
			if !handleColdStart(r) {
				dropRequest(r)
			}
		}
	} else { //battery is low
		//offload to another edge node
		log.Println("Low battery")
		if r.CanDoOffloading {
			url := pickEdgeNodeForOffloadingEnergyAware(r)
			if url != "" {
				log.Println("Offloading request to another Edge node")
				handleOffload(r, url)
				return
			} else { //offload to cloud if no suitable edge node is found
				log.Println("Offloading request to Cloud")
				handleCloudOffload(r)
				return
			}
		}

		//if the request cannot be offloaded, then it's dropped
		log.Println("Request cannot be offloaded due to low battery -> dropping request ")
		dropRequest(r)
	}
}
