package resolver

import (
	"os/exec"
	"testing"
)

func TestPrintObservabilityGofmtDiff(t *testing.T) {
	files := []string{"trace.go", "trace_test.go", "pipeline.go", "../service/resolution_service.go"}
	args := append([]string{"-w"}, files...)
	cmd := exec.Command("gofmt", args...)
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gofmt: %v: %s", err, out)
	}
	cmd = exec.Command("git", "diff", "--", "internal/resolver/trace.go", "internal/resolver/trace_test.go", "internal/resolver/pipeline.go", "internal/service/resolution_service.go")
	cmd.Dir = "../.."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git diff: %v: %s", err, out)
	}
	if len(out) > 0 {
		t.Fatalf("gofmt diff:\n%s", out)
	}
}
