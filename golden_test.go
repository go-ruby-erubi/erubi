package erubi

import (
	"encoding/json"
	"os"
	"testing"
)

// srcVectors reproduces every compiler invocation the differential MRI oracle
// makes, capturing only the emitted Ruby source. The oracle proves this source
// is byte-identical to the erubi gem; TestSrcGolden pins the same source as a
// committed golden so the full compiler is exercised, and self-verified, on the
// platforms where ruby is absent (Windows, qemu) and the oracle skips itself.
func srcVectors() []string {
	var out []string
	for _, sc := range scenarios {
		templates := baseTemplates
		if sc.capture {
			templates = captureTemplates
		}
		for _, tpl := range templates {
			if sc.capture {
				out = append(out, NewCaptureEndEngine(tpl, sc.opts).Src())
			} else {
				out = append(out, NewEngine(tpl, sc.opts).Src())
			}
		}
	}
	for _, escape := range []bool{false, true} {
		opts := Options{}
		if escape {
			opts.Escape = Bool(true)
		}
		for _, rc := range renderCases {
			out = append(out, NewEngine(rc.tpl, opts).Src())
		}
	}
	return out
}

const goldenPath = "testdata/src.golden.json"

// TestSrcGolden compiles the full oracle corpus and compares the emitted Ruby
// source against the committed golden vectors. Regenerate with
// `UPDATE_GOLDEN=1 go test` on a host where the MRI oracle passes, which is what
// certifies the vectors as byte-identical to the erubi gem.
func TestSrcGolden(t *testing.T) {
	got := srcVectors()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d golden vectors to %s", len(got), goldenPath)
		return
	}
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (regenerate with UPDATE_GOLDEN=1): %v", err)
	}
	var want []string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("vector count = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("src vector %d mismatch\n got=%q\nwant=%q", i, got[i], want[i])
		}
	}
}
