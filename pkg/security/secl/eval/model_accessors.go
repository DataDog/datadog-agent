// Code generated - DO NOT EDIT.

package eval

import (
	"github.com/pkg/errors"
)

var (
	ErrFieldNotFound = errors.New("field not found")
)

func GetAccessor(key string) (interface{}, []string, error) {
	switch key {

	case "container.id":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Container.ID },
			DebugEval: func(ctx *Context) string { return ctx.Event.Container.ID },
			Field:     key,
		}, []string{"container"}, nil

	case "open.filename":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Open.Filename },
			DebugEval: func(ctx *Context) string { return ctx.Event.Open.Filename },
			Field:     key,
		}, []string{"fs"}, nil

	case "open.flags":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Open.Flags) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Open.Flags) },
			Field:     key,
		}, []string{}, nil

	case "open.mode":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Open.Mode) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Open.Mode) },
			Field:     key,
		}, []string{}, nil

	case "process.gid":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.GID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.GID) },
			Field:     key,
		}, []string{}, nil

	case "process.name":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Process.Name },
			DebugEval: func(ctx *Context) string { return ctx.Event.Process.Name },
			Field:     key,
		}, []string{"process"}, nil

	case "process.pid":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.PID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.PID) },
			Field:     key,
		}, []string{}, nil

	case "process.uid":

		return &IntEvaluator{
			Eval:      func(ctx *Context) int { return int(ctx.Event.Process.UID) },
			DebugEval: func(ctx *Context) int { return int(ctx.Event.Process.UID) },
			Field:     key,
		}, []string{}, nil

	case "rename.newname":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Rename.NewName },
			DebugEval: func(ctx *Context) string { return ctx.Event.Rename.NewName },
			Field:     key,
		}, []string{"fs"}, nil

	case "rename.oldname":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Rename.OldName },
			DebugEval: func(ctx *Context) string { return ctx.Event.Rename.OldName },
			Field:     key,
		}, []string{"fs"}, nil

	case "syscall":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Syscall },
			DebugEval: func(ctx *Context) string { return ctx.Event.Syscall },
			Field:     key,
		}, []string{}, nil

	case "unlink.filename":

		return &StringEvaluator{
			Eval:      func(ctx *Context) string { return ctx.Event.Unlink.Filename },
			DebugEval: func(ctx *Context) string { return ctx.Event.Unlink.Filename },
			Field:     key,
		}, []string{"fs"}, nil

	}

	return nil, nil, errors.Wrap(ErrFieldNotFound, key)
}
