// Code generated by genpost.go; DO NOT EDIT.

package redis

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
)

func TestCgoAlignment_EbpfEvent(t *testing.T) {
	ebpftest.TestCgoAlignment[EbpfEvent](t)
}

func TestCgoAlignment_EbpfTx(t *testing.T) {
	ebpftest.TestCgoAlignment[EbpfTx](t)
}
