// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// There are multiple instances of the interner, one per worker (depends on # of virtual CPUs).
	// Most metrics are tagged with the instance ID, however some are left as global
	// Note `New` vs `NewSimple`
	tlmSIREntries = telemetry.NewGauge("dogstatsd", "string_interner_entries", []string{"interner_id"},
		"Number of entries in the string interner")
	tlmSIRBytes = telemetry.NewGauge("dogstatsd", "string_interner_bytes", []string{"interner_id"},
		"Number of bytes stored in the string interner")
	tlmSIRHits = telemetry.NewCounter("dogstatsd", "string_interner_hits", []string{"interner_id"},
		"Number of times string interner returned an existing string")
	tlmSIRMiss = telemetry.NewCounter("dogstatsd", "string_interner_miss", []string{"interner_id"},
		"Number of times string interner created a new string object")
	tlmSIRNew = telemetry.NewSimpleCounter("dogstatsd", "string_interner_new",
		"Number of times string interner was created")
)

// A StringValue pointer is the handle to the underlying string value.
// See Get how Value pointers may be used.
type StringValue struct {
	_           [0]func() // prevent people from accidentally using value type as comparable
	cmpVal      string
	resurrected bool
}

// Get the underlying string value
func (v *StringValue) Get() string {
	return v.cmpVal
}

// stringInterner interns strings while allowing them to be cleaned up by the GC.
// It can handle both string and []byte types without allocation.
type stringInterner struct {
	mu           sync.Mutex
	tlmEnabled   bool
	valMap       map[string]uintptr
	id           string
	totalBytes   uint32
	totalEntries uint32
}

// newStringInterner creates a new StringInterner
func newStringInterner(internerID int) *stringInterner {
	i := &stringInterner{
		valMap:     make(map[string]uintptr),
		tlmEnabled: utils.IsTelemetryEnabled(),
		id:         fmt.Sprintf("interner_%d", internerID),
	}

	if i.tlmEnabled {
		tlmSIRNew.Inc()
	}
	return i
}

// Get returns a pointer representing the []byte k
//
// The returned pointer will be the same for Get(v) and Get(v2)
// if and only if v == v2. The returned pointer will also be the same
// for a string with same contents as the byte slice.
//
//go:nocheckptr
func (s *stringInterner) LoadOrStore(k []byte) *StringValue {
	s.mu.Lock()
	defer s.mu.Unlock()

	var v *StringValue
	// the compiler will optimize the following map lookup to not alloc a string
	if addr, ok := s.valMap[string(k)]; ok {
		//goland:noinspection GoVetUnsafePointer
		v = (*StringValue)((unsafe.Pointer)(addr))
		v.resurrected = true
		if s.tlmEnabled {
			tlmSIRHits.WithTags(map[string]string{"interner_id": s.id}).Inc()
			tlmSIRBytes.WithTags(map[string]string{"interner_id": s.id}).Add(float64(len(k)))
			tlmSIREntries.WithTags(map[string]string{"interner_id": s.id}).Add(1)
		}
		return v
	}

	v = &StringValue{cmpVal: string(k)}
	runtime.SetFinalizer(v, s.finalize)
	s.valMap[string(k)] = uintptr(unsafe.Pointer(v))
	tlmSIRMiss.WithTags(map[string]string{"interner_id": s.id}).Inc()
	return v
}

func (s *stringInterner) finalize(v *StringValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.resurrected {
		// We lost the race. Somebody resurrected it while we
		// were about to finalize it. Try again next round.
		v.resurrected = false
		runtime.SetFinalizer(v, s.finalize)
		return
	}

	deadValue := v.Get()
	tlmSIRBytes.WithTags(map[string]string{"interner_id": s.id}).Sub(float64(len(deadValue)))
	tlmSIREntries.WithTags(map[string]string{"interner_id": s.id}).Sub(1)
	delete(s.valMap, v.Get())
}
