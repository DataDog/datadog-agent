// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"container/list"
	"fmt"
	"net"
	"reflect"
	"syscall"
)

var legacyFields = map[Field]Field{
	"process.legacy_name": "process.name",
}

type testItem struct {
	key   int
	value string
	flag  bool
}

type testProcess struct {
	name      string
	argv0     string
	uid       int
	gid       int
	pid       int
	isRoot    bool
	list      *list.List
	array     []*testItem
	createdAt int64

	// overridden values
	orName        string
	orNameValues  func() *StringValues
	orArray       []*testItem
	orArrayValues func() *StringValues
}

type testOpen struct {
	filename string
	mode     int
	flags    int
	openedAt int64
}

type testMkdir struct {
	filename string
	mode     int
}

type testNetwork struct {
	ip    net.IPNet
	ips   []net.IPNet
	cidr  net.IPNet
	cidrs []net.IPNet
}

type testEvent struct {
	id     string
	kind   string
	retval int

	process testProcess
	network testNetwork
	open    testOpen
	mkdir   testMkdir

	listEvaluated bool
	uidEvaluated  bool
	gidEvaluated  bool
}

type testModel struct {
}

func (e *testEvent) GetType() string {
	return e.kind
}

func (e *testEvent) GetTags() []string {
	return []string{}
}

func (m *testModel) NewEvent() Event {
	return &testEvent{}
}

func (m *testModel) ValidateField(key string, value FieldValue) error {
	switch key {
	case "process.uid":
		uid, ok := value.Value.(int)
		if !ok {
			return fmt.Errorf("invalid type for process.ui: %v", reflect.TypeOf(value.Value))
		}

		if uid < 0 {
			return fmt.Errorf("process.uid cannot be negative: %d", uid)
		}
	}

	return nil
}

func (m *testModel) GetFieldRestrictions(_ Field) []EventType {
	return nil
}

func (m *testModel) GetIteratorLen(field Field) (func(ctx *Context) int, error) {
	switch field {

	case "process.list":
		return func(ctx *Context) int {
			return ctx.Event.(*testEvent).process.list.Len()
		}, nil
	case "process.array":
		return func(ctx *Context) int {
			return len(ctx.Event.(*testEvent).process.array)
		}, nil
	}
	return nil, &ErrFieldNotFound{Field: field}
}

