// Code generated by genpost.go; DO NOT EDIT.

package oomkill

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
)

func TestCgoAlignment_oomStats(t *testing.T) {
	ebpftest.TestCgoAlignment[oomStats](t)
}
