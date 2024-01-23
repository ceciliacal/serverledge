package energy

import (
	"bytes"
	"fmt"
	_ "fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
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

func Init() {
	// config battery capacity

	batteryCapacity = 3.2 //capacita batteria viene misurata in mAh
	voltage = 3.7         //volt
	SoC = 100.0
	batteryCapacityWh = batteryCapacity * voltage

	getBattery()

}

func getBattery() {

	for {
		time.Sleep(10 * time.Second)
		raplValue := readRAPL()

		raplValueFloat, _ := strconv.ParseFloat(strings.TrimSpace(raplValue), 64)
		raplValueJoule := raplValueFloat / 1000000.0
		raplValueWh := raplValueFloat / (1000000.0 * 3600)

		diff := batteryCapacityWh - raplValueWh
		batteryPerc := (diff / batteryCapacityWh) * 100.0

		fmt.Println("======= ", time.Now().String(), " - RAPL [uJ]: ", raplValue, ", RAPL [J]: ", raplValueJoule, ", batteryPerc: ", batteryPerc)
		fmt.Println("\n")
	}

}

func readRAPL() string {
	command := "sudo cat /sys/class/powercap/intel-rapl/intel-rapl:0/energy_uj"
	output, err := RunCMD(command)
	if err != nil {
		log.Fatal("Error in reading RAPL:", err.Error(), "\nOutput :", string(output))
	}
	res := string(output[:])
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
