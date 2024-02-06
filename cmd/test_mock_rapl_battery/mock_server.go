package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

/*
this server gives each of 3 edge nodes a random distinct mock battery value between {15.0, 35.0, 55.0}
it has to run on the server machine on port 8090
*/
func main() {

	//chargeLevels := []float64{15.0, 35.0, 55.0}
	chargeLevels := []float64{19.0, 18.0, 35.0}

	rand.Seed(time.Now().UnixNano())

	// Define the handler function for the API endpoint with path and query parameters
	apiHandler := func(w http.ResponseWriter, r *http.Request) {
		// Extract the path parameter from the URL
		apiVersion := r.URL.Path[len("/mockBattery/"):]

		// Get the query parameters from the URL
		params := r.URL.Query()

		chargeLevelResult := 100.0
		// Extract specific query parameters
		vmName := params.Get("hostName")

		if vmName != "ceciliacloud1" && len(chargeLevels) > 0 { //se è uno dei nodi edge
			randomIndex := rand.Intn(len(chargeLevels))
			chargeLevelResult = chargeLevels[randomIndex]
			chargeLevels = delete_at_index(chargeLevels, randomIndex)
		}
		fmt.Println("chargeLevelResult: ", chargeLevelResult)

		// Set the Content-Type header
		w.Header().Set("Content-Type", "application/json")

		batteryResult := strconv.FormatFloat(chargeLevelResult, 'f', 2, 64)
		// Write the response JSON with the path and query parameter values
		response := map[string]string{
			"version":  apiVersion,
			"hostName": vmName,
			"battery":  batteryResult,
		}
		json.NewEncoder(w).Encode(response)
	}

	// Register the API handler function for the "/api" endpoint
	http.HandleFunc("/mockBattery/", apiHandler)

	// Start the HTTP server on port 8090
	// Get the host name
	hostName, _ := os.Hostname()
	addrs, _ := net.LookupIP(hostName)
	// Print the IP addresses
	fmt.Println("IP addresses for", hostName+":")
	for _, addr := range addrs {
		fmt.Println(addr)
	}

	fmt.Println("Server is running on http://", addrs, ":8090")
	err := http.ListenAndServe(":8090", nil)
	if err != nil {
		fmt.Println("Error starting the server:", err)
	}

} //192.168.178.49

func delete_at_index(slice []float64, index int) []float64 {
	return append(slice[:index], slice[index+1:]...)
}
