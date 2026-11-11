// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"cmp"
	"fmt"
	"net"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func newDeepCopyTestEntry(pid uint32, binary string) *ProcessCacheEntry {
	entry := NewProcessCacheEntry()
	entry.Pid = pid
	entry.Tid = pid
	entry.Comm = binary
	entry.FileEvent.PathnameStr = "/usr/bin/" + binary
	entry.FileEvent.BasenameStr = binary
	entry.ArgsEntry = &ArgsEntry{Values: []string{binary, "--flag", "value"}}
	entry.EnvsEntry = &EnvsEntry{Values: []string{"PATH=/usr/bin", "HOME=/root"}}
	entry.ContainerContext.ContainerID = "0123456789abcdef"
	entry.ContainerContext.Tags = []string{"image_name:image", "image_tag:tag"}
	entry.CGroup.CGroupID = "/docker/0123456789abcdef"
	return entry
}

// newDeepCopyTestChain returns the last entry of a chain of depth entries linked the way the process
// resolver links them.
func newDeepCopyTestChain(depth int) *ProcessCacheEntry {
	entry := newDeepCopyTestEntry(1, "init")
	for i := 1; i < depth; i++ {
		if i%2 == 1 {
			child := NewProcessCacheEntry()
			child.Pid = entry.Pid + 1
			child.Tid = child.Pid
			entry.Fork(child)
			entry = child
		} else {
			child := newDeepCopyTestEntry(entry.Pid, "binary")
			entry.Exec(child)
			entry = child
		}
	}

	k8sExtra := map[string][]string{"scopes": {"read", "write"}}
	for pce := entry; pce != nil; pce = pce.Ancestor {
		pce.UserSession.K8SExtra = k8sExtra
	}
	return entry
}

// newDeepCopyTestEvent links the event to its process cache entries the way the eBPF probe does.
func newDeepCopyTestEvent(depth int) *Event {
	entry := newDeepCopyTestChain(depth)

	e := &Event{}
	e.Type = uint32(ExecEventType)
	e.ProcessCacheEntry = entry
	e.ProcessContext = &entry.ProcessContext
	e.Exec.Process = &entry.Process
	e.Exit.Process = &entry.Process
	e.PTrace.Tracee = &entry.Ancestor.ProcessContext
	e.Signal.Target = &entry.Ancestor.ProcessContext
	e.Setrlimit.Target = &entry.Ancestor.Ancestor.ProcessContext

	mask := net.CIDRMask(32, 32)
	e.NetworkContext.Source.IPNet = net.IPNet{IP: net.IPv4(10, 0, 0, 1).To4(), Mask: mask}
	e.NetworkContext.Destination.IPNet = net.IPNet{IP: net.IPv4(10, 0, 0, 2).To4(), Mask: mask}
	return e
}

var timeType = reflect.TypeOf(time.Time{})

// filler sets every exported field reachable from a value to a distinct non-zero value, allocating
// every pointer, slice and map, without sharing anything.
type filler struct {
	n     int
	depth map[reflect.Type]int
}

func (f *filler) next() int {
	f.n++
	return f.n
}

func isInterfaceContainer(t reflect.Type) bool {
	return (t.Kind() == reflect.Slice || t.Kind() == reflect.Map) && t.Elem().Kind() == reflect.Interface
}

func (f *filler) fill(v reflect.Value) {
	if v.Type() == timeType {
		v.Set(reflect.ValueOf(time.Unix(int64(f.next()), 0)))
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(f.next()%100 + 1))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(uint64(f.next()%100 + 1))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(f.next()))
	case reflect.String:
		v.SetString(fmt.Sprintf("v%d", f.next()))
	case reflect.Pointer:
		t := v.Type().Elem()
		if f.depth[t] >= 2 {
			return
		}
		f.depth[t]++
		v.Set(reflect.New(t))
		f.fill(v.Elem())
		f.depth[t]--
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if field := v.Type().Field(i); field.IsExported() && !isInterfaceContainer(field.Type) {
				f.fill(v.Field(i))
			}
		}
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			f.fill(v.Index(i))
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 2, 2))
		for i := 0; i < v.Len(); i++ {
			f.fill(v.Index(i))
		}
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		for range 2 {
			key := reflect.New(v.Type().Key()).Elem()
			f.fill(key)
			value := reflect.New(v.Type().Elem()).Elem()
			f.fill(value)
			v.SetMapIndex(key, value)
		}
	}
}

