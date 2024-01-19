package energy

import (
	"bytes"
	_ "fmt"
	"log"
	"os/exec"
	"time"
)

func Init() {
	// config battery capacity
	getBattery()

}

func getBattery() {
	//TODO imposta capacità batteria
	// todo leggi RAPL e fai sottrazione

	for {
		time.Sleep(5 * time.Second)
		//fmt.Println("=== *PERCENTUALE BATTERIA* === ", time.Now().String())
		readRAPL()
	}

}

func readRAPL() {
	command := "sudo cat/sys/class/powercap/intel-rapl/intel-rapl:0/energy_uj"
	output, err := RunCMD(command)
	if err != nil {
		log.Fatal("Error in reading RAPL:", err.Error(), "\nOutput :", string(output))
	}
}

// RunCMD is a simple wrapper around terminal commands
func RunCMD(command string) ([]byte, error) {
	var buf bytes.Buffer
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Start()
	if err != nil {
		return buf.Bytes(), err
	}
	_, err = cmd.Process.Wait()
	return buf.Bytes(), err
}
