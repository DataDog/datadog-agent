// Code generated - DO NOT EDIT.

package eval

import (
	"github.com/pkg/errors"
)

var (
	ErrFieldNotFound = errors.New("field not found")
)

func GetAccessor(key string) (interface{}, error) {
	switch key {

	case "Container.ID":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Container.ID },
			DebugEval: func(ctx *Context) string { return ctx.Event.Container.ID },
		}, nil

	case "Open.Flags":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Open.Flags) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Open.Flags) },
		}, nil

	case "Open.Mode":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Open.Mode) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Open.Mode) },
		}, nil

	case "Open.Pathname":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Open.Pathname },
			DebugEval: func(ctx *Context) string { return ctx.Event.Open.Pathname },
		}, nil

	case "Process.GID":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.GID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.GID) },
		}, nil

	case "Process.Name":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Process.Name },
			DebugEval: func(ctx *Context) string { return ctx.Event.Process.Name },
		}, nil

	case "Process.PID":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.PID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.PID) },
		}, nil

	case "Process.UID":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.UID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.UID) },
		}, nil

	case "Rename.NewName":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Rename.NewName },
			DebugEval: func(ctx *Context) string { return ctx.Event.Rename.NewName },
		}, nil

	case "Rename.OldName":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Rename.OldName },
			DebugEval: func(ctx *Context) string { return ctx.Event.Rename.OldName },
		}, nil

	case "Syscall":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Syscall },
			DebugEval: func(ctx *Context) string { return ctx.Event.Syscall },
		}, nil

	case "Unlink.Pathname":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Unlink.Pathname },
			DebugEval: func(ctx *Context) string { return ctx.Event.Unlink.Pathname },
		}, nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}
