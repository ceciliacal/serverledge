package lb

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/registration"
)

var currentTargets []*middleware.ProxyTarget

func newBalancer(targets []*middleware.ProxyTarget) middleware.ProxyBalancer {
	return middleware.NewRoundRobinBalancer(targets)
}

// AreaStat holds averages for the LB's area.
type AreaStat struct {
	AvgAvailableMem float64   `json:"avg_available_mem"` // bytes
	CO2             float64   `json:"co2_intensity"`     // gCO2/kWh (or your unit)
	NodeCount       int       `json:"node_count"`
	UpdatedAt       time.Time `json:"updated_at"`

	ProcessingPowerConsumption float64 `mapstructure:"consumption.processing.power"`
	TxEnergyConsumption        float64 `mapstructure:"consumption.energy.tx"`
	RxEnergyConsumption        float64 `mapstructure:"consumption.energy.rx"`
	Cost                       float64 `mapstructure:"cost"`
}

var (
	statsMu  sync.RWMutex
	areaStat AreaStat
)

func StartReverseProxy(e *echo.Echo, region string) {
	targets, err := getTargets(region)
	if err != nil {
		log.Printf("Cannot connect to registry to retrieve targets: %v\n", err)
		os.Exit(2)
	}

	log.Printf("Initializing with %d targets.\n", len(targets))
	rr := newBalancer(targets)
	currentTargets = targets

	// --- Local (non-proxied) endpoints first ---
	startStatsPolling(region)
	e.GET("/lb/stats", func(c echo.Context) error {
		statsMu.RLock()
		defer statsMu.RUnlock()
		return c.JSON(http.StatusOK, areaStat)
	})

	// --- Proxy ONLY /invoke/* to the processing nodes ---
	invoke := e.Group("/invoke")
	invoke.Use(middleware.Proxy(rr))

	// keep dynamic target updates
	go updateTargets(rr, region)

	// start server
	portNumber := config.GetInt(config.API_PORT, 1329)
	if err := e.Start(fmt.Sprintf(":%d", portNumber)); err != nil && !errors.Is(err, http.ErrServerClosed) {
		e.Logger.Fatal("shutting down the server")
	}
}

func startStatsPolling(region string) {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			nodes, err := registration.GetNodesInArea(region, false, 0)
			if err != nil {
				log.Printf("LB: stats poll: get nodes: %v", err)
				continue
			}

			var sumAvail int64
			var co2Sum float64
			var procPowerSum float64
			var txEnergySum float64
			var rxEnergySum float64
			var n int

			for _, registeredNode := range nodes {
				info, _ := registration.GetStatus(&registeredNode) // UDP status
				if info == nil {
					continue
				}
				avail := info.TotalMemory - info.UsedMemory
				sumAvail += avail
				co2Sum += info.CO2Intensity
				procPowerSum += info.ProcessingPowerConsumption
				txEnergySum += info.TxEnergyConsumption
				rxEnergySum += info.RxEnergyConsumption

				n++
			}

			now := time.Now()
			statsMu.Lock()
			if n == 0 {
				// no nodes reachable
				areaStat = AreaStat{
					AvgAvailableMem:            0,
					CO2:                        0,
					NodeCount:                  0,
					UpdatedAt:                  now,
					ProcessingPowerConsumption: 0,
					TxEnergyConsumption:        0,
					RxEnergyConsumption:        0,
				}
			} else {
				areaStat = AreaStat{
					AvgAvailableMem:            float64(sumAvail) / float64(n),
					CO2:                        co2Sum / float64(n),
					NodeCount:                  n,
					UpdatedAt:                  now,
					ProcessingPowerConsumption: procPowerSum / float64(n),
					TxEnergyConsumption:        txEnergySum / float64(n),
					RxEnergyConsumption:        rxEnergySum / float64(n),
				}
			}
			statsMu.Unlock()
		}
	}()
}

func getTargets(region string) ([]*middleware.ProxyTarget, error) {
	cloudNodes, err := registration.GetNodesInArea(region, false, 0)
	if err != nil {
		return nil, err
	}

	targets := make([]*middleware.ProxyTarget, 0, len(cloudNodes))
	for _, target := range cloudNodes {
		log.Printf("Found target: %v\n", target.Key)
		// TODO: etcd should NOT contain URLs, but only host and port...
		parsedUrl, err := url.Parse(target.APIUrl())
		if err != nil {
			return nil, err
		}
		targets = append(targets, &middleware.ProxyTarget{Name: target.Key, URL: parsedUrl})
	}

	log.Printf("Found %d targets\n", len(targets))

	return targets, nil
}

func updateTargets(balancer middleware.ProxyBalancer, region string) {
	for {
		time.Sleep(30 * time.Second) // TODO: configure

		targets, err := getTargets(region)
		if err != nil {
			log.Printf("Cannot update targets: %v\n", err)
		}

		toKeep := make([]bool, len(currentTargets))
		for i := range currentTargets {
			toKeep[i] = false
		}
		for _, t := range targets {
			toAdd := true
			for i, curr := range currentTargets {
				if curr.Name == t.Name {
					toKeep[i] = true
					toAdd = false
				}
			}
			if toAdd {
				log.Printf("Adding %s\n", t.Name)
				balancer.AddTarget(t)
			}
		}

		toRemove := make([]string, 0)
		for i, curr := range currentTargets {
			if !toKeep[i] {
				log.Printf("Removing %s\n", curr.Name)
				toRemove = append(toRemove, curr.Name)
			}
		}
		for _, curr := range toRemove {
			balancer.RemoveTarget(curr)
		}

		currentTargets = targets
	}
}
