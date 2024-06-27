package proctracker

import (
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/gotls"
)

type pid = uint32

type binaryID = gotls.TlsBinaryId

type runningBinary struct {
	// Inode number of the binary
	binID binaryID

	// Modification time of the hooked binary, at the time of hooking.
	mTime syscall.Timespec

	// Reference counter for the number of currently running processes for
	// this binary.
	processCount int32

	// The location of the binary on the filesystem, as a string.
	binaryPath string

	// The value of DD_SERVICE for the given binary.
	// Associating a service name with a binary is not correct because
	// we may have the same binary running with different service names
	// on the same machine. However, for simplicity in the prototype we
	// assume a 1:1 mapping.
	serviceName string
}

type binaries map[binaryID]*runningBinary
type processes map[pid]binaryID