func newFilledEvent() *Event {
	e := &Event{}
	f := filler{depth: make(map[reflect.Type]int)}
	f.fill(reflect.ValueOf(e).Elem())
	return e
}

type memRange struct {
	start, end uintptr
}

type visitKey struct {
	addr uintptr
	typ  reflect.Type
}

// sizeWalker sums the memory reachable through the fields DeepCopy copies, counting memory shared by
// several pointers once.
type sizeWalker struct {
	ranges  []memRange
	visited map[visitKey]bool
}

func (w *sizeWalker) add(addr, size uintptr) {
	if size > 0 {
		w.ranges = append(w.ranges, memRange{addr, addr + size})
	}
}

func (w *sizeWalker) walk(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		key := visitKey{v.Pointer(), v.Type()}
		if w.visited[key] {
			return
		}
		w.visited[key] = true
		w.add(v.Pointer(), v.Type().Elem().Size())
		w.walk(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				w.walk(v.Field(i))
			}
		}
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			w.walk(v.Index(i))
		}
	case reflect.Slice:
		if v.IsNil() {
			return
		}
		w.add(v.Pointer(), uintptr(v.Cap())*v.Type().Elem().Size())
		for i := 0; i < v.Len(); i++ {
			w.walk(v.Index(i))
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			w.walk(iter.Key())
			w.walk(iter.Value())
		}
	case reflect.String:
		w.add(uintptr(unsafe.Pointer(unsafe.StringData(v.String()))), uintptr(v.Len()))
	}
}

// reachableSize returns the number of bytes and of distinct memory blocks reachable from e.
func reachableSize(e *Event) (bytes int, blocks int) {
	w := sizeWalker{visited: make(map[visitKey]bool)}
	w.walk(reflect.ValueOf(e))

	slices.SortFunc(w.ranges, func(a, b memRange) int { return cmp.Compare(a.start, b.start) })
	var cur memRange
	for _, r := range w.ranges {
		if r.start < cur.end {
			cur.end = max(cur.end, r.end)
			continue
		}
		bytes += int(cur.end - cur.start)
		cur = r
		blocks++
	}
	bytes += int(cur.end - cur.start)
	return bytes, blocks
}

// copyChecker walks an original value and its copy side by side and checks that they hold the same
// values, that the copy shares no memory with the original, and that the copy shares memory
// internally exactly where the original does.
type copyChecker struct {
	t          *testing.T
	origToCopy map[uintptr]uintptr
	copyToOrig map[uintptr]uintptr
	visited    map[visitKey]bool
}

func checkCopy(t *testing.T, original, copied *Event) {
	t.Helper()
	c := copyChecker{
		t:          t,
		origToCopy: make(map[uintptr]uintptr),
		copyToOrig: make(map[uintptr]uintptr),
		visited:    make(map[visitKey]bool),
	}
	c.walk("Event", reflect.ValueOf(original), reflect.ValueOf(copied))
}

// record checks that the memory at o in the original is copied to cp, like the other references to o.
func (c *copyChecker) record(path string, o, cp uintptr) bool {
	if o == cp {
		c.t.Errorf("%s: shared with the original", path)
		return false
	}
	if prev, ok := c.origToCopy[o]; ok && prev != cp {
		c.t.Errorf("%s: shared in the original but not in the copy", path)
	}
	if prev, ok := c.copyToOrig[cp]; ok && prev != o {
		c.t.Errorf("%s: shared in the copy but not in the original", path)
	}
	c.origToCopy[o] = cp
	c.copyToOrig[cp] = o
	return true
}

