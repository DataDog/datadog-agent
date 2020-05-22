package gobpf

import (
	"errors"
	"fmt"
	"io"
	"log"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	bpflib "github.com/iovisor/gobpf/elf"
)

var ErrEBPFNotSupported = errors.New("eBPF is not supported")

type Module struct {
	*bpflib.Module
}

type Table struct {
	*bpflib.Map
	module *bpflib.Module
}

func (t *Table) Get(key []byte) ([]byte, error) {
	var value [1024]byte
	err := t.module.LookupElement(t.Map, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]))
	return value[:], err
}

func (t *Table) Set(key, value []byte) {
	t.module.UpdateElement(t.Map, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]), 0)
}

func (t *Table) Delete(key []byte) error {
	return t.module.DeleteElement(t.Map, unsafe.Pointer(&key[0]))
}

func (m *Module) RegisterPerfMap(perfMap *types.PerfMap) (probe.PerfMap, error) {
	bufferLength := perfMap.BufferLength
	if bufferLength == 0 {
		bufferLength = DefaultBufferLength
	}

	eventChannel := make(chan []byte, perfMap.BufferLength)
	lostChannel := make(chan uint64, DefaultLostEventSize)

	pm, err := bpflib.InitPerfMap(m.Module, string(perfMap.Name), eventChannel, lostChannel)
	if err != nil {
		return nil, fmt.Errorf("error initializing perf map: %s", err)
	}

	log.Printf("Registered perf map %s", perfMap.Name)

	return &PerfMap{
		PerfMap:      pm,
		handler:      perfMap.Handler,
		eventChannel: eventChannel,
		lostChannel:  lostChannel,
	}, nil
}

func (m *Module) RegisterTable(table *types.Table) (probe.Table, error) {
	hashMap := m.Map(table.Name)
	if hashMap == nil {
		return nil, fmt.Errorf("failed to find table '%s'", table.Name)
	}

	return &Table{Map: m.Map(table.Name), module: m.Module}, nil
}

func NewModuleFromReader(reader io.ReaderAt) (*Module, error) {
	module := bpflib.NewModuleFromReader(reader)
	if module == nil {
		return nil, ErrEBPFNotSupported
	}

	if err := module.Load(nil); err != nil {
		log.Printf("eBPF verifiers logs: %s", string(module.Log()))
		return nil, err
	}

	/*
		map[string]bpflib.SectionParams{
			"maps/" + name: {
				MapMaxEntries: mapMaxEntries,
			},
		})
	*/

	return &Module{module}, nil
}
