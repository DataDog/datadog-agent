// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package loader supports setting up the eBPF program.
package loader

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// Loader is responsible for loading the eBPF, making it ready to attach.
type Loader struct {
	config

	// Shared ringbuffer for collecting probe output
	ringbufMap    *ebpf.Map
	ringbufReader *ringbuf.Reader

	ebpfSpec *ebpf.CollectionSpec
}

// WithEbpfConfig sets the eBPF configuration for the compiler.
func WithEbpfConfig(cfg *ddebpf.Config) Option {
	return (*ebpfConfigOption)(cfg)
}

// WithRingBufSize sets the size of the ring buffer for the Actuator.
func WithRingBufSize(size uint32) Option {
	return ringBufSizeOption(size)
}

// WithDebugLevel sets the debug level for the ebpf program.
// It forces use of a binary compiled in a debug mode
func WithDebugLevel(level int) Option {
	return debugLevelOption(level)
}

// WithAdditionalSerializer sets an additional serializer for the ebpf program.
func WithAdditionalSerializer(serializer compiler.CodeSerializer) Option {
	return additionalSerializerOption{serializer}
}

// NewLoader creates a new Loader.
func NewLoader(opts ...Option) (*Loader, error) {
	l := &Loader{}
	err := l.init(opts...)
	if err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Load loads the program.
func (l *Loader) Load(program compiler.Program) (*Program, error) {
	serialized, err := serializeProgram(program, l.additionalSerializer)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize program: %w", err)
	}

	spec := l.ebpfSpec.Copy()

	maps, err := l.loadData(serialized, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}
	defer func() {
		for k, m := range maps {
			if k == ringbufMapName {
				continue
			}
			_ = m.Close()
		}
	}()

	ringbufMapSpec, ok := spec.Maps[ringbufMapName]
	if !ok {
		return nil, fmt.Errorf("ringbuffer map not found in eBPF spec")
	}
	ringbufMapSpec.MaxEntries = uint32(l.config.ringBufSize)

	opts := ebpf.CollectionOptions{}
	opts.MapReplacements = maps
	opts.MapReplacements[ringbufMapName] = l.ringbufMap

	collection, err := ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			return nil, fmt.Errorf("failed to create collection: %w\n%+v", err, ve)
		}
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}
	bpfProgram, ok := collection.Programs["probe_run_with_cookie"]
	if !ok {
		return nil, fmt.Errorf("probe_run_with_cookie program not found in collection")
	}

	maps = nil
	return &Program{
		Collection:   collection,
		BpfProgram:   bpfProgram,
		Attachpoints: serialized.bpfAttachPoints,
	}, nil
}

// OutputReader returns the ringbuffer reader for the loader.
func (l *Loader) OutputReader() *ringbuf.Reader {
	if l.ringbufReader == nil {
		panic("ringbuffer reader not initialized")
	}
	return l.ringbufReader
}

// Close releases loader resources.
func (l *Loader) Close() (err error) {
	if l.ringbufReader != nil {
		err = errors.Join(err, l.ringbufReader.Close())
	}
	if l.ringbufMap != nil {
		err = errors.Join(err, l.ringbufMap.Close())
	}
	return err
}

// Program represents a loaded eBPF program.
type Program struct {
	Collection   *ebpf.Collection
	BpfProgram   *ebpf.Program
	Attachpoints []BPFAttachPoint
}

// Close releases the program resources.
func (p *Program) Close() {
	if p.Collection != nil {
		p.Collection.Close() // should already contain the program
	}
}

const defaultRingbufSize = 1 << 20 // 1 MiB
const ringbufMapName = "out_ringbuf"

type config struct {
	ebpfConfig *ddebpf.Config

	ringBufSize uint32

	dyninstDebugLevel   uint8
	dyninstDebugEnabled bool

	additionalSerializer compiler.CodeSerializer
}

// Option configures the Loader.
type Option interface {
	apply(c *config)
}

type ebpfConfigOption ddebpf.Config

func (o *ebpfConfigOption) apply(c *config) {
	c.ebpfConfig = (*ddebpf.Config)(o)
}

type ringBufSizeOption uint32

