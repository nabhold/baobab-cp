package resolver

import (
	"os/exec"
	"testing"
)

func TestPrintRelocationGofmtDiff(t *testing.T) {
	cmd := exec.Command("gofmt", "-w", "relocation.go", "relocation_test.go")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gofmt: %v: %s", err, out)
	}
	cmd = exec.Command("git", "diff", "--", "relocation.go", "relocation_test.go")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git diff: %v: %s", err, out)
	}
	if len(out) > 0 {
		t.Fatalf("gofmt diff:\n%s", out)
	}
}
