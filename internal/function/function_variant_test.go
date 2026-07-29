package function

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func testFunction(name string) *Function {
	return &Function{
		Name:           name,
		Runtime:        "python314",
		MemoryMB:       128,
		CPUDemand:      0.1,
		MaxConcurrency: 1,
		Handler:        name + ".handler",
		SupportedArchs: []string{"amd64", "arm64"},
		IsDefault:      true,
		SpeedUp:        1,
	}
}

func testVariant(name, defaultFunction string, speedup, utility float64) *Function {
	f := testFunction(name)
	f.IsDefault = false
	f.DefaultFunction = defaultFunction
	f.SpeedUp = speedup
	f.Utility = utility
	return f
}

func TestStandardFunctionWithoutVariantMetadata(t *testing.T) {
	var f Function
	if err := json.Unmarshal([]byte(`{"Name":"plain","Runtime":"python314","SupportedArchs":["amd64","arm64"]}`), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.IsVariant() {
		t.Fatal("plain function reported as variant")
	}
	if err := f.ValidateVariantMetadata(); err != nil {
		t.Fatalf("ValidateVariantMetadata() error = %v", err)
	}
}

func TestFilterVariantsOfDefaultFunction(t *testing.T) {
	base := testFunction("base")
	variant := testVariant("base-light", "base", 2, 0.8)

	variants := filterVariants("base", []*Function{base, variant})
	if len(variants) != 1 || variants[0].Name != "base-light" {
		t.Fatalf("variants = %#v", variants)
	}
	if variants[0].Utility != 0.8 || variants[0].SpeedUp != 2 {
		t.Fatalf("variant metadata = utility %v speedup %v", variants[0].Utility, variants[0].SpeedUp)
	}
}

func TestFilterVariantsKeepsDefaultsSeparate(t *testing.T) {
	functions := []*Function{
		testFunction("base-a"),
		testFunction("base-b"),
		testVariant("base-a-fast", "base-a", 2, 0.9),
		testVariant("base-a-small", "base-a", 1.5, 0.7),
		testVariant("base-b-fast", "base-b", 2, 0.6),
	}

	variants := filterVariants("base-a", functions)
	got := map[string]bool{}
	for _, f := range variants {
		got[f.Name] = true
	}
	if len(got) != 2 || !got["base-a-fast"] || !got["base-a-small"] {
		t.Fatalf("variants for base-a = %#v", variants)
	}
}

func TestVariantMetadataValidation(t *testing.T) {
	tests := []*Function{
		testVariant("same", "same", 1, 0.5),
		testVariant("bad-speed", "base", 0, 0.5),
		testVariant("bad-utility-low", "base", 1, -0.1),
		testVariant("bad-utility-high", "base", 1, 1.1),
		testVariant("conflict", "base", 1, 0.5),
	}
	tests[len(tests)-1].IsDefault = true

	for _, f := range tests {
		if err := f.ValidateVariantMetadata(); err == nil {
			t.Fatalf("ValidateVariantMetadata(%s) error = nil", f.Name)
		}
	}
}

func TestVariantMetadataValidationRequiresOriginalWhenMarkedNonDefault(t *testing.T) {
	var f Function
	if err := json.Unmarshal([]byte(`{"Name":"base-fast","IsDefault":false,"SpeedUp":2,"Utility":0.8}`), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	err := f.ValidateVariantMetadataFromJSON()
	if err == nil {
		t.Fatal("ValidateVariantMetadataFromJSON() error = nil")
	}
	if !strings.Contains(err.Error(), "default function is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestVariantMetadataValidationRequiresExplicitJSONSpeedupAndUtility(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "missing speedup",
			payload: `{"Name":"base-fast","DefaultFunction":"base","Utility":0.8}`,
			want:    "speedup is required",
		},
		{
			name:    "missing utility",
			payload: `{"Name":"base-fast","DefaultFunction":"base","SpeedUp":2}`,
			want:    "utility is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f Function
			if err := json.Unmarshal([]byte(tt.payload), &f); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			err := f.ValidateVariantMetadataFromJSON()
			if err == nil {
				t.Fatal("ValidateVariantMetadataFromJSON() error = nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestVariantUtilityZeroIsValidWhenExplicit(t *testing.T) {
	var f Function
	if err := json.Unmarshal([]byte(`{"Name":"base-fast","DefaultFunction":"base","SpeedUp":2,"Utility":0}`), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := f.ValidateVariantMetadataFromJSON(); err != nil {
		t.Fatalf("ValidateVariantMetadataFromJSON() error = %v", err)
	}
}

func TestDuplicateVariantNamesRemainDistinctRegistrationsByFunctionName(t *testing.T) {
	functions := []*Function{
		testVariant("base-fast", "base", 2, 0.7),
		testVariant("base-fast-v2", "base", 2.2, 0.9),
	}

	variants := filterVariants("base", functions)
	if len(variants) != 2 {
		t.Fatalf("len(variants) = %d, want 2", len(variants))
	}
	if variants[0].Name == variants[1].Name {
		t.Fatalf("variant names should be unique function registrations: %#v", variants)
	}
}

func TestVariantJSONPreservesSupportedArchs(t *testing.T) {
	f := testVariant("base-fast", "base", 2, 0.8)

	payload, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Function
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.DefaultFunction != "base" || decoded.Utility != 0.8 || decoded.SpeedUp != 2 {
		t.Fatalf("decoded variant metadata = %#v", decoded)
	}
	if !reflect.DeepEqual(decoded.SupportedArchs, []string{"amd64", "arm64"}) {
		t.Fatalf("SupportedArchs = %#v", decoded.SupportedArchs)
	}
}

func TestFunctionHasNoAWSVariantFields(t *testing.T) {
	typ := reflect.TypeOf(Function{})
	if _, ok := typ.FieldByName("ExternalProvider"); ok {
		t.Fatal("Function has ExternalProvider field")
	}
	if _, ok := typ.FieldByName("ArnCode"); ok {
		t.Fatal("Function has ArnCode field")
	}
}
