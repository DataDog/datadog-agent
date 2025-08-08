//go:build linux_bpf

package decode

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
)

// mockProbeDefinition implements ir.ProbeDefinition for testing
type mockProbeDefinition struct {
	id      string
	version int
	tags    []string
	kind    ir.ProbeKind
	where   mockWhere
}

func (m *mockProbeDefinition) GetID() string         { return m.id }
func (m *mockProbeDefinition) GetVersion() int       { return m.version }
func (m *mockProbeDefinition) GetTags() []string     { return m.tags }
func (m *mockProbeDefinition) GetKind() ir.ProbeKind { return m.kind }
func (m *mockProbeDefinition) GetWhere() ir.Where    { return &m.where }
func (m *mockProbeDefinition) GetCaptureConfig() ir.CaptureConfig {
	return &mockCaptureConfig{}
}
func (m *mockProbeDefinition) GetThrottleConfig() ir.ThrottleConfig {
	return &mockThrottleConfig{}
}

type mockWhere struct {
	location string
}

func (m *mockWhere) Where()                          {}
func (m *mockWhere) Location() (functionName string) { return m.location }

type mockCaptureConfig struct{}

func (m *mockCaptureConfig) GetMaxReferenceDepth() uint32 { return 10 }
func (m *mockCaptureConfig) GetMaxFieldCount() uint32     { return 100 }
func (m *mockCaptureConfig) GetMaxCollectionSize() uint32 { return 100 }

type mockThrottleConfig struct{}

func (m *mockThrottleConfig) GetThrottlePeriodMs() uint32 { return 1000 }
func (m *mockThrottleConfig) GetThrottleBudget() int64    { return 100 }

// mockSymbolicator implements symbol.Symbolicator for testing
type mockSymbolicator struct{}

func (m *mockSymbolicator) Symbolicate(stack []uint64, stackHash uint64) ([]symbol.StackFrame, error) {
	frames := make([]symbol.StackFrame, len(stack))
	for i := range stack {
		frames[i] = symbol.StackFrame{
			Lines: []symbol.StackLine{
				{
					Function: "test.func",
					File:     "test.go",
					Line:     42,
				},
			},
		}
	}
	return frames, nil
}

// Helper function to create aligned byte slices
func alignTo8(b []byte) []byte {
	for i, n := 0, nextMultipleOf8(len(b))-len(b); i < n; i++ {
		b = append(b, 0)
	}
	return b
}

func nextMultipleOf8(v int) int {
	return (v + 7) & ^7
}

// Helper to build test events
func buildTestEvent(
	header *output.EventHeader,
	stack []uint64,
	items []testDataItem,
) []byte {
	eventHeaderSize := int(unsafe.Sizeof(output.EventHeader{}))
	dataItemHeaderSize := int(unsafe.Sizeof(output.DataItemHeader{}))

	b := make([]byte, eventHeaderSize)
	*(*output.EventHeader)(unsafe.Pointer(&b[0])) = *header

	if len(stack) > 0 {
		stackBytes := unsafe.Slice((*byte)(unsafe.Pointer(&stack[0])), len(stack)*8)
		b = append(b, stackBytes...)
	}
	b = alignTo8(b)

	for _, item := range items {
		itemHeaderStart := len(b)
		b = append(b, make([]byte, dataItemHeaderSize)...)
		*(*output.DataItemHeader)(unsafe.Pointer(&b[itemHeaderStart])) = item.header

		b = append(b, item.data...)
		b = alignTo8(b)
	}

	// Update header with actual length
	header.Data_byte_len = uint32(len(b))
	*(*output.EventHeader)(unsafe.Pointer(&b[0])) = *header

	return b
}

type testDataItem struct {
	header output.DataItemHeader
	data   []byte
}

