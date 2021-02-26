// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"reflect"
	"syscall"
	"unsafe"

	"container/list"

	"github.com/pkg/errors"
)

type testItem struct {
	key   int
	value int
}

type testProcess struct {
	name   string
	uid    int
	gid    int
	isRoot bool
	list   *list.List
	array  []*testItem
}

type testItemListIterator struct {
	prev *list.Element
}

func (t *testItemListIterator) Front(ctx *Context) unsafe.Pointer {
	if front := (*testEvent)(ctx.Object).process.list.Front(); front != nil {
		t.prev = front
		return unsafe.Pointer(front)
	}
	return nil
}

func (t *testItemListIterator) Next() unsafe.Pointer {
	if next := t.prev.Next(); next != nil {
		t.prev = next
		return unsafe.Pointer(next)
	}
	return nil
}

type testItemArrayIterator struct {
	ctx   *Context
	index int
}

func (t *testItemArrayIterator) Front(ctx *Context) unsafe.Pointer {
	t.ctx = ctx

	array := (*testEvent)(ctx.Object).process.array
	if t.index < len(array) {
		t.index++
		return unsafe.Pointer(array[0])
	}
	return nil
}

func (t *testItemArrayIterator) Next() unsafe.Pointer {
	array := (*testEvent)(t.ctx.Object).process.array
	if t.index < len(array) {
		value := array[t.index]
		t.index++

		return unsafe.Pointer(value)
	}

	return nil
}

type testOpen struct {
	filename string
	mode     int
	flags    int
}

type testMkdir struct {
	filename string
	mode     int
}

type testEvent struct {
	id   string
	kind string

	process testProcess
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

func (e *testEvent) GetPointer() unsafe.Pointer {
	return unsafe.Pointer(e)
}

func (m *testModel) NewEvent() Event {
	return &testEvent{}
}

func (m *testModel) ValidateField(key string, value FieldValue) error {
	switch key {

	case "process.uid":

		uid, ok := value.Value.(int)
		if !ok {
			return errors.New("invalid type for process.ui")
		}

		if uid < 0 {
			return errors.New("process.uid cannot be negative")
		}

	}

	return nil
}

func (m *testModel) TranslateLegacyField(field Field) Field {
	switch field {
	case "process.legacy_name":
		return "process.name"
	default:
		return field
	}
}

func (m *testModel) GetIterator(field Field) (Iterator, error) {
	switch field {
	case "process.list":
		return &testItemListIterator{}, nil
	case "process.array":
		return &testItemArrayIterator{}, nil
	}

	return nil, &ErrIteratorNotSupported{Field: field}
}

func (m *testModel) GetEvaluator(field Field, regID RegisterID) (Evaluator, error) {
	switch field {

	case "process.name":

		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string { return (*testEvent)(ctx.Object).process.name },
			Field:   field,
		}, nil

	case "process.uid":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				// to test optimisation
				(*testEvent)(ctx.Object).uidEvaluated = true

				return (*testEvent)(ctx.Object).process.uid
			},
			Field: field,
		}, nil

	case "process.gid":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				// to test optimisation
				(*testEvent)(ctx.Object).gidEvaluated = true

				return (*testEvent)(ctx.Object).process.gid
			},
			Field: field,
		}, nil

	case "process.is_root":

		return &BoolEvaluator{
			EvalFnc: func(ctx *Context) bool { return (*testEvent)(ctx.Object).process.isRoot },
			Field:   field,
		}, nil

	case "process.list.key":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				// to test optimisation
				(*testEvent)(ctx.Object).listEvaluated = true

				reg := ctx.Registers[regID]
				if element := (*list.Element)(reg.Value); element != nil {
					return element.Value.(*testItem).key
				}

				return 0
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.list.value":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				// to test optimisation
				(*testEvent)(ctx.Object).listEvaluated = true

				reg := ctx.Registers[regID]
				if element := (*list.Element)(reg.Value); element != nil {
					return element.Value.(*testItem).value
				}

				return 0
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.array.key":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				reg := ctx.Registers[regID]
				if item := (*testItem)(reg.Value); item != nil {
					return item.key
				}

				return 0
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "process.array.value":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				reg := ctx.Registers[regID]
				if item := (*testItem)(reg.Value); item != nil {
					return item.value
				}

				return 0
			},
			Field:  field,
			Weight: IteratorWeight,
		}, nil

	case "open.filename":

		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string { return (*testEvent)(ctx.Object).open.filename },
			Field:   field,
		}, nil

	case "open.flags":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int { return (*testEvent)(ctx.Object).open.flags },
			Field:   field,
		}, nil

	case "open.mode":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int { return (*testEvent)(ctx.Object).open.mode },
			Field:   field,
		}, nil

	case "mkdir.filename":

		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string { return (*testEvent)(ctx.Object).mkdir.filename },
			Field:   field,
		}, nil

	case "mkdir.mode":

		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int { return (*testEvent)(ctx.Object).mkdir.mode },
			Field:   field,
		}, nil
	}

	return nil, &ErrFieldNotFound{Field: field}
}

func (e *testEvent) GetFieldValue(field Field) (interface{}, error) {
	switch field {

	case "process.name":

		return e.process.name, nil

	case "process.uid":

		return e.process.uid, nil

	case "process.gid":

		return e.process.gid, nil

	case "process.is_root":

		return e.process.isRoot, nil

	case "open.filename":

		return e.open.filename, nil

	case "open.flags":

		return e.open.flags, nil

	case "open.mode":

		return e.open.mode, nil

	case "mkdir.filename":

		return e.mkdir.filename, nil

	case "mkdir.mode":

		return e.mkdir.mode, nil

	}

	return nil, &ErrFieldNotFound{Field: field}
}

func (e *testEvent) GetFieldEventType(field Field) (string, error) {
	switch field {

	case "process.name":

		return "*", nil

	case "process.uid":

		return "*", nil

	case "process.gid":

		return "*", nil

	case "process.is_root":

		return "*", nil

	case "process.list.key":

		return "*", nil

	case "process.list.value":

		return "*", nil

	case "process.array.key":

		return "*", nil

	case "process.array.value":

		return "*", nil

	case "open.filename":

		return "open", nil

	case "open.flags":

		return "open", nil

	case "open.mode":

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

	case "process.name":

		e.process.name = value.(string)
		return nil

	case "process.uid":

		e.process.uid = value.(int)
		return nil

	case "process.gid":

		e.process.gid = value.(int)
		return nil

	case "process.is_root":

		e.process.isRoot = value.(bool)
		return nil

	case "open.filename":

		e.open.filename = value.(string)
		return nil

	case "open.flags":

		e.open.flags = value.(int)
		return nil

	case "open.mode":

		e.open.mode = value.(int)
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

	case "process.name":

		return reflect.String, nil

	case "process.uid":

		return reflect.Int, nil

	case "process.gid":

		return reflect.Int, nil

	case "process.is_root":

		return reflect.Bool, nil

	case "process.list.key":
		return reflect.Int, nil

	case "process.list.value":
		return reflect.Int, nil

	case "process.array.key":
		return reflect.Int, nil

	case "process.array.value":
		return reflect.Int, nil

	case "open.filename":

		return reflect.String, nil

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
}