func (m *testModel) GetEvaluator(field Field, regID RegisterID) (Evaluator, error) {
	switch field {

	case "network.ip":

		return &CIDREvaluator{
			EvalFnc: func(ctx *Context) net.IPNet {
				return ctx.Event.(*testEvent).network.ip
			},
			Field: field,
		}, nil

	case "network.cidr":

		return &CIDREvaluator{
			EvalFnc: func(ctx *Context) net.IPNet {
				return ctx.Event.(*testEvent).network.cidr
			},
			Field: field,
		}, nil

	case "network.ips":

		return &CIDRArrayEvaluator{
			EvalFnc: func(ctx *Context) []net.IPNet {
				var ipnets []net.IPNet
				ipnets = append(ipnets, ctx.Event.(*testEvent).network.ips...)
				return ipnets
			},
		}, nil

	case "network.cidrs":

		return &CIDRArrayEvaluator{
			EvalFnc: func(ctx *Context) []net.IPNet {
				return ctx.Event.(*testEvent).network.cidrs
			},
			Field: field,
		}, nil

	case "process.name":

		return &StringEvaluator{
			EvalFnc:     func(ctx *Context) string { return ctx.Event.(*testEvent).process.name },
			Field:       field,
			OpOverrides: GlobCmp,
		}, nil

	case "process.argv0":

		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string { return ctx.Event.(*testEvent).process.argv0 },
			Field:   field,
		}, nil

	case "process.uid":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				// to test optimisation
				ctx.Event.(*testEvent).uidEvaluated = true

				return ctx.Event.(*testEvent).process.uid
			},
			Field: field,
		}, nil

	case "process.gid":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				// to test optimisation
				ctx.Event.(*testEvent).gidEvaluated = true

				return ctx.Event.(*testEvent).process.gid
			},
			Field: field,
		}, nil

	case "process.pid":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				// to test optimisation
				ctx.Event.(*testEvent).uidEvaluated = true

				return ctx.Event.(*testEvent).process.pid
			},
			Field: field,
		}, nil

	case "process.is_root":

		return &BoolEvaluator{
			EvalFnc: func(ctx *Context) bool { return ctx.Event.(*testEvent).process.isRoot },
			Field:   field,
		}, nil

	case "process.list.length":
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				return ctx.Event.(*testEvent).process.list.Len()
			},
			Field: field,
		}, nil

	case "process.list.key":

		if regID != "" {
			return &IntArrayEvaluator{
				EvalFnc: func(ctx *Context) []int {
					idx := ctx.Registers[regID]

					var i int

					el := ctx.Event.(*testEvent).process.list.Front()
					for el != nil {
						if i == idx {
							return []int{el.Value.(*testItem).key}
						}
						el = el.Next()
						i++
					}

					return nil
				},
				Field:  field,
				Weight: IteratorWeight,
			}, nil
		}

		return &IntArrayEvaluator{
			EvalFnc: func(ctx *Context) []int {
				// to test optimisation
				ctx.Event.(*testEvent).listEvaluated = true

				var result []int

				el := ctx.Event.(*testEvent).process.list.Front()
				for el != nil {
					result = append(result, el.Value.(*testItem).key)
					el = el.Next()
				}

				return result
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.list.value":

		if regID != "" {
			return &StringArrayEvaluator{
				EvalFnc: func(ctx *Context) []string {
					idx := ctx.Registers[regID]

					var i int

					el := ctx.Event.(*testEvent).process.list.Front()
					for el != nil {
						if i == idx {
							return []string{el.Value.(*testItem).value}
						}
						el = el.Next()
						i++
					}

					return nil
				},
				Field:  field,
				Weight: IteratorWeight,
			}, nil
		}

		return &StringArrayEvaluator{
			EvalFnc: func(ctx *Context) []string {
				// to test optimisation
				ctx.Event.(*testEvent).listEvaluated = true

				var values []string

				el := ctx.Event.(*testEvent).process.list.Front()
				for el != nil {
					values = append(values, el.Value.(*testItem).value)
					el = el.Next()
				}

				return values
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.list.flag":

		if regID != "" {
			return &BoolArrayEvaluator{
				EvalFnc: func(ctx *Context) []bool {
					idx := ctx.Registers[regID]

					var i int

					el := ctx.Event.(*testEvent).process.list.Front()
					for el != nil {
						if i == idx {
							return []bool{el.Value.(*testItem).flag}
						}
						el = el.Next()
						i++
					}

					return nil
				},
				Field:  field,
				Weight: IteratorWeight,
			}, nil
		}

		return &BoolArrayEvaluator{
			EvalFnc: func(ctx *Context) []bool {
				// to test optimisation
				ctx.Event.(*testEvent).listEvaluated = true

				var result []bool

				el := ctx.Event.(*testEvent).process.list.Front()
				for el != nil {
					result = append(result, el.Value.(*testItem).flag)
					el = el.Next()
				}

				return result
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.array.length":
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				return len(ctx.Event.(*testEvent).process.array)
			},
			Field: field,
		}, nil

	case "process.array.key":

		if regID != "" {
			return &IntArrayEvaluator{
				EvalFnc: func(ctx *Context) []int {
					idx := ctx.Registers[regID]

					return []int{ctx.Event.(*testEvent).process.array[idx].key}
				},
				Field:  field,
				Weight: IteratorWeight,
			}, nil
		}

		return &IntArrayEvaluator{
			EvalFnc: func(ctx *Context) []int {
				var result []int

				for _, el := range ctx.Event.(*testEvent).process.array {
					result = append(result, el.key)
				}

				return result
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.array.value":

		if regID != "" {
			return &StringArrayEvaluator{
				EvalFnc: func(ctx *Context) []string {
					idx := ctx.Registers[regID]

					return []string{ctx.Event.(*testEvent).process.array[idx].value}
				},
				Field:  field,
				Weight: IteratorWeight,
			}, nil
		}

		return &StringArrayEvaluator{
			EvalFnc: func(ctx *Context) []string {
				var values []string

				for _, el := range ctx.Event.(*testEvent).process.array {
					values = append(values, el.value)
				}

				return values
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.array.flag":

		return &BoolArrayEvaluator{
			EvalFnc: func(ctx *Context) []bool {
				var result []bool

				for _, el := range ctx.Event.(*testEvent).process.array {
					result = append(result, el.flag)
				}

				return result
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.created_at":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				return int(ctx.Event.(*testEvent).process.createdAt)
			},
			Field: field,
		}, nil

	case "process.or_name":

		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string {
				return ctx.Event.(*testEvent).process.orName
			},
			Field: field,
			OpOverrides: &OpOverrides{
				StringValuesContains: func(a *StringEvaluator, _ *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
					evaluator := StringValuesEvaluator{
						EvalFnc: func(ctx *Context) *StringValues {
							return ctx.Event.(*testEvent).process.orNameValues()
						},
					}

					return StringValuesContains(a, &evaluator, state)
				},
				StringEquals: func(a *StringEvaluator, _ *StringEvaluator, state *State) (*BoolEvaluator, error) {
					evaluator := StringValuesEvaluator{
						EvalFnc: func(ctx *Context) *StringValues {
							return ctx.Event.(*testEvent).process.orNameValues()
						},
					}

					return StringValuesContains(a, &evaluator, state)
				},
			},
		}, nil

	case "process.or_array.value":

		return &StringArrayEvaluator{
			EvalFnc: func(ctx *Context) []string {
				var values []string

				for _, el := range ctx.Event.(*testEvent).process.orArray {
					values = append(values, el.value)
				}

				return values
			},
			Field: field,
			OpOverrides: &OpOverrides{
				StringArrayContains: func(_ *StringEvaluator, b *StringArrayEvaluator, state *State) (*BoolEvaluator, error) {
					evaluator := StringValuesEvaluator{
						EvalFnc: func(ctx *Context) *StringValues {
							return ctx.Event.(*testEvent).process.orArrayValues()
						},
					}

					return StringArrayMatches(b, &evaluator, state)
				},
				StringArrayMatches: func(a *StringArrayEvaluator, _ *StringValuesEvaluator, state *State) (*BoolEvaluator, error) {
					evaluator := StringValuesEvaluator{
						EvalFnc: func(ctx *Context) *StringValues {
							return ctx.Event.(*testEvent).process.orArrayValues()
						},
					}

					return StringArrayMatches(a, &evaluator, state)
				},
			},
		}, nil

	case "open.filename":

		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string { return ctx.Event.(*testEvent).open.filename },
			Field:   field,
		}, nil

	case "open.flags":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int { return ctx.Event.(*testEvent).open.flags },
			Field:   field,
		}, nil

	case "open.mode":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int { return ctx.Event.(*testEvent).open.mode },
			Field:   field,
		}, nil

	case "open.opened_at":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				return int(ctx.Event.(*testEvent).open.openedAt)
			},
			Field: field,
		}, nil

	case "retval":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int { return ctx.Event.(*testEvent).retval },
			Field:   field,
		}, nil

	case "mkdir.filename":

		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string { return ctx.Event.(*testEvent).mkdir.filename },
			Field:   field,
		}, nil

	case "mkdir.mode":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int { return ctx.Event.(*testEvent).mkdir.mode },
			Field:   field,
		}, nil
	}

	return nil, &ErrFieldNotFound{Field: field}
}

