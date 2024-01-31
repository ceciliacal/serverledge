package energy

import (
	"bytes"
	"fmt"
	_ "fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	batteryCapacity   float64 //Ah
	voltage           float64 //voltage
	SoC               float64 //estimated State of Charge
	batteryCapacityWh float64 //Wh
	//Watt = Volt * Ampere
	//joule = volt * ampere * secondi
)

type Battery struct {
	Value float64
	Mu    sync.Mutex
}

var MyBattery = &Battery{}

func Init() {
	// config battery capacity

	batteryCapacity = 3.2 //capacita batteria viene misurata in Ah
	voltage = 3.7         //volt
	SoC = 100.0
	batteryCapacityWh = batteryCapacity * voltage

	getBattery()

}

func getBattery() {

	count := 0
	prevRaplWh := 0.0
	CWh := batteryCapacityWh
	for {

		time.Sleep(60 * time.Second)

		raplUj := readRAPL()
		currRaplWh := raplUj / (1000000.0 * 3600)

		if count == 0 {
			prevRaplWh = currRaplWh
			count = 1
			continue
		}

		if prevRaplWh > currRaplWh {
			fmt.Println("ALERT! \n")
			continue
		}

		diffWh := currRaplWh - prevRaplWh
		CWh = CWh - diffWh
		batteryPercentage := (CWh / batteryCapacityWh) * 100.0

		MyBattery.Mu.Lock()
		MyBattery.Value = batteryPercentage
		MyBattery.Mu.Unlock()

		prevRaplWh = currRaplWh

		log.Println("======= ", time.Now().Format("2006-01-02 15:04:05"), " - RAPL [uJ]: ", raplUj, " - batteryPercentage: ", batteryPercentage, "\n\n")

	}

}

func readRAPL() float64 {
	command := "sudo cat /sys/class/powercap/intel-rapl/intel-rapl:0/energy_uj"
	output, err := RunCMD(command)
	if err != nil {
		log.Fatal("Error in reading RAPL:", err.Error(), "\nOutput :", string(output))
	}

	resStr := string(output[:])
	res, _ := strconv.ParseFloat(strings.TrimSpace(resStr), 64)
	return res
}

// RunCMD is a simple wrapper around terminal commands
func RunCMD(command string) ([]byte, error) {
	var buf bytes.Buffer
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Start()
	if err != nil {
		return buf.Bytes(), err
	}
	_, err = cmd.Process.Wait()
	return buf.Bytes(), err
}
