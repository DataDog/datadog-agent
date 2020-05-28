package eval

import (
	"syscall"

	"github.com/pkg/errors"
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
	event *testEvent
}

func (e *testEvent) GetID() string {
	return e.id
}

func (e *testEvent) GetType() string {
	return e.kind
}

func (m *testModel) SetEvent(event interface{}) {
	m.event = event.(*testEvent)
}

func (m *testModel) GetEvent() Event {
	return m.event
}

func (m *testModel) GetEvaluator(key string) (interface{}, error) {
	switch key {

	case "process.name":

		return &StringEvaluator{
			EvalFnc:      func(ctx *Context) string { return m.event.process.name },
			DebugEvalFnc: func(ctx *Context) string { return m.event.process.name },
			Field:        key,
		}, nil

	case "process.uid":

		return &IntEvaluator{
			EvalFnc:      func(ctx *Context) int { return m.event.process.uid },
			DebugEvalFnc: func(ctx *Context) int { return m.event.process.uid },
			Field:        key,
		}, nil

	case "process.gid":

		return &IntEvaluator{
			EvalFnc:      func(ctx *Context) int { return m.event.process.gid },
			DebugEvalFnc: func(ctx *Context) int { return m.event.process.gid },
			Field:        key,
		}, nil

	case "process.is_root":

		return &BoolEvaluator{
			EvalFnc:      func(ctx *Context) bool { return m.event.process.isRoot },
			DebugEvalFnc: func(ctx *Context) bool { return m.event.process.isRoot },
			Field:        key,
		}, nil

	case "open.filename":

		return &StringEvaluator{
			EvalFnc:      func(ctx *Context) string { return m.event.open.filename },
			DebugEvalFnc: func(ctx *Context) string { return m.event.open.filename },
			Field:        key,
		}, nil

	case "open.flags":

		return &IntEvaluator{
			EvalFnc:      func(ctx *Context) int { return m.event.open.flags },
			DebugEvalFnc: func(ctx *Context) int { return m.event.open.flags },
			Field:        key,
		}, nil

	case "open.mode":

		return &IntEvaluator{
			EvalFnc:      func(ctx *Context) int { return m.event.open.mode },
			DebugEvalFnc: func(ctx *Context) int { return m.event.open.mode },
			Field:        key,
		}, nil

	case "mkdir.filename":

		return &StringEvaluator{
			EvalFnc:      func(ctx *Context) string { return m.event.mkdir.filename },
			DebugEvalFnc: func(ctx *Context) string { return m.event.mkdir.filename },
			Field:        key,
		}, nil

	case "mkdir.mode":

		return &IntEvaluator{
			EvalFnc:      func(ctx *Context) int { return m.event.mkdir.mode },
			DebugEvalFnc: func(ctx *Context) int { return m.event.mkdir.mode },
			Field:        key,
		}, nil

	}

	return nil, errors.Wrap(ErrEvaluatorNotFound, key)
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

	return nil, errors.Wrap(ErrEvaluatorNotFound, key)
}

func (e *testEvent) GetFieldTags(key string) ([]string, error) {
	switch key {

	case "process.name":

		return []string{"process"}, nil

	case "process.uid":

		return []string{"process"}, nil

	case "process.gid":

		return []string{"process"}, nil

	case "process.is_root":

		return []string{"process"}, nil

	case "open.filename":

		return []string{"fs"}, nil

	case "open.flags":

		return []string{"fs"}, nil

	case "open.mode":

		return []string{"fs"}, nil

	case "mkdir.filename":

		return []string{"fs"}, nil

	case "mkdir.flags":

		return []string{"fs"}, nil

	}

	return nil, errors.Wrap(ErrTagsNotFound, key)
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

	return "", errors.Wrap(ErrEventTypeNotFound, key)
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

	return errors.Wrap(ErrSetEventValueNotFound, key)
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
