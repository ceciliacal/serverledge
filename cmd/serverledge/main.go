package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/serverledge-faas/serverledge/internal/api"
	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/metrics"
	"github.com/serverledge-faas/serverledge/internal/node"
	"github.com/serverledge-faas/serverledge/internal/registration"
	"github.com/serverledge-faas/serverledge/internal/scheduling"
	"github.com/serverledge-faas/serverledge/internal/telemetry"
	"github.com/serverledge-faas/serverledge/internal/workflow"
)

func main() {
	configFileName := ""
	if len(os.Args) > 1 {
		configFileName = os.Args[1]
	}
	config.ReadConfiguration(configFileName)

	// === carbon aware config setup ===
	configCo2TraceFileName := ""
	if len(os.Args) > 2 {
		fmt.Println("ciao")

		configCo2TraceFileName = os.Args[2]
	}

	co2TracesFilename, co2Timestamp, co2Intensity, pollInterval, err := node.ReadCO2Configuration(configCo2TraceFileName)
	if err != nil {
		log.Printf("Skipping CO2 monitoring: %v\n", err)
	} else {
		//todo: setup config energy info in node
		if err := startCO2Monitoring(co2TracesFilename, co2Timestamp, co2Intensity, pollInterval, nil); err != nil {
			log.Fatal(err)
		}
		fmt.Println("CO2 poller started")
	}

	//setting up cache parameters
	api.CacheSetup()

	// register to etcd, this way server is visible to the others under a given local area
	myArea := config.GetString(config.REGISTRY_AREA, "ROME")
	node.LocalNode = node.NewIdentifier(myArea)

	err = registration.RegisterNode()
	if err != nil {
		log.Fatal(err)
	}

	metrics.Init()

	if config.GetBool(config.TRACING_ENABLED, false) {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		tracesOutfile := config.GetString(config.TRACING_OUTFILE, "")
		if len(tracesOutfile) < 1 {
			tracesOutfile = fmt.Sprintf("traces-%s.json", time.Now().Format("20060102-150405"))
		}
		log.Printf("Enabling tracing to %s\n", tracesOutfile)
		otelShutdown, err := telemetry.SetupOTelSDK(ctx, tracesOutfile)
		if err != nil {
			log.Fatal(err)
		}
		// Handle shutdown properly so nothing leaks.
		defer func() {
			err = errors.Join(err, otelShutdown(context.Background()))
		}()
	}

	e := echo.New()

	// Register a signal handler to cleanup things on termination
	api.RegisterTerminationHandler(e)

	// Function scheduling policy
	schedulingPolicy := api.CreateSchedulingPolicy()
	go scheduling.Run(schedulingPolicy)

	// Workflow offloading policy
	workflow.CreateOffloadingPolicy()

	err = registration.StartMonitoring()
	if err != nil {
		log.Fatal(err)
	}

	api.StartAPIServer(e)

}

func startCO2Monitoring(
	path, tsHeader, valHeader string,
	period time.Duration,
	loc *time.Location,
) error {
	poller := &node.CO2Poller{}

	if err := poller.Start(path, tsHeader, valHeader, period, loc); err != nil {
		return err
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			t, intensity, count := poller.CarbonFootprint().Snapshot()
			now := time.Now().UTC().Format(time.RFC3339)
			log.Printf("CO2 snapshot now=%s t=%s intensity=%.3f count=%d",
				now, t.Format(time.RFC3339), intensity, count)
		}
	}()

	return nil
}