func (e *testEvent) Init() {}

func (e *testEvent) GetFieldValue(field Field) (interface{}, error) {
	switch field {

	case "network.ip":
		return e.network.ip, nil

	case "network.ips":
		return e.network.ips, nil

	case "network.cidr":
		return e.network.cidr, nil

	case "network.cidrs":
		return e.network.cidrs, nil

	case "process.name":

		return e.process.name, nil

	case "process.argv0":

		return e.process.argv0, nil

	case "process.uid":

		return e.process.uid, nil

	case "process.gid":

		return e.process.gid, nil

	case "process.pid":

		return e.process.pid, nil

	case "process.is_root":

		return e.process.isRoot, nil

	case "process.created_at":

		return e.process.createdAt, nil

	case "open.filename":

		return e.open.filename, nil

	case "retval":

		return e.retval, nil

	case "open.flags":

		return e.open.flags, nil

	case "open.mode":

		return e.open.mode, nil

	case "open.opened_at":

		return e.open.openedAt, nil

	case "mkdir.filename":

		return e.mkdir.filename, nil

	case "mkdir.mode":

		return e.mkdir.mode, nil

	}

	return nil, &ErrFieldNotFound{Field: field}
}

func (e *testEvent) GetFieldEventType(field Field) (string, error) {
	switch field {

	case "network.ip":

		return "network", nil

	case "network.ips":

		return "network", nil

	case "network.cidr":

		return "network", nil

	case "network.cidrs":

		return "network", nil

	case "process.name":

		return "", nil

	case "process.argv0":

		return "", nil

	case "process.uid":

		return "", nil

	case "process.gid":

		return "", nil

	case "process.pid":

		return "", nil

	case "process.is_root":

		return "", nil

	case "process.list.key":

		return "", nil

	case "process.list.value":

		return "", nil

	case "process.list.flag":

		return "", nil

	case "process.array.key":

		return "", nil

	case "process.array.value":

		return "", nil

	case "process.array.flag":

		return "", nil

	case "process.created_at":

		return "", nil

	case "process.or_name":

		return "", nil

	case "process.or_array.value":

		return "", nil

	case "open.filename":

		return "open", nil

	case "retval":

		return "", nil

	case "open.flags":

		return "open", nil

	case "open.mode":

		return "open", nil

	case "open.opened_at":

		return "open", nil

	case "mkdir.filename":

		return "mkdir", nil

	case "mkdir.mode":

		return "mkdir", nil

	}

	return "", &ErrFieldNotFound{Field: field}
}

