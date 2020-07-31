package bytecode

import (
	"io"
)

// DefaultBPFDir is the default path for eBPF programs
var DefaultBPFDir = "/opt/datadog-agent/embedded/usr/share/system-probe/ebpf"

// AssetReader describes the combination of both io.Reader and io.ReaderAt
type AssetReader interface {
	io.Reader
	io.ReaderAt
}
