package energy

import (
	"bytes"
	"encoding/json"
	"fmt"
	_ "fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
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

func ReadBattery() float64 {

	MyBattery.Mu.Lock()
	batteryValue := MyBattery.Value
	MyBattery.Mu.Unlock()

	return batteryValue
}

func Init() {
	// config battery capacity

	batteryCapacity = 3.2 //capacita batteria viene misurata in Ah
	voltage = 3.7         //volt
	SoC = 100.0
	batteryCapacityWh = batteryCapacity * voltage

	getMockBattery()
	//getBattery()

}

func getMockBattery() {
	//each edge node can retrieve from mock_server a distinct random battery value between 15.0, 35.0, 55.0
	//cloud node will get 100.0 as battery value

	hostName, err := os.Hostname() //nodes identify through hostname
	if err != nil {
		fmt.Println("Error getting hostname:", err)
		return
	}

	//160.80.97.154
	url := fmt.Sprintf("http://160.80.97.154:8090/mockBattery/?hostName=%s", hostName)
	//url := fmt.Sprintf("http://127.0.0.1:8090/mockBattery/?hostName=%s", hostName)
	response, err := http.Get(url)
	if err != nil {
		fmt.Println("Error making GET request:", err)
		return
	}
	defer response.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}

	var resp map[string]interface{}
	err = json.Unmarshal(body, &resp)
	if err != nil {
		fmt.Println("error decoding json: ", err)
		return
	}

	// Print the decoded JSON data
	fmt.Println("Decoded JSON:")
	for key, value := range resp {
		fmt.Printf("%s: %v\n", key, value)
	}

	mockBatteryLevel := resp["battery"].(string)
	fmt.Println("=== battery extracted: ", mockBatteryLevel)

	//now that we have mock battery charge level, we can decrement it by 5% every minute
	batteryPercentage, err := strconv.ParseFloat(strings.TrimSpace(mockBatteryLevel), 64)
	decrementStep := (batteryPercentage * 5.0) / 100.0

	for {
		time.Sleep(60 * time.Second)

		batteryPercentage = batteryPercentage - (decrementStep)
		MyBattery.Mu.Lock()
		MyBattery.Value = batteryPercentage
		MyBattery.Mu.Unlock()

		log.Println("======= ", time.Now().Format("2006-01-02 15:04:05"), "mockBatteryLevel: ", mockBatteryLevel, " - batteryPercentage: ", batteryPercentage, "\n\n")

	}

}

func getBattery() {

	count := 0
	prevRaplWh := 0.0
	CWh := batteryCapacityWh

	getMockBattery()

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
