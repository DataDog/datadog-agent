// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"net"
	"net/netip"
	"reflect"
	"sort"
	"testing"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// TestGeneratedReadersAgreeWithEvaluators is what makes replacing the accessors
// defensible: for every SECL field, on every element of every iterated field,
// the generated reader must return what the evaluator the accessors return
// would.
//
// The two are generated from the same field data, so this checks that they stay
// two renderings of one contract — the same `check:` guards, the same handlers,
// the same conversions and defaults — rather than two implementations that have
// to be kept in step.
func TestGeneratedReadersAgreeWithEvaluators(t *testing.T) {
	var m model.Model
	event := populatedEvent()
	ctx := eval.NewContext(event)

	var direct, iterated int
	for _, field := range sortedKeys(celReaders) {
		reader := celReaders[field]
		prefix := ModelFieldTypes{}.ListPrefix(field)

		if prefix == "" {
			evaluator, err := m.GetEvaluator(field, "", 0)
			require.NoError(t, err, "field %q has a reader but no evaluator", field)

			assertSameValue(t, field, evaluator.Eval(ctx), reader(ctx, nil))
			direct++
			continue
		}
		iterated++

		// A field of an iterated element is read by the accessors through a
		// register holding a position, and by the reader from the element itself.
		// Walking the elements compares them at every position.
		evaluator, err := m.GetEvaluator(field, prefix, 0)
		require.NoError(t, err, "field %q has a reader but no evaluator", field)

		var elements int
		cursor := celIterators[prefix](ctx)
		for pos := 0; ; pos++ {
			element := cursor.next()
			if element == nil {
				break
			}
			ctx.Registers[prefix] = pos

			want := evaluator.Eval(ctx)
			if !(ModelFieldTypes{}).IsListLeaf(field) {
				// Read through a register, a scalar field arrives as a one element
				// slice holding the element at that position.
				want = onlyElement(t, field, want)
			}

			assertSameValue(t, field, want, reader(ctx, element))
			elements++
		}
		require.Positive(t, elements, "field %q was compared on no element", field)
	}

	// Every reader was compared, which is the coverage claim this test makes.
	require.Equal(t, len(celReaders), direct+iterated)
	require.Greater(t, direct, 1000, "expected the whole field set to be covered")
	require.Greater(t, iterated, 100, "expected the iterated fields to be covered")
}

// TestGeneratedReadersCoverTheTypeTree checks the other half: the readers and the
// types must describe the same namespace, member for member. They are two
// outputs of one generator run, and an expression type-checked against one is
// evaluated against the other.
func TestGeneratedReadersCoverTheTypeTree(t *testing.T) {
	// Consumed as they are matched, so what is left over is what has a reader but
	// nothing to reach it through.
	unclaimedReaders := make(map[string]bool, len(celReaders))
	for field := range celReaders {
		unclaimedReaders[field] = true
	}
	unclaimedIterators := make(map[string]bool, len(celIterators))
	for field := range celIterators {
		unclaimedIterators[field] = true
	}

	// Every root's type must describe the root's own path, which is what makes
	// joining a type's path with a member name give the field.
	for name, rootType := range modelRoots {
		require.Equal(t, types.StructKind, rootType.Kind(), "root %q is not an object", name)
		assert.Equal(t, name, modelPaths[rootType.TypeName()], "the type of root %q describes another path", name)
	}

	for typeName, members := range modelShapes {
		path, ok := modelPaths[typeName]
		require.True(t, ok, "type %q describes no path", typeName)

		for member, memberType := range members {
			field := join(path, member)

			elem, isObjectList := objectListElem(memberType)
			switch {
			case isObjectList:
				assert.True(t, unclaimedIterators[field] || celIterators[field] != nil,
					"%q is typed as iterated but has no cursor", field)
				assert.Equal(t, field, modelPaths[elem.TypeName()],
					"the element type of %q describes another path", field)
				delete(unclaimedIterators, field)

			case memberType.Kind() == types.StructKind:
				assert.Equal(t, field, modelPaths[memberType.TypeName()],
					"the type of %q describes another path", field)

			default:
				assert.True(t, unclaimedReaders[field] || celReaders[field] != nil,
					"%q is typed but has no reader", field)
				delete(unclaimedReaders, field)
			}
		}
	}

	assert.Empty(t, unclaimedReaders, "readers no type reaches")
	assert.Empty(t, unclaimedIterators, "cursors no type reaches")
}

