package api

import (
	"reflect"
	"testing"

	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/scheduling"
	"github.com/spf13/viper"
)

func TestCreateSchedulingPolicyPreservesExistingPoliciesAndAddsCo2Qos(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		wantType   any
	}{
		{name: "default", configured: "default", wantType: &scheduling.DefaultLocalPolicy{}},
		{name: "localonly", configured: "localonly", wantType: &scheduling.DefaultLocalPolicy{}},
		{name: "cloudonly", configured: "cloudonly", wantType: &scheduling.CloudOnlyPolicy{}},
		{name: "edgecloud", configured: "edgecloud", wantType: &scheduling.CloudEdgePolicy{}},
		{name: "edgeonly", configured: "edgeonly", wantType: &scheduling.EdgePolicy{}},
		{name: "co2qosaware", configured: "co2qosaware", wantType: &scheduling.Co2QosAwarePolicy{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.Set(config.SCHEDULING_POLICY, tt.configured)

			got := CreateSchedulingPolicy()
			if reflect.TypeOf(got) != reflect.TypeOf(tt.wantType) {
				t.Fatalf("policy type = %T, want %T", got, tt.wantType)
			}
		})
	}
}
