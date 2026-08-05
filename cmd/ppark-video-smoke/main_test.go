package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCasesUsesIdentityReferenceWithoutEmbeddingItInSummary(t *testing.T) {
	opt := options{
		model:      defaultModel,
		cases:      "all",
		reference:  filepath.Join("samples", "idle.gif"),
		seconds:    4,
		resolution: "480p",
		ratio:      "1:1",
	}
	cases := buildCases(opt, []byte("png"))
	if len(cases) != 2 {
		t.Fatalf("expected T2V and I2V cases, got %d", len(cases))
	}
	i2v := cases[1]
	if i2v.Request.ReferenceMode != "identity_reference" {
		t.Fatalf("I2V reference mode mismatch: %q", i2v.Request.ReferenceMode)
	}
	if strings.Contains(i2v.Summary.ReferenceFile, "data:") {
		t.Fatal("report summary must not contain inline reference data")
	}
	if len(i2v.Request.MediaSlots["reference_images"]) != 1 {
		t.Fatal("I2V request must contain exactly one identity reference")
	}
}

func TestWriteReportDoesNotContainEnvironmentKey(t *testing.T) {
	dir := t.TempDir()
	secret := "ark-secret-never-write"
	t.Setenv("VOLCENGINE_ARK_API_KEY", secret)
	if err := writeReport(dir, report{KeySource: "VOLCENGINE_ARK_API_KEY"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatal("report leaked the API key")
	}
}