func (e *testEvent) SetFieldValue(field Field, value interface{}) error {
	switch field {

	case "network.ip":

		e.network.ip = value.(net.IPNet)
		return nil

	case "network.ips":

		e.network.ips = value.([]net.IPNet)

	case "network.cidr":

		e.network.cidr = value.(net.IPNet)
		return nil

	case "network.cidrs":

		e.network.cidrs = value.([]net.IPNet)
		return nil

	case "process.name":

		e.process.name = value.(string)
		return nil

	case "process.argv0":

		e.process.argv0 = value.(string)
		return nil

	case "process.uid":

		e.process.uid = value.(int)
		return nil

	case "process.gid":

		e.process.gid = value.(int)
		return nil

	case "process.pid":

		e.process.pid = value.(int)
		return nil

	case "process.is_root":

		e.process.isRoot = value.(bool)
		return nil

	case "process.created_at":

		e.process.createdAt = value.(int64)
		return nil

	case "open.filename":

		e.open.filename = value.(string)
		return nil

	case "retval":

		e.retval = value.(int)
		return nil

	case "open.flags":

		e.open.flags = value.(int)
		return nil

	case "open.mode":

		e.open.mode = value.(int)
		return nil

	case "open.opened_at":

		e.open.openedAt = value.(int64)
		return nil

	case "mkdir.filename":

		e.mkdir.filename = value.(string)
		return nil

	case "mkdir.mode":

		e.mkdir.mode = value.(int)
		return nil

	}

	return &ErrFieldNotFound{Field: field}
}

func (e *testEvent) GetFieldType(field Field) (reflect.Kind, error) {
	switch field {

	case "network.ip":

		return reflect.Struct, nil

	case "network.ips":

		return reflect.Array, nil

	case "network.cidr":

		return reflect.Struct, nil

	case "network.cidrs":

		return reflect.Array, nil

	case "process.name":

		return reflect.String, nil

	case "process.argv0":

		return reflect.String, nil

	case "process.uid":

		return reflect.Int, nil

	case "process.gid":

		return reflect.Int, nil

	case "process.pid":

		return reflect.Int, nil

	case "process.is_root":

		return reflect.Bool, nil

	case "process.list.key":
		return reflect.Int, nil

	case "process.list.value":
		return reflect.Int, nil

	case "process.list.flag":
		return reflect.Bool, nil

	case "process.array.key":
		return reflect.Int, nil

	case "process.array.value":
		return reflect.String, nil

	case "process.array.flag":
		return reflect.Bool, nil

	case "open.filename":

		return reflect.String, nil

	case "retval":

		return reflect.Int, nil

	case "open.flags":

		return reflect.Int, nil

	case "open.mode":

		return reflect.Int, nil

	case "mkdir.filename":

		return reflect.String, nil

	case "mkdir.mode":

		return reflect.Int, nil

	}

	return reflect.Invalid, &ErrFieldNotFound{Field: field}
}

var testConstants = map[string]interface{}{
	// boolean
	"true":  &BoolEvaluator{Value: true},
	"false": &BoolEvaluator{Value: false},

	// open flags
	"O_RDONLY": &IntEvaluator{Value: syscall.O_RDONLY},
	"O_WRONLY": &IntEvaluator{Value: syscall.O_WRONLY},
	"O_RDWR":   &IntEvaluator{Value: syscall.O_RDWR},
	"O_APPEND": &IntEvaluator{Value: syscall.O_APPEND},
	"O_CREAT":  &IntEvaluator{Value: syscall.O_CREAT},
	"O_EXCL":   &IntEvaluator{Value: syscall.O_EXCL},
	"O_SYNC":   &IntEvaluator{Value: syscall.O_SYNC},
	"O_TRUNC":  &IntEvaluator{Value: syscall.O_TRUNC},

	// retval
	"EPERM":        &IntEvaluator{Value: int(syscall.EPERM)},
	"EACCES":       &IntEvaluator{Value: int(syscall.EACCES)},
	"EPFNOSUPPORT": &IntEvaluator{Value: int(syscall.EPFNOSUPPORT)},
	"EPIPE":        &IntEvaluator{Value: int(syscall.EPIPE)},

	// string constants
	"my_constant_1": &StringEvaluator{Value: "my_constant_1"},
	"my_constant_2": &StringEvaluator{Value: "my_constant_2"},
}
