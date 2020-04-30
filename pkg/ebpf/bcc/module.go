package bcc

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	"github.com/iovisor/gobpf/bcc"
)

type Module struct {
	*bcc.Module
}

type Table struct {
	*bcc.Table
}

func (m *Module) Close() error {
	m.Module.Close()
	return nil
}

func (m *Module) RegisterPerfMap(perfMap *types.PerfMap) (*PerfMap, error) {
	if perfMap.Handler == nil {
		return nil, fmt.Errorf("no handler specified for perfmap %s", perfMap.Name)
	}

	bufferLength := perfMap.BufferLength
	if bufferLength == 0 {
		bufferLength = DefaultBufferLength
	}

	dataChan := make(chan []byte, bufferLength)
	bccTable := bcc.NewTable(m.TableId(perfMap.Name), m.Module)
	if bccTable == nil {
		return nil, fmt.Errorf("could not register perfmap %s", perfMap.Name)
	}

	bccPerfMap, err := bcc.InitPerfMap(bccTable, dataChan)
	if err != nil {
		return nil, fmt.Errorf("failed to start perf map: %s", err)
	}

	return &PerfMap{
		PerfMap:    perfMap,
		bccPerfMap: bccPerfMap,
		dataChan:   dataChan,
	}, nil
}

func (m *Module) RegisterTable(t *types.Table) (*Table, error) {
	bccTable := bcc.NewTable(m.TableId(t.Name), m.Module)
	if bccTable == nil {
		return nil, fmt.Errorf("could not register table %s", t.Name)
	}

	return &Table{bccTable}, nil
}

func (m *Module) RegisterKprobe(k *types.KProbe) error {
	if k.EntryFunc != "" {
		entryFd, err := m.LoadKprobe(k.EntryFunc)
		if err != nil {
			return fmt.Errorf("failed to load Kprobe %v: %s", k.EntryFunc, err)
		}
		if err = m.AttachKprobe(k.EntryEvent, entryFd, -1); err != nil {
			return fmt.Errorf("failed to attach Kprobe %v: %s", k.EntryEvent, err)
		}
	}
	if k.ExitFunc != "" {
		exitFd, err := m.LoadKprobe(k.ExitFunc)
		if err != nil {
			return fmt.Errorf("failed to load Kprobe %v: %s", k.ExitFunc, err)
		}
		if err = m.AttachKretprobe(k.ExitEvent, exitFd, -1); err != nil {
			return fmt.Errorf("failed to attach Kretprobe %v: %s", k.ExitEvent, err)
		}
	}

	return nil
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
