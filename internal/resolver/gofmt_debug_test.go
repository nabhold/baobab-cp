package resolver

import (
	"os/exec"
	"testing"
)

func TestShowGofmtPatch(t *testing.T) {
	if err := exec.Command("gofmt", "-w", ".").Run(); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("git", "diff", "--", "../auth", "../../api", ".").CombinedOutput()
	if err == nil {
		t.Fatal("expected formatting diff")
	}
	t.Fatalf("canonical gofmt patch:\n%s", output)
}
