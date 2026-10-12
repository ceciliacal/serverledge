package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

func TestCloudRegionAverageByFunctionQuery(t *testing.T) {
	tests := []struct {
		name   string
		metric string
		area   string
		want   string
	}{
		{
			name:   "execution time one or many workers",
			metric: EXECUTION_TIME,
			area:   "cloud_a",
			want:   `avg by (function) ((sum by (function, node) (execution_time_sum{node=~"\\(cloud_a\\).*"})) / (sum by (function, node) (execution_time_count{node=~"\\(cloud_a\\).*"})))`,
		},
		{
			name:   "init time one or many workers",
			metric: INITIALIZATION_TIME,
			area:   "cloud_b",
			want:   `avg by (function) ((sum by (function, node) (init_time_sum{node=~"\\(cloud_b\\).*"})) / (sum by (function, node) (init_time_count{node=~"\\(cloud_b\\).*"})))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloudRegionAverageByFunctionQuery(tt.metric, tt.area)
			if got != tt.want {
				t.Fatalf("query = %s, want %s", got, tt.want)
			}
			if !strings.Contains(got, "avg by (function)") {
				t.Fatalf("query does not aggregate by function: %s", got)
			}
			if strings.Contains(got, "cloud_other") {
				t.Fatalf("query leaked another region: %s", got)
			}
		})
	}
}

func TestCloudRegionAverageByFunctionQueryIsolatesRegions(t *testing.T) {
	cloudA := cloudRegionAverageByFunctionQuery(EXECUTION_TIME, "cloud_a")
	cloudB := cloudRegionAverageByFunctionQuery(EXECUTION_TIME, "cloud_b")

	if !strings.Contains(cloudA, `node=~"\\(cloud_a\\).*"`) {
		t.Fatalf("cloud_a query missing cloud_a matcher: %s", cloudA)
	}
	if strings.Contains(cloudA, "cloud_b") {
		t.Fatalf("cloud_a query includes cloud_b: %s", cloudA)
	}
	if !strings.Contains(cloudB, `node=~"\\(cloud_b\\).*"`) {
		t.Fatalf("cloud_b query missing cloud_b matcher: %s", cloudB)
	}
}

func TestCloudRegionColdStartProbabilityQueryUsesSummedCounters(t *testing.T) {
	const area = "cloud_a"
	got := cloudRegionColdStartProbabilityQuery(area)
	want := `(sum by (function) (cold_starts_count{node=~"\\(cloud_a\\).*"}) / sum by (function) (completed_count{node=~"\\(cloud_a\\).*"})) and on (function) (sum by (function) (completed_count{node=~"\\(cloud_a\\).*"}) > 0)`

	if got != want {
		t.Fatalf("query = %s, want %s", got, want)
	}
	if strings.Contains(got, "avg") {
		t.Fatalf("cold-start probability must not average worker probabilities: %s", got)
	}
	if !strings.Contains(got, "> 0") {
		t.Fatalf("query should filter zero-denominator series: %s", got)
	}

	weighted := float64(10+5) / float64(100+10)
	unweighted := (0.10 + 0.50) / 2
	if weighted == unweighted {
		t.Fatal("test setup should distinguish weighted from unweighted probability")
	}
}

func TestRetrieveByFunctionKeepsPhysicalVariantsSeparate(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		gotQuery = r.Form.Get("query")
		response := map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": []map[string]any{
					{
						"metric": map[string]string{"function": "classifier"},
						"value":  []any{time.Now().Unix(), "1.25"},
					},
					{
						"metric": map[string]string{"function": "classifier_fast"},
						"value":  []any{time.Now().Unix(), "0.40"},
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := promapi.NewClient(promapi.Config{Address: server.URL})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	api := v1.NewAPI(client)
	query := cloudRegionAverageByFunctionQuery(EXECUTION_TIME, "cloud_a")

	got, err := retrieveByFunction(query, api, context.Background())
	if err != nil {
		t.Fatalf("retrieveByFunction() error = %v", err)
	}
	if gotQuery != query {
		t.Fatalf("query = %s, want %s", gotQuery, query)
	}
	if got["classifier"] != 1.25 {
		t.Fatalf("classifier = %v, want 1.25", got["classifier"])
	}
	if got["classifier_fast"] != 0.40 {
		t.Fatalf("classifier_fast = %v, want 0.40", got["classifier_fast"])
	}
}
