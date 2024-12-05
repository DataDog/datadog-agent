// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestConsumer(t *testing.T) {
	kversion, err := kernel.HostVersion()
	require.NoError(t, err)
	if minVersion := kernel.VersionCode(4, 14, 0); kversion < minVersion {
		t.Skipf("package not supported by kernels < %s", minVersion)
	}

	const numEvents = 100
	c := config.New()
	program, err := newEBPFProgram(c)
	require.NoError(t, err)

	var mux sync.Mutex
	result := make(map[uint64]int)
	callback := func(events []uint64) {
		mux.Lock()
		defer mux.Unlock()
		for _, n := range events {
			result[n] = +1
		}
	}

	consumer, err := NewConsumer("test", program, callback)
	require.NoError(t, err)
	consumer.Start()

	err = program.Start()
	require.NoError(t, err)

	// generate test events
	generator := newEventGenerator(program, t)
	for i := 0; i < numEvents; i++ {
		require.NoError(t, generator.Generate(uint64(i)))
	}
	generator.Stop()
	time.Sleep(100 * time.Millisecond)

	// this ensures that any incomplete batch left in eBPF is fully processed
	consumer.Sync()
	require.NoError(t, program.Stop(manager.CleanAll))
	consumer.Stop()

	// ensure that we have received each event exactly once
	for i := 0; i < numEvents; i++ {
		actual := result[uint64(i)]
		assert.Equalf(t, 1, actual, "eventID=%d should have 1 occurrence. got %d", i, actual)
	}
}

type eventGenerator struct {
	// map used for coordinating test with eBPF program space
	testMap *ebpf.Map

	// file used for triggering write(2) syscalls
	testFile *os.File
}

func newEventGenerator(program *manager.Manager, t *testing.T) *eventGenerator {
	m, _, _ := program.GetMap("test")
	require.NotNilf(t, m, "couldn't find test map")

	f, err := os.Create(filepath.Join(t.TempDir(), "foobar"))
	require.NoError(t, err)

	return &eventGenerator{
		testMap:  m,
		testFile: f,
	}
}

func (e *eventGenerator) Generate(eventID uint64) error {
	type testCtx struct {
		pid     uint32
		fd      uint64
		eventID uint64
	}

	var (
		fd  = uint64(e.testFile.Fd())
		key = uint32(0)
		val = testCtx{pid: uint32(os.Getpid()), fd: fd, eventID: eventID}
	)

	// this is done so the eBPF program will echo back an event that is equal to
	// eventID once the write syscall is triggered below
	err := e.testMap.Put(unsafe.Pointer(&key), unsafe.Pointer(&val))
	if err != nil {
		return err
	}

	_, err = e.testFile.Write([]byte("whatever"))
	return err
}

func (e *eventGenerator) Stop() {
	e.testFile.Close()
}

func newEBPFProgram(c *config.Config) (*manager.Manager, error) {
	bc, err := bytecode.GetReader(c.BPFDir, "usm_events_test-debug.o")
	if err != nil {
		return nil, err
	}
	defer bc.Close()

	m := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "tracepoint__syscalls__sys_enter_write",
				},
			},
		},
	}
	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "tracepoint__syscalls__sys_enter_write",
				},
			},
		},
		ConstantEditors: []manager.ConstantEditor{
			{
				Name:  "test_monitoring_enabled",
				Value: uint64(1),
			},
		},
	}

	Configure(config.New(), "test", m, &options)
	err = m.InitWithOptions(bc, options)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func TestInvalidBatchCountMetric(t *testing.T) {
	kversion, err := kernel.HostVersion()
	require.NoError(t, err)
	if minVersion := kernel.VersionCode(4, 14, 0); kversion < minVersion {
		t.Skipf("package not supported by kernels < %s", minVersion)
	}

	program, err := newEBPFProgram(config.New())
	require.NoError(t, err)

	ringBufferHandler := ddebpf.NewRingBufferHandler(1)
	ringBufferHandler.RecordHandler(&ringbuf.Record{
		RawSample: []byte("test"),
	}, nil, nil)

	consumer, err := NewConsumer("test", program, func(_ []uint64) {})
	require.NoError(t, err)
	consumer.handler = ringBufferHandler

	consumer.Start()
	program.Stop(manager.CleanAll)
	consumer.Stop()

	require.Equalf(t, int(consumer.invalidBatchCount.Get()), 1, "invalidBatchCount should be greater than 0")
}

