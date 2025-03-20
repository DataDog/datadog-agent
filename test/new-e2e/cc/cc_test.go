package cc

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

func TestCcMarked(t *testing.T) {
	flake.MarkOnLog(t, "panic")

	panic("im panicing")
}

func TestCcMarkedMultiple(t *testing.T) {
	flake.MarkOnLog(t, "hey")
	flake.MarkOnLog(t, "panic")
	flake.MarkOnLog(t, "ho")

	panic("im panicing")
}

func TestCcNotMarked(t *testing.T) {
	panic("im panicing")
}

func TestCcPass(t *testing.T) {
	println("I'm passing")
}
