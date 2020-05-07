package eval

import (
	"github.com/pkg/errors"
)

type testProcess struct {
	name   string
	uid    int
	isRoot bool
}

type testOpen struct {
	filename string
	flags    int
}

type testEvent struct {
	id   string
	kind string

	process testProcess
	open    testOpen
}

type testModel struct {
	data *testEvent
}

func (e *testEvent) GetID() string {
	return e.id
}

func (e *testEvent) GetType() string {
	return e.kind
}

func (m *testModel) SetData(data interface{}) {
	m.data = data.(*testEvent)
}

func (m *testModel) GetEvaluator(key string) (interface{}, error) {
	switch key {

	case "process.name":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return m.data.process.name },
			DebugEval: func(ctx *Context) string { return m.data.process.name },
			Field:     key,
		}, nil

	case "process.uid":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return m.data.process.uid },
			DebugEval: func(ctx *Context) int { return m.data.process.uid },
			Field:     key,
		}, nil

	case "process.is_root":

		return &BoolEvaluator{
			Eval:      func(ctx *Context) bool { return m.data.process.isRoot },
			DebugEval: func(ctx *Context) bool { return m.data.process.isRoot },
			Field:     key,
		}, nil

	case "open.filename":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return m.data.open.filename },
			DebugEval: func(ctx *Context) string { return m.data.open.filename },
			Field:     key,
		}, nil

	case "open.flags":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return m.data.open.flags },
			DebugEval: func(ctx *Context) int { return m.data.open.flags },
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

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}