func EncodeToBytes(p interface{}) []byte {
	buf := bytes.Buffer{}
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(p)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("uncompressed size (bytes): ", len(buf.Bytes()))
	return buf.Bytes()
}

// Serialize a struct to a [4096]int8 array with specified endianness
func structToBytes(data EbpfEvent, littleEndian bool) ([4096]int8, error) {
	var result [4096]int8
	var buffer bytes.Buffer

	// Set the binary encoding order based on the littleEndian flag
	var order binary.ByteOrder
	if littleEndian {
		order = binary.LittleEndian
	} else {
		order = binary.BigEndian
	}

	// Serialize the struct manually field by field
	err := binary.Write(&buffer, order, data)
	if err != nil {
		return result, fmt.Errorf("failed to serialize struct: %v", err)
	}

	// Check if the serialized data fits in the array
	serializedData := buffer.Bytes()
	if len(serializedData) > len(result) {
		return result, fmt.Errorf("serialized data exceeds 4096 bytes")
	}

	// Convert serialized data to [4096]int8
	for i, b := range serializedData {
		result[i] = int8(b)
	}

	return result, nil
}

type ConnTuple = struct {
	Saddr_h  uint64
	Saddr_l  uint64
	Daddr_h  uint64
	Daddr_l  uint64
	Sport    uint16
	Dport    uint16
	Netns    uint32
	Pid      uint32
	Metadata uint32
}

type EbpfEvent struct {
	Tuple ConnTuple
	Http  EbpfTx
}
type EbpfTx struct {
	Request_started      uint64
	Response_last_seen   uint64
	Tags                 uint64
	Tcp_seq              uint32
	Response_status_code uint16
	Request_method       uint8
	Pad_cgo_0            [1]byte
	Request_fragment     [208]byte
}

// BenchmarkConsumer benchmarks the consumer with a large number of events to measure the performance.
func BenchmarkConsumer(b *testing.B) {
	const numOfEvents = 1

	program, err := newEBPFProgram(config.New())
	require.NoError(b, err)

	httpReq := "GET /etc/os-release HTTP/1.1\nHost: localhost:8001\nUser-Agent: curl/7.81.0\nAccept: */*"

	consumer, err := NewConsumer[EbpfEvent]("test", program, func([]EbpfEvent) {})
	require.NoError(b, err)

	httpEvent := EbpfEvent{
		Tuple: ConnTuple{
			Saddr_h:  0,
			Saddr_l:  0,
			Daddr_h:  0,
			Daddr_l:  0,
			Sport:    0,
			Dport:    0,
			Netns:    0,
			Pid:      0,
			Metadata: 0,
		},
		Http: EbpfTx{
			Request_started:      12854883222184,
			Response_last_seen:   12854883577725,
			Tags:                 0,
			Tcp_seq:              4294967295,
			Response_status_code: 200,
			Request_method:       1,
			Pad_cgo_0:            [1]byte{},
		},
	}
	copy(httpEvent.Http.Request_fragment[:], httpReq)

	require.NoError(b, program.Start())
	dataForBatch, err := structToBytes(httpEvent, true)
	require.NoError(b, err)

	mockBatch := batch{
		Idx:        0,
		Cpu:        1,
		Len:        uint16(numOfEvents),
		Cap:        uint16(numOfEvents),
		Event_size: uint16(unsafe.Sizeof(httpEvent)),
		Data:       dataForBatch,
	}

	// Reset the events count as the benchmark will run multiple times.
	consumer.eventsCount.Reset()

	var buf bytes.Buffer
	require.NoError(b, binary.Write(&buf, binary.LittleEndian, &mockBatch))

	consumer.handler.(*ddebpf.RingBufferHandler).RecordHandler(&ringbuf.Record{
		RawSample: buf.Bytes(),
	}, nil, nil)

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		consumer.Start()
		require.Eventually(b, func() bool {
			program.Stop(manager.CleanAll)
			return true
		}, 5*time.Second, 100*time.Millisecond)
		require.Equalf(b, numOfEvents, int(consumer.eventsCount.Get()),
			"Not all events were processed correctly, expected %d, got %d, iteration: %d", numOfEvents, consumer.eventsCount.Get(), i)
	}
}