// TestCIDRConversion pins the conversion the readers use for IP and CIDR fields,
// which is the one place a reader does more than name a struct field.
func TestCIDRConversion(t *testing.T) {
	tests := []struct {
		name  string
		ipnet net.IPNet
		want  string
	}{
		{"v4 address", ipnet("10.0.0.1/32"), "10.0.0.1/32"},
		{"v4 network", ipnet("10.0.0.0/8"), "10.0.0.0/8"},
		{"v6 address", ipnet("2001:db8::1/128"), "2001:db8::1/128"},
		// The model holds a v4 address in 16 bytes, so its mask does not fit the
		// unmapped address and the prefix has to be rebuilt.
		{"v4 in 16 bytes", net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(128, 128)}, "10.0.0.1/32"},
		// An unset field matches nothing rather than raising an error.
		{"unset", net.IPNet{}, "invalid Prefix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := cidrToVal(tt.ipnet)
			cidr, ok := val.(ext.CIDR)
			require.True(t, ok, "got %s", val)
			assert.Equal(t, tt.want, cidr.Prefix.String())
		})
	}

	// The unset case has to be inert, not just non-erroring: it is what a field
	// of an event that carries no address reads as.
	unset, ok := cidrToVal(net.IPNet{}).(ext.CIDR)
	require.True(t, ok)
	assert.False(t, unset.Prefix.Contains(mustAddr(t, "10.0.0.1")))
}

// TestIndexedAncestorsIgnoreTheirRoot records a bug in the model that the
// readers do not reproduce, so that the difference is a decision on the record
// rather than a surprise for the differential harness.
//
// ProcessAncestorsIterator.At and .Len read ev.ProcessContext.Ancestor instead
// of the root they were constructed with, while .Front and .Next read the root.
// The model has four ancestries — the process one and the ptrace, signal and
// setrlimit targets — so for the three that are not the process one, the two
// forms of the same rule disagree today: the implicit
// `ptrace.tracee.ancestors.comm == "x"` reads the tracee's ancestry through
// Front/Next, and `ptrace.tracee.ancestors[A].comm == "x"` reads the process's
// through At.
//
// A cursor only ever walks Front/Next, so the readers always read the ancestry
// the field names. Fixing the model is a separate change: it lives in
// //pkg/security/secl, which is synchronised into seclwin, and it changes what
// existing rules match.
func TestIndexedAncestorsIgnoreTheirRoot(t *testing.T) {
	event := model.NewFakeEvent()
	event.Init()
	ancestry(event, []string{"process-ancestor"}, []uint32{1})
	event.PTrace.Tracee = &model.ProcessContext{
		Ancestor: &model.ProcessCacheEntry{
			ProcessContext: model.ProcessContext{Process: model.Process{Comm: "tracee-ancestor"}},
		},
	}

	var m model.Model
	ctx := eval.NewContext(event)
	const field = "ptrace.tracee.ancestors.comm"

	implicit, err := m.GetEvaluator(field, "", 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"tracee-ancestor"}, implicit.Eval(ctx),
		"walked with Front/Next, the field reads the ancestry it names")

	indexed, err := m.GetEvaluator(field, "A", 0)
	require.NoError(t, err)
	ctx.Registers["A"] = 0
	assert.Equal(t, []string{"process-ancestor"}, indexed.Eval(ctx),
		"walked with At, it reads the process ancestry instead")

	// The reader agrees with the form that reads the ancestry the field names.
	element := celIterators["ptrace.tracee.ancestors"](ctx).next()
	require.NotNil(t, element)
	assert.Equal(t, types.String("tracee-ancestor"), celReaders[field](ctx, element))
}

// populatedEvent is an event with enough set on it that the readers and the
// evaluators are compared on values rather than only on zeroes.
func populatedEvent() *model.Event {
	event := model.NewFakeEvent()
	// The accessors assume the pointer members of the event are allocated, which
	// is what the rule engine guarantees them. Without it a field of an event
	// type that did not happen dereferences nil — in both engines alike, so it is
	// nothing this comparison can say anything about.
	event.Init()

	process := &event.BaseEvent.ProcessContext.Process
	process.Comm = "sh"
	process.Credentials.UID = 1000
	process.Credentials.User = "root"
	process.FileEvent.BasenameStr = "bash"
	process.FileEvent.PathnameStr = "/usr/bin/bash"
	process.ContainerContext.Tags = []string{"env:prod", "service:web"}
	process.Envp = []string{"PATH=/bin", "HOME=/root"}

	event.BaseEvent.ProcessContext.Parent = &model.Process{Comm: "sshd"}
	ancestry(event, []string{"bash", "sshd", "init"}, []uint32{1000, 0, 0})

	// The other ancestries are given the same chain rather than one of their own,
	// because the accessors would not read theirs: see
	// TestIndexedAncestorsIgnoreTheirRoot.
	ancestors := event.BaseEvent.ProcessContext.Ancestor
	event.PTrace.Tracee = &model.ProcessContext{Ancestor: ancestors}
	event.Signal.Target = &model.ProcessContext{Ancestor: ancestors}
	event.Setrlimit.Target = &model.ProcessContext{Ancestor: ancestors}

	event.NetworkFlowMonitor.Flows = []model.Flow{
		{Source: model.IPPortContext{Port: 4242}, Destination: model.IPPortContext{Port: 443}},
		{Source: model.IPPortContext{Port: 4243}, Destination: model.IPPortContext{Port: 80}},
	}

	return event
}

