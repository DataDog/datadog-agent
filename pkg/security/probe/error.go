package probe

import "errors"

var (
	// ErrEmptySource - Empty source error
	ErrEmptySource = errors.New("eBPF source cannot be empty")
	// ErrUnknownProbeType - Unknown probe type error
	ErrUnknownProbeType = errors.New("unknown eBPF probe type")
	// ErrNoDataHandler - No data handler provided
	ErrNoDataHandler = errors.New("data handler missing")
)