func (o ringBufSizeOption) apply(c *config) {
	c.ringBufSize = uint32(o)
}

type debugLevelOption uint8

func (o debugLevelOption) apply(c *config) {
	c.dyninstDebugLevel = uint8(o)
	c.dyninstDebugEnabled = true
}

type additionalSerializerOption struct {
	compiler.CodeSerializer
}

func (o additionalSerializerOption) apply(c *config) {
	c.additionalSerializer = o
}

func (l *Loader) init(opts ...Option) error {
	l.config.ringBufSize = defaultRingbufSize
	for _, opt := range opts {
		opt.apply(&l.config)
	}
	if l.config.ebpfConfig == nil {
		l.config.ebpfConfig = ddebpf.NewConfig()
	}
	var err error
	l.ringbufMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Name:       ringbufMapName,
		Type:       ebpf.RingBuf,
		MaxEntries: uint32(l.config.ringBufSize),
	})
	if err != nil {
		return fmt.Errorf("failed to create ringbuffer map: %w", err)
	}
	l.ringbufReader, err = ringbuf.NewReader(l.ringbufMap)
	if err != nil {
		return fmt.Errorf("failed to create ringbuffer reader: %w", err)
	}
	var obj io.ReaderAt
	if l.config.dyninstDebugEnabled {
		obj, err = bytecode.GetReader(filepath.Join(l.ebpfConfig.BPFDir, "co-re"), "dyninst_event-debug.o")
	} else {
		obj, err = bytecode.GetReader(filepath.Join(l.ebpfConfig.BPFDir, "co-re"), "dyninst_event.o")
	}
	if err != nil {
		return fmt.Errorf("failed to get eBPF object: %w", err)
	}
	l.ebpfSpec, err = ebpf.LoadCollectionSpecFromReader(obj)
	if err != nil {
		return fmt.Errorf("failed to load eBPF object: %w", err)
	}
	ringbufMapSpec, ok := l.ebpfSpec.Maps[ringbufMapName]
	if !ok {
		return fmt.Errorf("ringbuffer map not found in eBPF spec")
	}
	ringbufMapSpec.MaxEntries = uint32(l.config.ringBufSize)
	return nil

}

