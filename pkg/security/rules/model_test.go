// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"reflect"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type testProcess struct {
	name   string
	uid    int
	gid    int
	isRoot bool
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
}

type testModel struct {
}

func (e *testEvent) GetType() string {
	return e.kind
}

func (e *testEvent) GetTags() []string {
	return nil
}

func (e *testEvent) GetPointer() unsafe.Pointer {
	return unsafe.Pointer(e)
}

func (m *testModel) NewEvent() eval.Event {
	return &testEvent{}
}

func (m *testModel) ValidateField(key string, value eval.FieldValue) error {
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

func (m *testModel) GetIterator(field eval.Field) (eval.Iterator, error) {
	return nil, &eval.ErrIteratorNotSupported{Field: field}
}

func (m *testModel) GetEvaluator(key string, regID eval.RegisterID) (eval.Evaluator, error) {
	switch key {

	case "process.name":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return (*testEvent)(ctx.Object).process.name },
			Field:   key,
		}, nil

	case "process.uid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return (*testEvent)(ctx.Object).process.uid },
			Field:   key,
		}, nil

	case "process.gid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return (*testEvent)(ctx.Object).process.gid },
			Field:   key,
		}, nil

	case "process.is_root":

		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return (*testEvent)(ctx.Object).process.isRoot },
			Field:   key,
		}, nil

	case "open.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return (*testEvent)(ctx.Object).open.filename },
			Field:   key,
		}, nil

	case "open.flags":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return (*testEvent)(ctx.Object).open.flags },
			Field:   key,
		}, nil

	case "open.mode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return (*testEvent)(ctx.Object).open.mode },
			Field:   key,
		}, nil

	case "mkdir.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return (*testEvent)(ctx.Object).mkdir.filename },
			Field:   key,
		}, nil

	case "mkdir.mode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return (*testEvent)(ctx.Object).mkdir.mode },
			Field:   key,
		}, nil

	}

	return nil, &eval.ErrFieldNotFound{Field: key}
}

func (e *testEvent) GetFieldValue(key string) (interface{}, error) {
	switch key {

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

	return nil, &eval.ErrFieldNotFound{Field: key}
}

func (e *testEvent) GetFieldEventType(key string) (string, error) {
	switch key {

	case "process.name":

		return "*", nil

	case "process.uid":

		return "*", nil

	case "process.gid":

		return "*", nil

	case "process.is_root":

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

	return "", &eval.ErrFieldNotFound{Field: key}
}

func (e *testEvent) SetFieldValue(key string, value interface{}) error {
	switch key {

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

	return &eval.ErrFieldNotFound{Field: key}
}

func (e *testEvent) GetFieldType(key string) (reflect.Kind, error) {
	switch key {

	case "process.name":

		return reflect.String, nil

	case "process.uid":

		return reflect.Int, nil

	case "process.gid":

		return reflect.Int, nil

	case "process.is_root":

		return reflect.Bool, nil

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

	return reflect.Invalid, &eval.ErrFieldNotFound{Field: key}
}

var testConstants = map[string]interface{}{
	// boolean
	"true":  &eval.BoolEvaluator{Value: true},
	"false": &eval.BoolEvaluator{Value: false},

	// open flags
	"O_RDONLY": &eval.IntEvaluator{Value: syscall.O_RDONLY},
	"O_WRONLY": &eval.IntEvaluator{Value: syscall.O_WRONLY},
	"O_RDWR":   &eval.IntEvaluator{Value: syscall.O_RDWR},
	"O_APPEND": &eval.IntEvaluator{Value: syscall.O_APPEND},
	"O_CREAT":  &eval.IntEvaluator{Value: syscall.O_CREAT},
	"O_EXCL":   &eval.IntEvaluator{Value: syscall.O_EXCL},
	"O_SYNC":   &eval.IntEvaluator{Value: syscall.O_SYNC},
	"O_TRUNC":  &eval.IntEvaluator{Value: syscall.O_TRUNC},
}

var testSupportedDiscarders = map[eval.Field]bool{
	"open.filename":  true,
	"mkdir.filename": true,
	"process.uid":    true,
}
