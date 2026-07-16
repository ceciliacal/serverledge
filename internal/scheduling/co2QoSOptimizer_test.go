package scheduling

import (
	"testing"

	"github.com/serverledge-faas/serverledge/internal/function"
)

func TestIsFunctionVariant(t *testing.T) {
	tests := []struct {
		name string
		f    *function.Function
		want bool
	}{
		{
			name: "nil is not a variant",
			f:    nil,
			want: false,
		},
		{
			name: "legacy registered function with zero IsDefault is not a variant",
			f: &function.Function{
				Name: "base",
			},
			want: false,
		},
		{
			name: "default function is not a variant",
			f: &function.Function{
				Name:      "base",
				IsDefault: true,
			},
			want: false,
		},
		{
			name: "function with parent default function is a variant",
			f: &function.Function{
				Name:            "base-fast",
				DefaultFunction: "base",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFunctionVariant(tt.f); got != tt.want {
				t.Fatalf("isFunctionVariant() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyTestCloudLatencyMultiplier(t *testing.T) {
	tests := []struct {
		name    string
		latency float64
		region  string
		cfg     Co2QosPolicyConfig
		want    float64
	}{
		{
			name:    "disabled leaves latency unchanged",
			latency: 0.25,
			region:  "b",
			cfg: Co2QosPolicyConfig{
				TestCloudLatencyMultiplierEnable: false,
				TestCloudLatencyMultiplierRegion: "b",
				TestCloudLatencyMultiplier:       2,
			},
			want: 0.25,
		},
		{
			name:    "enabled doubles matching region",
			latency: 0.25,
			region:  "b",
			cfg: Co2QosPolicyConfig{
				TestCloudLatencyMultiplierEnable: true,
				TestCloudLatencyMultiplierRegion: "b",
				TestCloudLatencyMultiplier:       2,
			},
			want: 0.5,
		},
		{
			name:    "region match is case insensitive",
			latency: 0.25,
			region:  "B",
			cfg: Co2QosPolicyConfig{
				TestCloudLatencyMultiplierEnable: true,
				TestCloudLatencyMultiplierRegion: "b",
				TestCloudLatencyMultiplier:       2,
			},
			want: 0.5,
		},
		{
			name:    "non matching region is unchanged",
			latency: 0.25,
			region:  "a",
			cfg: Co2QosPolicyConfig{
				TestCloudLatencyMultiplierEnable: true,
				TestCloudLatencyMultiplierRegion: "b",
				TestCloudLatencyMultiplier:       2,
			},
			want: 0.25,
		},
		{
			name:    "invalid multiplier is ignored",
			latency: 0.25,
			region:  "b",
			cfg: Co2QosPolicyConfig{
				TestCloudLatencyMultiplierEnable: true,
				TestCloudLatencyMultiplierRegion: "b",
				TestCloudLatencyMultiplier:       0,
			},
			want: 0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applyTestCloudLatencyMultiplier(tt.latency, tt.region, tt.cfg); got != tt.want {
				t.Fatalf("applyTestCloudLatencyMultiplier() = %v, want %v", got, tt.want)
			}
		})
	}
}