func (l *Loader) loadData(
	serialized *serializedProgram,
	spec *ebpf.CollectionSpec,
) (map[string]*ebpf.Map, error) {
	const codeMapName = "stack_machine_code"
	const typeIDsMapName = "type_ids"
	const typeInfoMapName = "type_info"
	const throttlerMapName = "throttler_params"
	const throttlerStateMapName = "throttler_buf"
	const probeParamsMapName = "probe_params"

	mapSpec, codeMap, err := makeArrayMap(codeMapName, serialized.code, true /* singleEntry */)
	spec.Maps[codeMapName] = mapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create code map: %w", err)
	}
	defer func() {
		if codeMap != nil {
			codeMap.Close()
		}
	}()
	err = setVariable(spec, "stack_machine_code_len", uint32(len(serialized.code)))
	if err != nil {
		return nil, fmt.Errorf("failed to set stack_machine_code_len: %w", err)
	}
	err = setVariable(spec, "stack_machine_code_max_op", serialized.maxOpLen)
	if err != nil {
		return nil, fmt.Errorf("failed to set stack_machine_code_max_op: %w", err)
	}
	err = setVariable(spec, "chase_pointers_entrypoint", serialized.chasePointersEntrypoint)
	if err != nil {
		return nil, fmt.Errorf("failed to set chase_pointers_entrypoint: %w", err)
	}
	err = setVariable(spec, "prog_id", uint32(serialized.programID))
	if err != nil {
		return nil, fmt.Errorf("failed to set prog_id: %w", err)
	}

	mapSpec, typeIDsMap, err := makeArrayMap(typeIDsMapName, serialized.typeIDs, false /* singleEntry */)
	spec.Maps[typeIDsMapName] = mapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create type_ids map: %w", err)
	}
	defer func() {
		if typeIDsMap != nil {
			typeIDsMap.Close()
		}
	}()
	err = setVariable(spec, "num_types", uint32(len(serialized.typeIDs)))
	if err != nil {
		return nil, fmt.Errorf("failed to set num_types: %w", err)
	}

	mapSpec, typeInfoMap, err := makeArrayMap(typeInfoMapName, serialized.typeInfos, false /* singleEntry */)
	spec.Maps[typeInfoMapName] = mapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create type_info map: %w", err)
	}
	defer func() {
		if typeInfoMap != nil {
			typeInfoMap.Close()
		}
	}()

	mapSpec, throttlerMap, err := makeArrayMap(throttlerMapName, serialized.throttlerParams, false /* singleEntry */)
	spec.Maps[throttlerMapName] = mapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create throttler_params map: %w", err)
	}
	defer func() {
		if throttlerMap != nil {
			throttlerMap.Close()
		}
	}()
	err = setVariable(spec, "num_throttlers", uint32(len(serialized.throttlerParams)))
	if err != nil {
		return nil, fmt.Errorf("failed to set num_throttlers: %w", err)
	}

	mapSpec, ok := spec.Maps[throttlerStateMapName]
	if !ok {
		return nil, fmt.Errorf("throttler_buf map not found in eBPF spec")
	}
	mapSpec.MaxEntries = uint32(len(serialized.throttlerParams))

	mapSpec, probeParamsMap, err := makeArrayMap(probeParamsMapName, serialized.probeParams, false /* singleEntry */)
	spec.Maps[probeParamsMapName] = mapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create probe_params map: %w", err)
	}
	defer func() {
		if probeParamsMap != nil {
			probeParamsMap.Close()
		}
	}()
	err = setVariable(spec, "num_probe_params", uint32(len(serialized.probeParams)))
	if err != nil {
		return nil, fmt.Errorf("failed to set num_probe_params: %w", err)
	}

	if l.config.dyninstDebugEnabled {
		err = setVariable(spec, "debug_level", uint32(l.config.dyninstDebugLevel))
		if err != nil {
			return nil, fmt.Errorf("failed to set debug_level: %w", err)
		}
	}

	m := map[string]*ebpf.Map{
		codeMapName:        codeMap,
		typeIDsMapName:     typeIDsMap,
		typeInfoMapName:    typeInfoMap,
		throttlerMapName:   throttlerMap,
		probeParamsMapName: probeParamsMap,
	}
	codeMap = nil
	typeIDsMap = nil
	typeInfoMap = nil
	throttlerMap = nil
	probeParamsMap = nil
	return m, nil
}

func makeArrayMap[T any](name string, data []T, singleEntry bool) (*ebpf.MapSpec, *ebpf.Map, error) {
	var val T
	elemSize := uint32(unsafe.Sizeof(val))
	mapSpec := &ebpf.MapSpec{
		Name:       name,
		Type:       ebpf.Array,
		KeySize:    4,
		ValueSize:  uint32(elemSize),
		MaxEntries: uint32(len(data)),
		Flags:      features.BPF_F_MMAPABLE,
	}
	// singleEntry makes the map have a single element holding all the data.
	if singleEntry {
		mapSpec.ValueSize = mapSpec.MaxEntries * mapSpec.ValueSize
		mapSpec.MaxEntries = 1
	}
	if mapSpec.ValueSize%8 != 0 && !singleEntry {
		return nil, nil, fmt.Errorf("map %s has value size %d which is not a multiple of 8", name, mapSpec.ValueSize)
	}
	m, err := ebpf.NewMap(mapSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create %s map: %w", name, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = m.Close()
		}
	}()
	mem, err := m.Memory()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get %s map memory: %w", name, err)
	}
	bytes := unsafe.Slice(
		(*byte)(unsafe.Pointer(&data[0])),
		int(elemSize)*len(data),
	)
	_, err = mem.WriteAt(bytes, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write %s map: %w", name, err)
	}
	cleanup = false
	return mapSpec, m, nil
}

func setVariable(spec *ebpf.CollectionSpec, name string, value uint32) error {
	vari, ok := spec.Variables[name]
	if !ok {
		return fmt.Errorf("variable %s not found in spec", name)
	}
	err := vari.Set(value)
	if err != nil {
		return fmt.Errorf("failed to set %s: %w", name, err)
	}
	return nil
}