// createTestProgram creates a minimal test program for fuzzing
func createTestProgram() *ir.Program {
	// Create a basic type for the expression
	basicType := &ir.BaseType{
		TypeCommon: ir.TypeCommon{
			ID:       2,
			Name:     "int32",
			ByteSize: 4,
		},
	}

	// Create a basic EventRootType with proper expression
	eventRootType := &ir.EventRootType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "root_event",
			ByteSize: 8, // 4 bytes for bitset + 4 bytes for data
		},
		PresenceBitsetSize: 4,
		Expressions: []*ir.RootExpression{
			{
				Name:   "test_expr",
				Offset: 4, // After the bitset
				Expression: ir.Expression{
					Type: basicType,
					Operations: []ir.ExpressionOp{
						&ir.LocationOp{
							Offset:   0,
							ByteSize: 4,
						},
					},
				},
			},
		},
	}

	// Create a mock probe
	mockProbe := &mockProbeDefinition{
		id:      "test-probe-1",
		version: 1,
		tags:    []string{"test"},
		kind:    ir.ProbeKindSnapshot,
		where:   mockWhere{location: "main.test"},
	}

	// Create an event
	event := &ir.Event{
		ID:   1,
		Type: eventRootType,
	}

	// Create a probe
	probe := &ir.Probe{
		ProbeDefinition: mockProbe,
		Events:          []*ir.Event{event},
	}

	return &ir.Program{
		ID:     1,
		Probes: []*ir.Probe{probe},
		Types: map[ir.TypeID]ir.Type{
			1: eventRootType,
			2: basicType,
		},
		MaxTypeID: 2,
	}
}

// Seeds for the fuzzer based on common patterns
var fuzzSeeds = []struct {
	name   string
	header output.EventHeader
	stack  []uint64
	items  []testDataItem
}{
	{
		name: "basic_event",
		header: output.EventHeader{
			Prog_id:        1,
			Stack_byte_len: 16,
			Stack_hash:     12345,
			Ktime_ns:       1000000,
		},
		stack: []uint64{0x1000, 0x2000},
		items: []testDataItem{
			{
				header: output.DataItemHeader{Type: 1, Length: 8, Address: 0x100},
				data:   []byte{0x01, 0x00, 0x00, 0x00, 0x42, 0x00, 0x00, 0x00}, // bitset + int32 value
			},
		},
	},
	{
		name: "empty_stack",
		header: output.EventHeader{
			Prog_id:        1,
			Stack_byte_len: 0,
			Stack_hash:     0,
			Ktime_ns:       2000000,
		},
		stack: nil,
		items: []testDataItem{
			{
				header: output.DataItemHeader{Type: 1, Length: 8, Address: 0x200},
				data:   []byte{0x00, 0x00, 0x00, 0x00, 0xAA, 0xBB, 0xCC, 0xDD}, // bitset + data
			},
		},
	},
	{
		name: "multiple_items",
		header: output.EventHeader{
			Prog_id:        1,
			Stack_byte_len: 8,
			Stack_hash:     54321,
			Ktime_ns:       3000000,
		},
		stack: []uint64{0x3000},
		items: []testDataItem{
			{
				header: output.DataItemHeader{Type: 1, Length: 8, Address: 0x300},
				data:   []byte{0x01, 0x00, 0x00, 0x00, 0xFF, 0xFE, 0x00, 0x00}, // bitset + data
			},
		},
	},
}

func FuzzDecode(f *testing.F) {
	// Add seed corpus based on existing test patterns
	for _, seed := range fuzzSeeds {
		eventData := buildTestEvent(&seed.header, seed.stack, seed.items)
		serviceName := "test-service"
		f.Add(eventData, serviceName)
	}

	// Create test program and decoder once for all fuzz iterations
	program := createTestProgram()
	decoder, err := NewDecoder(program)
	if err != nil {
		f.Fatalf("Failed to create decoder: %v", err)
	}

	symbolicator := &mockSymbolicator{}

	f.Fuzz(func(t *testing.T, eventData []byte, serviceName string) {
		event := Event{
			Event:       output.Event(eventData),
			ServiceName: serviceName,
		}

		var buf bytes.Buffer
		probe, err := decoder.Decode(event, symbolicator, &buf)

		header, _ := event.Event.Header()
		if header != nil {
			testStack := []uint64{0x1000, 0x2000, 0x3000}
			_, _ = symbolicator.Symbolicate(testStack, header.Stack_hash)
		}

		_, _ = event.StackPCs()
		for item, itemErr := range event.Event.DataItems() {
			if itemErr != nil {
				continue
			}
			_ = item.Header()
			_ = item.Data()
		}

		_, _ = event.Event.FirstDataItemHeader()

		if err == nil && probe != nil {
			_ = probe.GetID()
			_ = probe.GetVersion()
			_ = probe.GetTags()
			_ = probe.GetKind()
			_ = probe.GetWhere()
			_ = probe.GetCaptureConfig()
			_ = probe.GetThrottleConfig()

			where := probe.GetWhere()
			if funcWhere, ok := where.(ir.FunctionWhere); ok {
				_ = funcWhere.Location()
			}
		}
	})
}
