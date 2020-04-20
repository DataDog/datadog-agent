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

	case "container.id":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Container.ID },
			DebugEval: func(ctx *Context) string { return ctx.Event.Container.ID },
		}, nil

	case "open.flags":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Open.Flags) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Open.Flags) },
		}, nil

	case "open.mode":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Open.Mode) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Open.Mode) },
		}, nil

	case "open.pathname":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Open.Pathname },
			DebugEval: func(ctx *Context) string { return ctx.Event.Open.Pathname },
		}, nil

	case "process.gid":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.GID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.GID) },
		}, nil

	case "process.name":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Process.Name },
			DebugEval: func(ctx *Context) string { return ctx.Event.Process.Name },
		}, nil

	case "process.pid":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.PID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.PID) },
		}, nil

	case "process.uid":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.UID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.UID) },
		}, nil

	case "rename.newname":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Rename.NewName },
			DebugEval: func(ctx *Context) string { return ctx.Event.Rename.NewName },
		}, nil

	case "rename.oldname":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Rename.OldName },
			DebugEval: func(ctx *Context) string { return ctx.Event.Rename.OldName },
		}, nil

	case "syscall":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Syscall },
			DebugEval: func(ctx *Context) string { return ctx.Event.Syscall },
		}, nil

	case "unlink.pathname":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Unlink.Pathname },
			DebugEval: func(ctx *Context) string { return ctx.Event.Unlink.Pathname },
		}, nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}