func (c *copyChecker) walk(path string, orig, cpy reflect.Value) {
	if orig.Type() == timeType {
		if !orig.Interface().(time.Time).Equal(cpy.Interface().(time.Time)) {
			c.t.Errorf("%s: %v in the original, %v in the copy", path, orig, cpy)
		}
		return
	}
	switch orig.Kind() {
	case reflect.Bool, reflect.String, reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if !orig.Equal(cpy) {
			c.t.Errorf("%s: %v in the original, %v in the copy", path, orig, cpy)
		}
	case reflect.Pointer, reflect.Map:
		if orig.IsNil() || cpy.IsNil() {
			if orig.IsNil() != cpy.IsNil() {
				c.t.Errorf("%s: nil in only one of the original and the copy", path)
			}
			return
		}
		if !c.record(path, orig.Pointer(), cpy.Pointer()) {
			return
		}
		key := visitKey{orig.Pointer(), orig.Type()}
		if c.visited[key] {
			return
		}
		c.visited[key] = true
		if orig.Kind() == reflect.Pointer {
			c.walk(path, orig.Elem(), cpy.Elem())
			return
		}
		if orig.Len() != cpy.Len() {
			c.t.Errorf("%s: %d entries in the original, %d in the copy", path, orig.Len(), cpy.Len())
			return
		}
		iter := orig.MapRange()
		for iter.Next() {
			value := cpy.MapIndex(iter.Key())
			if !value.IsValid() {
				c.t.Errorf("%s: key %v missing in the copy", path, iter.Key())
				continue
			}
			c.walk(path+"[]", iter.Value(), value)
		}
	case reflect.Struct:
		for i := 0; i < orig.NumField(); i++ {
			if f := orig.Type().Field(i); f.IsExported() {
				c.walk(path+"."+f.Name, orig.Field(i), cpy.Field(i))
			}
		}
	case reflect.Slice:
		if orig.IsNil() != cpy.IsNil() || orig.Len() != cpy.Len() {
			c.t.Errorf("%s: length %d (nil: %t) in the original, %d (nil: %t) in the copy",
				path, orig.Len(), orig.IsNil(), cpy.Len(), cpy.IsNil())
			return
		}
		if orig.Len() > 0 && !c.record(path, orig.Pointer(), cpy.Pointer()) {
			return
		}
		for i := 0; i < orig.Len(); i++ {
			c.walk(path+"[]", orig.Index(i), cpy.Index(i))
		}
	case reflect.Array:
		for i := 0; i < orig.Len(); i++ {
			c.walk(path+"[]", orig.Index(i), cpy.Index(i))
		}
	}
}

func TestEvent_DeepCopySize(t *testing.T) {
	original := newDeepCopyTestEvent(5)
	copied := original.DeepCopy()

	origBytes, origBlocks := reachableSize(original)
	copyBytes, copyBlocks := reachableSize(copied)
	alloc := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = original.DeepCopy()
		}
	})
	t.Logf("sizeof: Event=%d ProcessCacheEntry=%d Process=%d",
		unsafe.Sizeof(Event{}), unsafe.Sizeof(ProcessCacheEntry{}), unsafe.Sizeof(Process{}))
	t.Logf("original: %d bytes in %d blocks", origBytes, origBlocks)
	t.Logf("copy:     %d bytes in %d blocks (DeepCopy allocates %d bytes in %d allocations)",
		copyBytes, copyBlocks, alloc.AllocedBytesPerOp(), alloc.AllocsPerOp())

	assert.Equal(t, origBytes, copyBytes)
	assert.Equal(t, origBlocks, copyBlocks)
}

func TestEvent_DeepCopyKeepsSharedPointers(t *testing.T) {
	original := newDeepCopyTestEvent(5)
	checkCopy(t, original, original.DeepCopy())
}

func TestEvent_DeepCopyCopiesEveryField(t *testing.T) {
	original := newFilledEvent()
	copied := original.DeepCopy()
	checkCopy(t, original, copied)
	assert.True(t, reflect.DeepEqual(original, copied))
}

type nilableField struct {
	path  string
	value reflect.Value
}

