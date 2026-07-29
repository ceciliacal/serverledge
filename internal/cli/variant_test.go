package cli

import (
	"testing"

	"github.com/serverledge-faas/serverledge/internal/function"
)

func TestApplyVariantMetadataForStandardFunction(t *testing.T) {
	f := function.Function{Name: "base"}

	applyVariantMetadata(&f, "base", "", 2, 0.5)

	if f.Name != "base" {
		t.Fatalf("Name = %q, want base", f.Name)
	}
	if !f.IsDefault {
		t.Fatal("IsDefault = false, want true")
	}
	if f.DefaultFunction != "" {
		t.Fatalf("DefaultFunction = %q, want empty", f.DefaultFunction)
	}
	if f.SpeedUp != 1 {
		t.Fatalf("SpeedUp = %v, want 1", f.SpeedUp)
	}
}

func TestApplyVariantMetadataForVariant(t *testing.T) {
	f := function.Function{Name: "base"}

	applyVariantMetadata(&f, "base", "base-light", 2.5, 0.8)

	if f.Name != "base-light" {
		t.Fatalf("Name = %q, want base-light", f.Name)
	}
	if f.IsDefault {
		t.Fatal("IsDefault = true, want false")
	}
	if f.DefaultFunction != "base" {
		t.Fatalf("DefaultFunction = %q, want base", f.DefaultFunction)
	}
	if f.SpeedUp != 2.5 {
		t.Fatalf("SpeedUp = %v, want 2.5", f.SpeedUp)
	}
	if f.Utility != 0.8 {
		t.Fatalf("Utility = %v, want 0.8", f.Utility)
	}
}

func TestApplyVariantMetadataKeepsUtilityZero(t *testing.T) {
	f := function.Function{Name: "base"}

	applyVariantMetadata(&f, "base", "base-low-quality", 1.25, 0)

	if f.Name != "base-low-quality" {
		t.Fatalf("Name = %q, want base-low-quality", f.Name)
	}
	if f.DefaultFunction != "base" {
		t.Fatalf("DefaultFunction = %q, want base", f.DefaultFunction)
	}
	if f.SpeedUp != 1.25 {
		t.Fatalf("SpeedUp = %v, want 1.25", f.SpeedUp)
	}
	if f.Utility != 0 {
		t.Fatalf("Utility = %v, want 0", f.Utility)
	}
}

func TestApplyVariantMetadataNilRequestIsNoOp(t *testing.T) {
	applyVariantMetadata(nil, "base", "base-fast", 2, 0.8)
}
