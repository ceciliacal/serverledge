package scheduling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/serverledge-faas/serverledge/internal/config"
	"log"
	"math/rand"
	"net/http"
	"os"
)

type IlpOffloadingPolicy struct{}

func (policy *IlpOffloadingPolicy) Init() {

}

func (policy *IlpOffloadingPolicy) OnArrival(r *scheduledRequest) {
	_, err := policy.Evaluate(r)
	if err != nil {
		log.Printf("Error calling Evaluate request: %v", err)
		os.Exit(-1)
	}
	//TODO: In base alla richiesta eseguire la chiamata
}

func (policy *IlpOffloadingPolicy) OnCompletion(r *scheduledRequest) {

}

func (policy *IlpOffloadingPolicy) Evaluate(r *scheduledRequest) (OffloadingDecision, error) {

	if !r.CanDoOffloading {
		return OffloadingDecision{Offload: false}, nil
	}

	//TODO
	params := prepareParameters(r)

	jsonData, err := json.Marshal(params)

	if err != nil {
		log.Printf("Error marshalling parameters to JSON: %v", err)
		panic(err)
	}

	// Create POST request
	ilpOptimizerHost := config.GetString(config.FUNCTION_OFFLOADING_POLICY_OPTIMIZER_HOST, "localhost")
	ilpOptimizerPort := config.GetInt(config.FUNCTION_OFFLOADING_POLICY_OPTIMIZER_PORT, 8080)

	url := fmt.Sprintf("http://%s:%d/fc_ilp", ilpOptimizerHost, ilpOptimizerPort)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return OffloadingDecision{Offload: false}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return OffloadingDecision{Offload: false}, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode
	if statusCode != 200 {
		return OffloadingDecision{Offload: false}, fmt.Errorf("scheduling failed with status code %d", statusCode)
	}

	// Read and print response
	var placement Probs
	err = json.NewDecoder(resp.Body).Decode(&placement)
	if err != nil {
		fmt.Println(err)
		return OffloadingDecision{Offload: false}, fmt.Errorf("decoding response: %w", err)
	}

	return chooseRandom(placement), nil

}

func chooseRandom(placement Probs) OffloadingDecision {

	sum := placement.PLocal + placement.PCloud + placement.PEdge + placement.PDrop

	if sum == 0 {
		return OffloadingDecision{Offload: false, RemoteHost: ""}
	}

	placement.PLocal /= sum
	placement.PCloud /= sum
	placement.PEdge /= sum
	placement.PDrop /= sum

	// estrai random in [0,1)
	r := rand.Float64()

	// cumulativi
	if r < placement.PLocal {
		return OffloadingDecision{Offload: false, RemoteHost: ""}
	}
	if r < placement.PLocal+placement.PCloud {
		return OffloadingDecision{Offload: true, RemoteHost: "cloud"}
	}
	if r < placement.PLocal+placement.PCloud+placement.PEdge {
		return OffloadingDecision{Offload: true, RemoteHost: "edge"}
	}
	return OffloadingDecision{Offload: false, RemoteHost: "drop"}
}
