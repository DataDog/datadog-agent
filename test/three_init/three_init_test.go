package threeinit

import "testing"

func TestInit(t *testing.T) {
	if err := init3(); err != nil {
		t.Errorf("Expected nil, got: %v", err)
	}
}
