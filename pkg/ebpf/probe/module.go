package probe

import (
	"errors"

	"github.com/iovisor/gobpf/bcc"
)

type Module struct {
	*bcc.Module
}

func NewModuleFromSource(source string, cflags []string) (*Module, error) {
	if len(source) == 0 {
		return nil, errors.New("no source for eBPF probe")
	}

	bccModule := bcc.NewModule(source, cflags)
	if bccModule == nil {
		return nil, errors.New("failed to compile eBPF probe")
	}

	return &Module{bccModule}, nil
}
