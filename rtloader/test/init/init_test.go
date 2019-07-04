package testinit

import (
	"testing"
)

func TestInit(t *testing.T) {
	if err := runInit(); err != nil {
		t.Errorf("Expected nil, got: %v", err)
	}
}
