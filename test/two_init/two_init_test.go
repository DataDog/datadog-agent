package twoinit

import "testing"

func TestInit(t *testing.T) {
	if err := init2(); err != nil {
		t.Errorf("Expected nil, got: %v", err)
	}
}
