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
			Eval:       func(ctx *Context) string { return ctx.Event.Container.ID },
			DebugEval:  func(ctx *Context) string { return ctx.Event.Container.ID },
			ModelField: "Container.ID",
		}, []string{"container"}, nil

	case "open.filename":

		return &StringEvaluator{
			Eval:       func(ctx *Context) string { return ctx.Event.Open.Filename },
			DebugEval:  func(ctx *Context) string { return ctx.Event.Open.Filename },
			ModelField: "Open.Filename",
		}, []string{"fs"}, nil

	case "open.flags":

		return &IntEvaluator{
			Eval:       func(ctx *Context) int { return int(ctx.Event.Open.Flags) },
			DebugEval:  func(ctx *Context) int { return int(ctx.Event.Open.Flags) },
			ModelField: "Open.Flags",
		}, []string{}, nil

	case "open.mode":

		return &IntEvaluator{
			Eval:       func(ctx *Context) int { return int(ctx.Event.Open.Mode) },
			DebugEval:  func(ctx *Context) int { return int(ctx.Event.Open.Mode) },
			ModelField: "Open.Mode",
		}, []string{}, nil

	case "process.gid":

		return &IntEvaluator{
			Eval:       func(ctx *Context) int { return int(ctx.Event.Process.GID) },
			DebugEval:  func(ctx *Context) int { return int(ctx.Event.Process.GID) },
			ModelField: "Process.GID",
		}, []string{}, nil

	case "process.name":

		return &StringEvaluator{
			Eval:       func(ctx *Context) string { return ctx.Event.Process.Name },
			DebugEval:  func(ctx *Context) string { return ctx.Event.Process.Name },
			ModelField: "Process.Name",
		}, []string{"process"}, nil

	case "process.pid":

		return &IntEvaluator{
			Eval:       func(ctx *Context) int { return int(ctx.Event.Process.PID) },
			DebugEval:  func(ctx *Context) int { return int(ctx.Event.Process.PID) },
			ModelField: "Process.PID",
		}, []string{}, nil

	case "process.uid":

		return &IntEvaluator{
			Eval:       func(ctx *Context) int { return int(ctx.Event.Process.UID) },
			DebugEval:  func(ctx *Context) int { return int(ctx.Event.Process.UID) },
			ModelField: "Process.UID",
		}, []string{}, nil

	case "rename.newname":

		return &StringEvaluator{
			Eval:       func(ctx *Context) string { return ctx.Event.Rename.NewName },
			DebugEval:  func(ctx *Context) string { return ctx.Event.Rename.NewName },
			ModelField: "Rename.NewName",
		}, []string{"fs"}, nil

	case "rename.oldname":

		return &StringEvaluator{
			Eval:       func(ctx *Context) string { return ctx.Event.Rename.OldName },
			DebugEval:  func(ctx *Context) string { return ctx.Event.Rename.OldName },
			ModelField: "Rename.OldName",
		}, []string{"fs"}, nil

	case "syscall":

		return &StringEvaluator{
			Eval:       func(ctx *Context) string { return ctx.Event.Syscall },
			DebugEval:  func(ctx *Context) string { return ctx.Event.Syscall },
			ModelField: "Syscall",
		}, []string{}, nil

	case "unlink.filename":

		return &StringEvaluator{
			Eval:       func(ctx *Context) string { return ctx.Event.Unlink.Filename },
			DebugEval:  func(ctx *Context) string { return ctx.Event.Unlink.Filename },
			ModelField: "Unlink.Filename",
		}, []string{"fs"}, nil

	}

	return nil, nil, errors.Wrap(ErrFieldNotFound, key)
}