// nilableFields returns the non-nil pointers, slices and maps reachable from v. It doesn't look inside
// maps, as map values can't be set in place.
func nilableFields(v reflect.Value, path string) []nilableField {
	var fields []nilableField
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			fields = append(fields, nilableField{path, v})
			fields = append(fields, nilableFields(v.Elem(), path)...)
		}
	case reflect.Slice:
		if !v.IsNil() {
			fields = append(fields, nilableField{path, v})
			for i := 0; i < v.Len(); i++ {
				fields = append(fields, nilableFields(v.Index(i), fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	case reflect.Map:
		if !v.IsNil() {
			fields = append(fields, nilableField{path, v})
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if f := v.Type().Field(i); f.IsExported() {
				fields = append(fields, nilableFields(v.Field(i), path+"."+f.Name)...)
			}
		}
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			fields = append(fields, nilableFields(v.Index(i), fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return fields
}

func deepCopyRecover(e *Event) (copied *Event, panicked any) {
	defer func() { panicked = recover() }()
	return e.DeepCopy(), nil
}

func TestEvent_DeepCopyNilFields(t *testing.T) {
	assert.Nil(t, (*Event)(nil).DeepCopy())
	checkCopy(t, &Event{}, (&Event{}).DeepCopy())

	original := newFilledEvent()
	fields := nilableFields(reflect.ValueOf(original).Elem(), "Event")
	t.Logf("%d pointers, slices and maps", len(fields))
	for _, f := range fields {
		saved := reflect.ValueOf(f.value.Interface())
		f.value.SetZero()
		copied, panicked := deepCopyRecover(original)
		if panicked != nil {
			t.Errorf("%s set to nil: DeepCopy panicked: %v", f.path, panicked)
		} else if !reflect.DeepEqual(original, copied) {
			t.Errorf("%s set to nil: the copy differs from the original", f.path)
		}
		f.value.Set(saved)
	}
}

func evalField(e *Event, field string) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return e.GetFieldValue(field)
}

func TestEvent_DeepCopySECLFields(t *testing.T) {
	for name, newEvent := range map[string]func() *Event{
		"process chain": func() *Event { return newDeepCopyTestEvent(5) },
		"every field":   newFilledEvent,
	} {
		t.Run(name, func(t *testing.T) {
			original := newEvent()
			original.FieldHandlers = &FakeFieldHandlers{PCEs: make(map[uint32]*ProcessCacheEntry)}
			copied := original.DeepCopy()

			var evaluated int
			for _, field := range original.GetFields() {
				want, wantErr := evalField(original, field)
				got, gotErr := evalField(copied, field)
				assert.Equal(t, wantErr, gotErr, field)
				assert.Equal(t, want, got, field)
				if wantErr == nil && !reflect.ValueOf(want).IsZero() {
					evaluated++
				}
			}
			t.Logf("%d fields, %d evaluated to a non-zero value", len(original.GetFields()), evaluated)
		})
	}
}

type processPointerField struct {
	name  string
	index []int
}

// processPointerFields returns the fields of t, outside of pointers, that point to a process.
func processPointerFields(t reflect.Type, name string, index []int) []processPointerField {
	var fields []processPointerField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		fieldName, fieldIndex := strings.TrimPrefix(name+"."+f.Name, "."), append(slices.Clone(index), i)
		switch f.Type {
		case reflect.TypeOf(&ProcessCacheEntry{}), reflect.TypeOf(&ProcessContext{}), reflect.TypeOf(&Process{}):
			fields = append(fields, processPointerField{fieldName, fieldIndex})
		default:
			if f.Type.Kind() == reflect.Struct {
				fields = append(fields, processPointerFields(f.Type, fieldName, fieldIndex)...)
			}
		}
	}
	return fields
}

// pointTo makes v point to entry, or to the part of entry of the type of v.
func pointTo(v reflect.Value, entry *ProcessCacheEntry) {
	switch v.Interface().(type) {
	case *ProcessCacheEntry:
		v.Set(reflect.ValueOf(entry))
	case *ProcessContext:
		v.Set(reflect.ValueOf(&entry.ProcessContext))
	case *Process:
		v.Set(reflect.ValueOf(&entry.Process))
	}
}

func TestEvent_DeepCopyKeepsEverySharedProcess(t *testing.T) {
	var fields []processPointerField
	for _, f := range processPointerFields(reflect.TypeOf(Event{}), "", nil) {
		if f.name != "BaseEvent.ProcessCacheEntry" && f.name != "BaseEvent.ProcessContext" {
			fields = append(fields, f)
		}
	}
	t.Logf("%d fields point to a process", len(fields))

	newEvent := func() (*Event, []*ProcessCacheEntry) {
		entry := newDeepCopyTestChain(5)
		e := &Event{}
		e.ProcessCacheEntry = entry
		e.ProcessContext = &entry.ProcessContext
		return e, []*ProcessCacheEntry{entry, entry.Ancestor, entry.Ancestor.Ancestor.Ancestor}
	}

	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			for i := range 3 {
				original, targets := newEvent()
				pointTo(reflect.ValueOf(original).Elem().FieldByIndex(f.index), targets[i])
				checkCopy(t, original, original.DeepCopy())
			}
		})
	}

	t.Run("all", func(t *testing.T) {
		original, targets := newEvent()
		for i, f := range fields {
			pointTo(reflect.ValueOf(original).Elem().FieldByIndex(f.index), targets[i%len(targets)])
		}
		checkCopy(t, original, original.DeepCopy())
	})
}

