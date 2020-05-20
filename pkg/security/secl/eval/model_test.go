package eval

import (
	"syscall"

	"github.com/pkg/errors"
)

type testProcess struct {
	name   string
	uid    int
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

func (m *testModel) GetEvaluator(key string) (interface{}, error) {
	switch key {

	case "process.name":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return m.event.process.name },
			DebugEval: func(ctx *Context) string { return m.event.process.name },
			Field:     key,
		}, nil

	case "process.uid":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return m.event.process.uid },
			DebugEval: func(ctx *Context) int { return m.event.process.uid },
			Field:     key,
		}, nil

	case "process.is_root":

		return &BoolEvaluator{
			Eval:      func(ctx *Context) bool { return m.event.process.isRoot },
			DebugEval: func(ctx *Context) bool { return m.event.process.isRoot },
			Field:     key,
		}, nil

	case "open.filename":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return m.event.open.filename },
			DebugEval: func(ctx *Context) string { return m.event.open.filename },
			Field:     key,
		}, nil

	case "open.flags":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return m.event.open.flags },
			DebugEval: func(ctx *Context) int { return m.event.open.flags },
			Field:     key,
		}, nil

	case "open.mode":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return m.event.open.mode },
			DebugEval: func(ctx *Context) int { return m.event.open.mode },
			Field:     key,
		}, nil

	case "mkdir.filename":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return m.event.mkdir.filename },
			DebugEval: func(ctx *Context) string { return m.event.mkdir.filename },
			Field:     key,
		}, nil

	case "mkdir.mode":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return m.event.mkdir.mode },
			DebugEval: func(ctx *Context) int { return m.event.mkdir.mode },
			Field:     key,
		}, nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}

func (m *testModel) GetTags(key string) ([]string, error) {
	switch key {

	case "process.name":

		return []string{"process"}, nil

	case "process.uid":

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

	return nil, errors.Wrap(ErrFieldNotFound, key)
}

func (m *testModel) GetEventType(key string) (string, error) {
	switch key {

	case "process.name":

		return "", nil

	case "process.uid":

		return "", nil

	case "process.is_root":

		return "", nil

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

	return "", errors.Wrap(ErrFieldNotFound, key)
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
