// This file is currently almost empty.
// It’s full implementation will come with PR #4619
// The purpose of this empty shell here and now is to ensure that we pull the gobpf/bcc package
// otherwise go discards this package and the rest of the change isn’t tested at all.

// +build linux_bpf

package ebpf

import (
	bpflib "github.com/iovisor/gobpf/bcc"
)

func NewTCPQueueLengthTracer() {
	_ = bpflib.NewModule("", []string{})
}