func TestEvent_DeepCopyIsolation(t *testing.T) {
	original := newDeepCopyTestEvent(5)
	copied := original.DeepCopy()
	pristine := original.DeepCopy()

	original.ProcessCacheEntry.ArgsEntry.Values[0] = "changed"
	original.ProcessContext.ContainerContext.Tags[0] = "changed"
	original.ProcessContext.UserSession.K8SExtra["scopes"][0] = "changed"
	original.ProcessContext.UserSession.K8SExtra["added"] = []string{"value"}
	original.ProcessContext.Ancestor.Comm = "changed"
	original.ProcessContext.Ancestor.Ancestor = nil
	original.Exec.Process.Pid = 42
	original.NetworkContext.Source.IPNet.IP[3] = 42
	original.NetworkContext.Source.IPNet.Mask[0] = 0
	checkCopy(t, pristine, copied)

	copied.ProcessContext.ContainerContext.Tags = []string{"image_tag:new"}
	assert.Equal(t, []string{"image_tag:new"}, copied.ProcessCacheEntry.ContainerContext.Tags)
	copied.ProcessCacheEntry.Ancestor.Comm = "new"
	assert.Equal(t, "new", copied.ProcessContext.Parent.Comm)

	pce := copied.ProcessCacheEntry
	assert.Same(t, &pce.ProcessContext, copied.ProcessContext)
	assert.Same(t, &pce.Process, copied.Exec.Process)
	assert.Same(t, &pce.Process, copied.Exit.Process)
	assert.Same(t, &pce.Ancestor.Process, pce.Parent)
	assert.Same(t, &pce.Ancestor.ProcessContext, copied.PTrace.Tracee)
	assert.Same(t, &pce.Ancestor.ProcessContext, copied.Signal.Target)
	assert.Same(t, &pce.Ancestor.Ancestor.ProcessContext, copied.Setrlimit.Target)
	assert.Same(t, pce.Ancestor.ArgsEntry, pce.Ancestor.Ancestor.ArgsEntry)
}

func TestEvent_DeepCopyCycles(t *testing.T) {
	t.Run("two entries", func(t *testing.T) {
		a := newDeepCopyTestEntry(1, "a")
		b := newDeepCopyTestEntry(2, "b")
		a.Ancestor, a.Parent = b, &b.Process
		b.Ancestor, b.Parent = a, &a.Process

		original := &Event{}
		original.ProcessCacheEntry = a
		original.ProcessContext = &a.ProcessContext
		copied := original.DeepCopy()

		assert.Same(t, copied.ProcessCacheEntry, copied.ProcessCacheEntry.Ancestor.Ancestor)
		checkCopy(t, original, copied)
	})

	t.Run("self", func(t *testing.T) {
		a := newDeepCopyTestEntry(1, "a")
		a.Ancestor, a.Parent = a, &a.Process

		original := &Event{}
		original.ProcessCacheEntry = a
		original.ProcessContext = &a.ProcessContext
		copied := original.DeepCopy()

		assert.Same(t, copied.ProcessCacheEntry, copied.ProcessCacheEntry.Ancestor)
		checkCopy(t, original, copied)
	})
}

func BenchmarkDeepCopy(b *testing.B) {
	entry := newDeepCopyTestEntry(1, "init")
	singleProcess := &Event{}
	singleProcess.ProcessCacheEntry = entry
	singleProcess.ProcessContext = &entry.ProcessContext
	singleProcess.Exec.Process = &entry.Process

	for _, bench := range []struct {
		name  string
		event *Event
	}{
		{"5_processes", newDeepCopyTestEvent(5)},
		{"1_process", singleProcess},
	} {
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = bench.event.DeepCopy()
			}
		})
	}
}