// assertSameValue compares what an evaluator returned with what a reader
// returned for the same field.
func assertSameValue(t *testing.T, field string, want any, got ref.Val) {
	t.Helper()

	require.False(t, types.IsError(got), "reading %q: %s", field, got)

	// An IP or CIDR is the one shape the two engines hold differently, so the
	// comparison goes through the conversion the readers use. TestCIDRConversion
	// is what pins that conversion itself.
	switch typed := want.(type) {
	case net.IPNet:
		assert.Equal(t, types.True, cidrToVal(typed).Equal(got), "value of %q", field)
		return
	case []net.IPNet:
		assert.Equal(t, types.True, cidrsToVal(typed).Equal(got), "value of %q", field)
		return
	}

	assert.Equal(t, emptied(want), nativeOf(t, field, got), "value of %q", field)
}

// emptied normalises the nil slice an evaluator returns for an empty field into
// the empty one it returns elsewhere for the same field, which a CEL list cannot
// tell apart.
func emptied(value any) any {
	reflected := reflect.ValueOf(value)
	if reflected.Kind() == reflect.Slice && reflected.IsNil() {
		return reflect.MakeSlice(reflected.Type(), 0, 0).Interface()
	}
	return value
}

// nativeOf renders a CEL value as the Go value a SECL evaluator would have
// returned for the same field.
func nativeOf(t *testing.T, field string, val ref.Val) any {
	t.Helper()

	switch typed := val.(type) {
	case types.String:
		return string(typed)
	case types.Int:
		return int(typed)
	case types.Bool:
		return bool(typed)
	case traits.Lister:
		return nativeListOf(t, field, typed)
	}

	require.Fail(t, "unexpected CEL value", "field %q holds %s", field, val.Type())
	return nil
}

// nativeListOf renders a CEL list, typed by its first element because an empty
// list carries no element type of its own.
func nativeListOf(t *testing.T, field string, list traits.Lister) any {
	t.Helper()

	size, ok := list.Size().(types.Int)
	require.True(t, ok, "size of %q", field)

	elements := make([]any, 0, size)
	for i := types.Int(0); i < size; i++ {
		elements = append(elements, nativeOf(t, field, list.Get(i)))
	}

	// The evaluators return a typed slice, so the comparison needs one too.
	switch {
	case len(elements) == 0:
		// An empty list is compared against whichever empty slice the evaluator
		// returned, which the caller normalises.
		return emptySliceLike(t, field)
	default:
		return typedSlice(elements)
	}
}

// emptySliceLike returns the empty slice a SECL evaluator returns for an empty
// field, which depends only on the field's type.
func emptySliceLike(t *testing.T, field string) any {
	t.Helper()

	fieldType, _, ok := walkModel(field)
	require.True(t, ok, "field %q is not typed", field)
	require.Equal(t, types.ListKind, fieldType.Kind(), "field %q is not a list", field)

	switch fieldType.Parameters()[0] {
	case types.IntType:
		return []int{}
	case types.BoolType:
		return []bool{}
	default:
		return []string{}
	}
}

// typedSlice turns the elements of a CEL list back into the typed slice a SECL
// evaluator returns.
func typedSlice(elements []any) any {
	switch elements[0].(type) {
	case int:
		values := make([]int, 0, len(elements))
		for _, element := range elements {
			values = append(values, element.(int))
		}
		return values
	case bool:
		values := make([]bool, 0, len(elements))
		for _, element := range elements {
			values = append(values, element.(bool))
		}
		return values
	default:
		values := make([]string, 0, len(elements))
		for _, element := range elements {
			values = append(values, element.(string))
		}
		return values
	}
}

// onlyElement unwraps the one element slice a scalar field yields when read
// through a register.
func onlyElement(t *testing.T, field string, value any) any {
	t.Helper()

	switch typed := value.(type) {
	case []string:
		require.Len(t, typed, 1, "field %q", field)
		return typed[0]
	case []int:
		require.Len(t, typed, 1, "field %q", field)
		return typed[0]
	case []bool:
		require.Len(t, typed, 1, "field %q", field)
		return typed[0]
	case []net.IPNet:
		require.Len(t, typed, 1, "field %q", field)
		return typed[0]
	}

	require.Fail(t, "unexpected register read", "field %q holds %T", field, value)
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mustAddr(t *testing.T, addr string) netip.Addr {
	t.Helper()

	parsed, err := netip.ParseAddr(addr)
	require.NoError(t, err)
	return parsed
}

func ipnet(cidr string) net.IPNet {
	_, parsed, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return *parsed
}
