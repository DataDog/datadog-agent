package exitcode

import (
	"errors"
	"os/exec"
	"testing"
)

func TestFromNil(t *testing.T) {
	if got := From(nil); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestFromGenericError(t *testing.T) {
	if got := From(errors.New("boom")); got != 1 {
		t.Fatalf("expected fallback 1, got %d", got)
	}
}

func TestFromExitError(t *testing.T) {
	cmd := exec.Command("bash", "-c", "exit 5")
	err := cmd.Run()
	if got := From(err); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}
