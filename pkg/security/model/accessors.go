// +build linux

// Code generated - DO NOT EDIT.

package model

import (
	"reflect"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// suppress unused package warning
var (
	_ *unsafe.Pointer
)

func (m *Model) GetIterator(field eval.Field) (eval.Iterator, error) {
	switch field {

	case "process.ancestors":
		return &ProcessAncestorsIterator{}, nil

	}

	return nil, &eval.ErrIteratorNotSupported{Field: field}
}

func (m *Model) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {

	case "chmod.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chmod.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chmod.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chmod.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Container.ID

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Cookie)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.GID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "exec.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "exec.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.PPid)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.TTYName

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.UID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.source.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Source.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.source.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Source.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.source.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Source.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.source.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Source.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.source.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Source.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.target.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Target.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.target.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Target.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.target.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Target.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.target.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Target.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.target.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Target.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Mkdir.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Mkdir.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Mkdir.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Open.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "open.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Open.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "open.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Open.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "open.flags":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.Flags)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.ancestors.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.ContainerPath

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.Cookie)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.PathnameStr

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.GID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.Group

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ContainerContext.ID

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.FileFields.Inode)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.BasenameStr

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.FileFields.OverlayNumLower)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.Pid)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.PPid)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.Tid)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.TTYName

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.UID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.User

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.Cookie)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.Pid)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.PPid)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.Tid)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.TTYName

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "removexattr.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.Name

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.namespace":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.Namespace

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "removexattr.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.new.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.New.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.new.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.New.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.new.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.New.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.new.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.New.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.new.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.New.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.old.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.Old.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.old.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.Old.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.old.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.Old.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.old.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.Old.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.old.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.Old.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rmdir.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rmdir.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rmdir.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.Name

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.namespace":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.Namespace

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Unlink.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Unlink.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Unlink.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.flags":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.Flags)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.basename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Utimes.FileEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Utimes.FileEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.filename":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Utimes.FileEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.FileEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.FileEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFields() []eval.Field {
	return []eval.Field{

		"chmod.basename",
		"chmod.container_path",
		"chmod.filename",
		"chmod.inode",
		"chmod.mode",
		"chmod.overlay_numlower",
		"chmod.retval",
		"chown.basename",
		"chown.container_path",
		"chown.filename",
		"chown.gid",
		"chown.inode",
		"chown.overlay_numlower",
		"chown.retval",
		"chown.uid",
		"container.id",
		"exec.container_path",
		"exec.cookie",
		"exec.filename",
		"exec.gid",
		"exec.group",
		"exec.inode",
		"exec.name",
		"exec.overlay_numlower",
		"exec.ppid",
		"exec.tty_name",
		"exec.uid",
		"exec.user",
		"link.retval",
		"link.source.basename",
		"link.source.container_path",
		"link.source.filename",
		"link.source.inode",
		"link.source.overlay_numlower",
		"link.target.basename",
		"link.target.container_path",
		"link.target.filename",
		"link.target.inode",
		"link.target.overlay_numlower",
		"mkdir.basename",
		"mkdir.container_path",
		"mkdir.filename",
		"mkdir.inode",
		"mkdir.mode",
		"mkdir.overlay_numlower",
		"mkdir.retval",
		"open.basename",
		"open.container_path",
		"open.filename",
		"open.flags",
		"open.inode",
		"open.mode",
		"open.overlay_numlower",
		"open.retval",
		"process.ancestors.container_path",
		"process.ancestors.cookie",
		"process.ancestors.filename",
		"process.ancestors.gid",
		"process.ancestors.group",
		"process.ancestors.id",
		"process.ancestors.inode",
		"process.ancestors.name",
		"process.ancestors.overlay_numlower",
		"process.ancestors.pid",
		"process.ancestors.ppid",
		"process.ancestors.tid",
		"process.ancestors.tty_name",
		"process.ancestors.uid",
		"process.ancestors.user",
		"process.container_path",
		"process.cookie",
		"process.filename",
		"process.gid",
		"process.group",
		"process.inode",
		"process.name",
		"process.overlay_numlower",
		"process.pid",
		"process.ppid",
		"process.tid",
		"process.tty_name",
		"process.uid",
		"process.user",
		"removexattr.basename",
		"removexattr.container_path",
		"removexattr.filename",
		"removexattr.inode",
		"removexattr.name",
		"removexattr.namespace",
		"removexattr.overlay_numlower",
		"removexattr.retval",
		"rename.new.basename",
		"rename.new.container_path",
		"rename.new.filename",
		"rename.new.inode",
		"rename.new.overlay_numlower",
		"rename.old.basename",
		"rename.old.container_path",
		"rename.old.filename",
		"rename.old.inode",
		"rename.old.overlay_numlower",
		"rename.retval",
		"rmdir.basename",
		"rmdir.container_path",
		"rmdir.filename",
		"rmdir.inode",
		"rmdir.overlay_numlower",
		"rmdir.retval",
		"setxattr.basename",
		"setxattr.container_path",
		"setxattr.filename",
		"setxattr.inode",
		"setxattr.name",
		"setxattr.namespace",
		"setxattr.overlay_numlower",
		"setxattr.retval",
		"unlink.basename",
		"unlink.container_path",
		"unlink.filename",
		"unlink.flags",
		"unlink.inode",
		"unlink.overlay_numlower",
		"unlink.retval",
		"utimes.basename",
		"utimes.container_path",
		"utimes.filename",
		"utimes.inode",
		"utimes.overlay_numlower",
		"utimes.retval",
	}
}

func (e *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {

	case "chmod.basename":

		return e.Chmod.FileEvent.BasenameStr, nil

	case "chmod.container_path":

		return e.Chmod.FileEvent.ContainerPath, nil

	case "chmod.filename":

		return e.Chmod.FileEvent.PathnameStr, nil

	case "chmod.inode":

		return int(e.Chmod.FileEvent.FileFields.Inode), nil

	case "chmod.mode":

		return int(e.Chmod.Mode), nil

	case "chmod.overlay_numlower":

		return int(e.Chmod.FileEvent.FileFields.OverlayNumLower), nil

	case "chmod.retval":

		return int(e.Chmod.SyscallEvent.Retval), nil

	case "chown.basename":

		return e.Chown.FileEvent.BasenameStr, nil

	case "chown.container_path":

		return e.Chown.FileEvent.ContainerPath, nil

	case "chown.filename":

		return e.Chown.FileEvent.PathnameStr, nil

	case "chown.gid":

		return int(e.Chown.GID), nil

	case "chown.inode":

		return int(e.Chown.FileEvent.FileFields.Inode), nil

	case "chown.overlay_numlower":

		return int(e.Chown.FileEvent.FileFields.OverlayNumLower), nil

	case "chown.retval":

		return int(e.Chown.SyscallEvent.Retval), nil

	case "chown.uid":

		return int(e.Chown.UID), nil

	case "container.id":

		return e.Container.ID, nil

	case "exec.container_path":

		return e.Exec.ContainerPath, nil

	case "exec.cookie":

		return int(e.Exec.Cookie), nil

	case "exec.filename":

		return e.Exec.PathnameStr, nil

	case "exec.gid":

		return int(e.Exec.GID), nil

	case "exec.group":

		return e.Exec.Group, nil

	case "exec.inode":

		return int(e.Exec.FileFields.Inode), nil

	case "exec.name":

		return e.Exec.BasenameStr, nil

	case "exec.overlay_numlower":

		return int(e.Exec.FileFields.OverlayNumLower), nil

	case "exec.ppid":

		return int(e.Exec.PPid), nil

	case "exec.tty_name":

		return e.Exec.TTYName, nil

	case "exec.uid":

		return int(e.Exec.UID), nil

	case "exec.user":

		return e.Exec.User, nil

	case "link.retval":

		return int(e.Link.SyscallEvent.Retval), nil

	case "link.source.basename":

		return e.Link.Source.BasenameStr, nil

	case "link.source.container_path":

		return e.Link.Source.ContainerPath, nil

	case "link.source.filename":

		return e.Link.Source.PathnameStr, nil

	case "link.source.inode":

		return int(e.Link.Source.FileFields.Inode), nil

	case "link.source.overlay_numlower":

		return int(e.Link.Source.FileFields.OverlayNumLower), nil

	case "link.target.basename":

		return e.Link.Target.BasenameStr, nil

	case "link.target.container_path":

		return e.Link.Target.ContainerPath, nil

	case "link.target.filename":

		return e.Link.Target.PathnameStr, nil

	case "link.target.inode":

		return int(e.Link.Target.FileFields.Inode), nil

	case "link.target.overlay_numlower":

		return int(e.Link.Target.FileFields.OverlayNumLower), nil

	case "mkdir.basename":

		return e.Mkdir.FileEvent.BasenameStr, nil

	case "mkdir.container_path":

		return e.Mkdir.FileEvent.ContainerPath, nil

	case "mkdir.filename":

		return e.Mkdir.FileEvent.PathnameStr, nil

	case "mkdir.inode":

		return int(e.Mkdir.FileEvent.FileFields.Inode), nil

	case "mkdir.mode":

		return int(e.Mkdir.Mode), nil

	case "mkdir.overlay_numlower":

		return int(e.Mkdir.FileEvent.FileFields.OverlayNumLower), nil

	case "mkdir.retval":

		return int(e.Mkdir.SyscallEvent.Retval), nil

	case "open.basename":

		return e.Open.FileEvent.BasenameStr, nil

	case "open.container_path":

		return e.Open.FileEvent.ContainerPath, nil

	case "open.filename":

		return e.Open.FileEvent.PathnameStr, nil

	case "open.flags":

		return int(e.Open.Flags), nil

	case "open.inode":

		return int(e.Open.FileEvent.FileFields.Inode), nil

	case "open.mode":

		return int(e.Open.Mode), nil

	case "open.overlay_numlower":

		return int(e.Open.FileEvent.FileFields.OverlayNumLower), nil

	case "open.retval":

		return int(e.Open.SyscallEvent.Retval), nil

	case "process.ancestors.container_path":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.ContainerPath

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.cookie":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.Cookie)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.filename":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.PathnameStr

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.gid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.GID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.group":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.Group

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.id":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ContainerContext.ID

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.inode":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.FileFields.Inode)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.name":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.BasenameStr

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.overlay_numlower":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.FileFields.OverlayNumLower)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.pid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Pid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.ppid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.PPid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.tid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Tid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.tty_name":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.TTYName

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.uid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.UID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.user":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.User

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.container_path":

		return e.Process.ExecEvent.ContainerPath, nil

	case "process.cookie":

		return int(e.Process.ExecEvent.Cookie), nil

	case "process.filename":

		return e.Process.ExecEvent.PathnameStr, nil

	case "process.gid":

		return int(e.Process.GID), nil

	case "process.group":

		return e.Process.ExecEvent.Group, nil

	case "process.inode":

		return int(e.Process.ExecEvent.FileFields.Inode), nil

	case "process.name":

		return e.Process.ExecEvent.BasenameStr, nil

	case "process.overlay_numlower":

		return int(e.Process.ExecEvent.FileFields.OverlayNumLower), nil

	case "process.pid":

		return int(e.Process.Pid), nil

	case "process.ppid":

		return int(e.Process.ExecEvent.PPid), nil

	case "process.tid":

		return int(e.Process.Tid), nil

	case "process.tty_name":

		return e.Process.ExecEvent.TTYName, nil

	case "process.uid":

		return int(e.Process.UID), nil

	case "process.user":

		return e.Process.ExecEvent.User, nil

	case "removexattr.basename":

		return e.RemoveXAttr.FileEvent.BasenameStr, nil

	case "removexattr.container_path":

		return e.RemoveXAttr.FileEvent.ContainerPath, nil

	case "removexattr.filename":

		return e.RemoveXAttr.FileEvent.PathnameStr, nil

	case "removexattr.inode":

		return int(e.RemoveXAttr.FileEvent.FileFields.Inode), nil

	case "removexattr.name":

		return e.RemoveXAttr.Name, nil

	case "removexattr.namespace":

		return e.RemoveXAttr.Namespace, nil

	case "removexattr.overlay_numlower":

		return int(e.RemoveXAttr.FileEvent.FileFields.OverlayNumLower), nil

	case "removexattr.retval":

		return int(e.RemoveXAttr.SyscallEvent.Retval), nil

	case "rename.new.basename":

		return e.Rename.New.BasenameStr, nil

	case "rename.new.container_path":

		return e.Rename.New.ContainerPath, nil

	case "rename.new.filename":

		return e.Rename.New.PathnameStr, nil

	case "rename.new.inode":

		return int(e.Rename.New.FileFields.Inode), nil

	case "rename.new.overlay_numlower":

		return int(e.Rename.New.FileFields.OverlayNumLower), nil

	case "rename.old.basename":

		return e.Rename.Old.BasenameStr, nil

	case "rename.old.container_path":

		return e.Rename.Old.ContainerPath, nil

	case "rename.old.filename":

		return e.Rename.Old.PathnameStr, nil

	case "rename.old.inode":

		return int(e.Rename.Old.FileFields.Inode), nil

	case "rename.old.overlay_numlower":

		return int(e.Rename.Old.FileFields.OverlayNumLower), nil

	case "rename.retval":

		return int(e.Rename.SyscallEvent.Retval), nil

	case "rmdir.basename":

		return e.Rmdir.FileEvent.BasenameStr, nil

	case "rmdir.container_path":

		return e.Rmdir.FileEvent.ContainerPath, nil

	case "rmdir.filename":

		return e.Rmdir.FileEvent.PathnameStr, nil

	case "rmdir.inode":

		return int(e.Rmdir.FileEvent.FileFields.Inode), nil

	case "rmdir.overlay_numlower":

		return int(e.Rmdir.FileEvent.FileFields.OverlayNumLower), nil

	case "rmdir.retval":

		return int(e.Rmdir.SyscallEvent.Retval), nil

	case "setxattr.basename":

		return e.SetXAttr.FileEvent.BasenameStr, nil

	case "setxattr.container_path":

		return e.SetXAttr.FileEvent.ContainerPath, nil

	case "setxattr.filename":

		return e.SetXAttr.FileEvent.PathnameStr, nil

	case "setxattr.inode":

		return int(e.SetXAttr.FileEvent.FileFields.Inode), nil

	case "setxattr.name":

		return e.SetXAttr.Name, nil

	case "setxattr.namespace":

		return e.SetXAttr.Namespace, nil

	case "setxattr.overlay_numlower":

		return int(e.SetXAttr.FileEvent.FileFields.OverlayNumLower), nil

	case "setxattr.retval":

		return int(e.SetXAttr.SyscallEvent.Retval), nil

	case "unlink.basename":

		return e.Unlink.FileEvent.BasenameStr, nil

	case "unlink.container_path":

		return e.Unlink.FileEvent.ContainerPath, nil

	case "unlink.filename":

		return e.Unlink.FileEvent.PathnameStr, nil

	case "unlink.flags":

		return int(e.Unlink.Flags), nil

	case "unlink.inode":

		return int(e.Unlink.FileEvent.FileFields.Inode), nil

	case "unlink.overlay_numlower":

		return int(e.Unlink.FileEvent.FileFields.OverlayNumLower), nil

	case "unlink.retval":

		return int(e.Unlink.SyscallEvent.Retval), nil

	case "utimes.basename":

		return e.Utimes.FileEvent.BasenameStr, nil

	case "utimes.container_path":

		return e.Utimes.FileEvent.ContainerPath, nil

	case "utimes.filename":

		return e.Utimes.FileEvent.PathnameStr, nil

	case "utimes.inode":

		return int(e.Utimes.FileEvent.FileFields.Inode), nil

	case "utimes.overlay_numlower":

		return int(e.Utimes.FileEvent.FileFields.OverlayNumLower), nil

	case "utimes.retval":

		return int(e.Utimes.SyscallEvent.Retval), nil

	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {

	case "chmod.basename":
		return "chmod", nil

	case "chmod.container_path":
		return "chmod", nil

	case "chmod.filename":
		return "chmod", nil

	case "chmod.inode":
		return "chmod", nil

	case "chmod.mode":
		return "chmod", nil

	case "chmod.overlay_numlower":
		return "chmod", nil

	case "chmod.retval":
		return "chmod", nil

	case "chown.basename":
		return "chown", nil

	case "chown.container_path":
		return "chown", nil

	case "chown.filename":
		return "chown", nil

	case "chown.gid":
		return "chown", nil

	case "chown.inode":
		return "chown", nil

	case "chown.overlay_numlower":
		return "chown", nil

	case "chown.retval":
		return "chown", nil

	case "chown.uid":
		return "chown", nil

	case "container.id":
		return "*", nil

	case "exec.container_path":
		return "exec", nil

	case "exec.cookie":
		return "exec", nil

	case "exec.filename":
		return "exec", nil

	case "exec.gid":
		return "exec", nil

	case "exec.group":
		return "exec", nil

	case "exec.inode":
		return "exec", nil

	case "exec.name":
		return "exec", nil

	case "exec.overlay_numlower":
		return "exec", nil

	case "exec.ppid":
		return "exec", nil

	case "exec.tty_name":
		return "exec", nil

	case "exec.uid":
		return "exec", nil

	case "exec.user":
		return "exec", nil

	case "link.retval":
		return "link", nil

	case "link.source.basename":
		return "link", nil

	case "link.source.container_path":
		return "link", nil

	case "link.source.filename":
		return "link", nil

	case "link.source.inode":
		return "link", nil

	case "link.source.overlay_numlower":
		return "link", nil

	case "link.target.basename":
		return "link", nil

	case "link.target.container_path":
		return "link", nil

	case "link.target.filename":
		return "link", nil

	case "link.target.inode":
		return "link", nil

	case "link.target.overlay_numlower":
		return "link", nil

	case "mkdir.basename":
		return "mkdir", nil

	case "mkdir.container_path":
		return "mkdir", nil

	case "mkdir.filename":
		return "mkdir", nil

	case "mkdir.inode":
		return "mkdir", nil

	case "mkdir.mode":
		return "mkdir", nil

	case "mkdir.overlay_numlower":
		return "mkdir", nil

	case "mkdir.retval":
		return "mkdir", nil

	case "open.basename":
		return "open", nil

	case "open.container_path":
		return "open", nil

	case "open.filename":
		return "open", nil

	case "open.flags":
		return "open", nil

	case "open.inode":
		return "open", nil

	case "open.mode":
		return "open", nil

	case "open.overlay_numlower":
		return "open", nil

	case "open.retval":
		return "open", nil

	case "process.ancestors.container_path":
		return "*", nil

	case "process.ancestors.cookie":
		return "*", nil

	case "process.ancestors.filename":
		return "*", nil

	case "process.ancestors.gid":
		return "*", nil

	case "process.ancestors.group":
		return "*", nil

	case "process.ancestors.id":
		return "*", nil

	case "process.ancestors.inode":
		return "*", nil

	case "process.ancestors.name":
		return "*", nil

	case "process.ancestors.overlay_numlower":
		return "*", nil

	case "process.ancestors.pid":
		return "*", nil

	case "process.ancestors.ppid":
		return "*", nil

	case "process.ancestors.tid":
		return "*", nil

	case "process.ancestors.tty_name":
		return "*", nil

	case "process.ancestors.uid":
		return "*", nil

	case "process.ancestors.user":
		return "*", nil

	case "process.container_path":
		return "*", nil

	case "process.cookie":
		return "*", nil

	case "process.filename":
		return "*", nil

	case "process.gid":
		return "*", nil

	case "process.group":
		return "*", nil

	case "process.inode":
		return "*", nil

	case "process.name":
		return "*", nil

	case "process.overlay_numlower":
		return "*", nil

	case "process.pid":
		return "*", nil

	case "process.ppid":
		return "*", nil

	case "process.tid":
		return "*", nil

	case "process.tty_name":
		return "*", nil

	case "process.uid":
		return "*", nil

	case "process.user":
		return "*", nil

	case "removexattr.basename":
		return "removexattr", nil

	case "removexattr.container_path":
		return "removexattr", nil

	case "removexattr.filename":
		return "removexattr", nil

	case "removexattr.inode":
		return "removexattr", nil

	case "removexattr.name":
		return "removexattr", nil

	case "removexattr.namespace":
		return "removexattr", nil

	case "removexattr.overlay_numlower":
		return "removexattr", nil

	case "removexattr.retval":
		return "removexattr", nil

	case "rename.new.basename":
		return "rename", nil

	case "rename.new.container_path":
		return "rename", nil

	case "rename.new.filename":
		return "rename", nil

	case "rename.new.inode":
		return "rename", nil

	case "rename.new.overlay_numlower":
		return "rename", nil

	case "rename.old.basename":
		return "rename", nil

	case "rename.old.container_path":
		return "rename", nil

	case "rename.old.filename":
		return "rename", nil

	case "rename.old.inode":
		return "rename", nil

	case "rename.old.overlay_numlower":
		return "rename", nil

	case "rename.retval":
		return "rename", nil

	case "rmdir.basename":
		return "rmdir", nil

	case "rmdir.container_path":
		return "rmdir", nil

	case "rmdir.filename":
		return "rmdir", nil

	case "rmdir.inode":
		return "rmdir", nil

	case "rmdir.overlay_numlower":
		return "rmdir", nil

	case "rmdir.retval":
		return "rmdir", nil

	case "setxattr.basename":
		return "setxattr", nil

	case "setxattr.container_path":
		return "setxattr", nil

	case "setxattr.filename":
		return "setxattr", nil

	case "setxattr.inode":
		return "setxattr", nil

	case "setxattr.name":
		return "setxattr", nil

	case "setxattr.namespace":
		return "setxattr", nil

	case "setxattr.overlay_numlower":
		return "setxattr", nil

	case "setxattr.retval":
		return "setxattr", nil

	case "unlink.basename":
		return "unlink", nil

	case "unlink.container_path":
		return "unlink", nil

	case "unlink.filename":
		return "unlink", nil

	case "unlink.flags":
		return "unlink", nil

	case "unlink.inode":
		return "unlink", nil

	case "unlink.overlay_numlower":
		return "unlink", nil

	case "unlink.retval":
		return "unlink", nil

	case "utimes.basename":
		return "utimes", nil

	case "utimes.container_path":
		return "utimes", nil

	case "utimes.filename":
		return "utimes", nil

	case "utimes.inode":
		return "utimes", nil

	case "utimes.overlay_numlower":
		return "utimes", nil

	case "utimes.retval":
		return "utimes", nil

	}

	return "", &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {

	case "chmod.basename":

		return reflect.String, nil

	case "chmod.container_path":

		return reflect.String, nil

	case "chmod.filename":

		return reflect.String, nil

	case "chmod.inode":

		return reflect.Int, nil

	case "chmod.mode":

		return reflect.Int, nil

	case "chmod.overlay_numlower":

		return reflect.Int, nil

	case "chmod.retval":

		return reflect.Int, nil

	case "chown.basename":

		return reflect.String, nil

	case "chown.container_path":

		return reflect.String, nil

	case "chown.filename":

		return reflect.String, nil

	case "chown.gid":

		return reflect.Int, nil

	case "chown.inode":

		return reflect.Int, nil

	case "chown.overlay_numlower":

		return reflect.Int, nil

	case "chown.retval":

		return reflect.Int, nil

	case "chown.uid":

		return reflect.Int, nil

	case "container.id":

		return reflect.String, nil

	case "exec.container_path":

		return reflect.String, nil

	case "exec.cookie":

		return reflect.Int, nil

	case "exec.filename":

		return reflect.String, nil

	case "exec.gid":

		return reflect.Int, nil

	case "exec.group":

		return reflect.String, nil

	case "exec.inode":

		return reflect.Int, nil

	case "exec.name":

		return reflect.String, nil

	case "exec.overlay_numlower":

		return reflect.Int, nil

	case "exec.ppid":

		return reflect.Int, nil

	case "exec.tty_name":

		return reflect.String, nil

	case "exec.uid":

		return reflect.Int, nil

	case "exec.user":

		return reflect.String, nil

	case "link.retval":

		return reflect.Int, nil

	case "link.source.basename":

		return reflect.String, nil

	case "link.source.container_path":

		return reflect.String, nil

	case "link.source.filename":

		return reflect.String, nil

	case "link.source.inode":

		return reflect.Int, nil

	case "link.source.overlay_numlower":

		return reflect.Int, nil

	case "link.target.basename":

		return reflect.String, nil

	case "link.target.container_path":

		return reflect.String, nil

	case "link.target.filename":

		return reflect.String, nil

	case "link.target.inode":

		return reflect.Int, nil

	case "link.target.overlay_numlower":

		return reflect.Int, nil

	case "mkdir.basename":

		return reflect.String, nil

	case "mkdir.container_path":

		return reflect.String, nil

	case "mkdir.filename":

		return reflect.String, nil

	case "mkdir.inode":

		return reflect.Int, nil

	case "mkdir.mode":

		return reflect.Int, nil

	case "mkdir.overlay_numlower":

		return reflect.Int, nil

	case "mkdir.retval":

		return reflect.Int, nil

	case "open.basename":

		return reflect.String, nil

	case "open.container_path":

		return reflect.String, nil

	case "open.filename":

		return reflect.String, nil

	case "open.flags":

		return reflect.Int, nil

	case "open.inode":

		return reflect.Int, nil

	case "open.mode":

		return reflect.Int, nil

	case "open.overlay_numlower":

		return reflect.Int, nil

	case "open.retval":

		return reflect.Int, nil

	case "process.ancestors.container_path":

		return reflect.String, nil

	case "process.ancestors.cookie":

		return reflect.Int, nil

	case "process.ancestors.filename":

		return reflect.String, nil

	case "process.ancestors.gid":

		return reflect.Int, nil

	case "process.ancestors.group":

		return reflect.String, nil

	case "process.ancestors.id":

		return reflect.String, nil

	case "process.ancestors.inode":

		return reflect.Int, nil

	case "process.ancestors.name":

		return reflect.String, nil

	case "process.ancestors.overlay_numlower":

		return reflect.Int, nil

	case "process.ancestors.pid":

		return reflect.Int, nil

	case "process.ancestors.ppid":

		return reflect.Int, nil

	case "process.ancestors.tid":

		return reflect.Int, nil

	case "process.ancestors.tty_name":

		return reflect.String, nil

	case "process.ancestors.uid":

		return reflect.Int, nil

	case "process.ancestors.user":

		return reflect.String, nil

	case "process.container_path":

		return reflect.String, nil

	case "process.cookie":

		return reflect.Int, nil

	case "process.filename":

		return reflect.String, nil

	case "process.gid":

		return reflect.Int, nil

	case "process.group":

		return reflect.String, nil

	case "process.inode":

		return reflect.Int, nil

	case "process.name":

		return reflect.String, nil

	case "process.overlay_numlower":

		return reflect.Int, nil

	case "process.pid":

		return reflect.Int, nil

	case "process.ppid":

		return reflect.Int, nil

	case "process.tid":

		return reflect.Int, nil

	case "process.tty_name":

		return reflect.String, nil

	case "process.uid":

		return reflect.Int, nil

	case "process.user":

		return reflect.String, nil

	case "removexattr.basename":

		return reflect.String, nil

	case "removexattr.container_path":

		return reflect.String, nil

	case "removexattr.filename":

		return reflect.String, nil

	case "removexattr.inode":

		return reflect.Int, nil

	case "removexattr.name":

		return reflect.String, nil

	case "removexattr.namespace":

		return reflect.String, nil

	case "removexattr.overlay_numlower":

		return reflect.Int, nil

	case "removexattr.retval":

		return reflect.Int, nil

	case "rename.new.basename":

		return reflect.String, nil

	case "rename.new.container_path":

		return reflect.String, nil

	case "rename.new.filename":

		return reflect.String, nil

	case "rename.new.inode":

		return reflect.Int, nil

	case "rename.new.overlay_numlower":

		return reflect.Int, nil

	case "rename.old.basename":

		return reflect.String, nil

	case "rename.old.container_path":

		return reflect.String, nil

	case "rename.old.filename":

		return reflect.String, nil

	case "rename.old.inode":

		return reflect.Int, nil

	case "rename.old.overlay_numlower":

		return reflect.Int, nil

	case "rename.retval":

		return reflect.Int, nil

	case "rmdir.basename":

		return reflect.String, nil

	case "rmdir.container_path":

		return reflect.String, nil

	case "rmdir.filename":

		return reflect.String, nil

	case "rmdir.inode":

		return reflect.Int, nil

	case "rmdir.overlay_numlower":

		return reflect.Int, nil

	case "rmdir.retval":

		return reflect.Int, nil

	case "setxattr.basename":

		return reflect.String, nil

	case "setxattr.container_path":

		return reflect.String, nil

	case "setxattr.filename":

		return reflect.String, nil

	case "setxattr.inode":

		return reflect.Int, nil

	case "setxattr.name":

		return reflect.String, nil

	case "setxattr.namespace":

		return reflect.String, nil

	case "setxattr.overlay_numlower":

		return reflect.Int, nil

	case "setxattr.retval":

		return reflect.Int, nil

	case "unlink.basename":

		return reflect.String, nil

	case "unlink.container_path":

		return reflect.String, nil

	case "unlink.filename":

		return reflect.String, nil

	case "unlink.flags":

		return reflect.Int, nil

	case "unlink.inode":

		return reflect.Int, nil

	case "unlink.overlay_numlower":

		return reflect.Int, nil

	case "unlink.retval":

		return reflect.Int, nil

	case "utimes.basename":

		return reflect.String, nil

	case "utimes.container_path":

		return reflect.String, nil

	case "utimes.filename":

		return reflect.String, nil

	case "utimes.inode":

		return reflect.Int, nil

	case "utimes.overlay_numlower":

		return reflect.Int, nil

	case "utimes.retval":

		return reflect.Int, nil

	}

	return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) SetFieldValue(field eval.Field, value interface{}) error {
	switch field {

	case "chmod.basename":

		var ok bool
		if e.Chmod.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.FileEvent.BasenameStr"}
		}
		return nil

	case "chmod.container_path":

		var ok bool
		if e.Chmod.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.FileEvent.ContainerPath"}
		}
		return nil

	case "chmod.filename":

		var ok bool
		if e.Chmod.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.FileEvent.PathnameStr"}
		}
		return nil

	case "chmod.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.FileEvent.FileFields.Inode"}
		}
		e.Chmod.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "chmod.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Mode"}
		}
		e.Chmod.Mode = uint32(v)
		return nil

	case "chmod.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.FileEvent.FileFields.OverlayNumLower"}
		}
		e.Chmod.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "chmod.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.SyscallEvent.Retval"}
		}
		e.Chmod.SyscallEvent.Retval = int64(v)
		return nil

	case "chown.basename":

		var ok bool
		if e.Chown.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.FileEvent.BasenameStr"}
		}
		return nil

	case "chown.container_path":

		var ok bool
		if e.Chown.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.FileEvent.ContainerPath"}
		}
		return nil

	case "chown.filename":

		var ok bool
		if e.Chown.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.FileEvent.PathnameStr"}
		}
		return nil

	case "chown.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.GID"}
		}
		e.Chown.GID = int32(v)
		return nil

	case "chown.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.FileEvent.FileFields.Inode"}
		}
		e.Chown.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "chown.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.FileEvent.FileFields.OverlayNumLower"}
		}
		e.Chown.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "chown.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.SyscallEvent.Retval"}
		}
		e.Chown.SyscallEvent.Retval = int64(v)
		return nil

	case "chown.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.UID"}
		}
		e.Chown.UID = int32(v)
		return nil

	case "container.id":

		var ok bool
		if e.Container.ID, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Container.ID"}
		}
		return nil

	case "exec.container_path":

		var ok bool
		if e.Exec.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.ContainerPath"}
		}
		return nil

	case "exec.cookie":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Cookie"}
		}
		e.Exec.Cookie = uint32(v)
		return nil

	case "exec.filename":

		var ok bool
		if e.Exec.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.PathnameStr"}
		}
		return nil

	case "exec.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.GID"}
		}
		e.Exec.GID = uint32(v)
		return nil

	case "exec.group":

		var ok bool
		if e.Exec.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Group"}
		}
		return nil

	case "exec.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.Inode"}
		}
		e.Exec.FileFields.Inode = uint64(v)
		return nil

	case "exec.name":

		var ok bool
		if e.Exec.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.BasenameStr"}
		}
		return nil

	case "exec.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.OverlayNumLower"}
		}
		e.Exec.FileFields.OverlayNumLower = int32(v)
		return nil

	case "exec.ppid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.PPid"}
		}
		e.Exec.PPid = uint32(v)
		return nil

	case "exec.tty_name":

		var ok bool
		if e.Exec.TTYName, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.TTYName"}
		}
		return nil

	case "exec.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.UID"}
		}
		e.Exec.UID = uint32(v)
		return nil

	case "exec.user":

		var ok bool
		if e.Exec.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.User"}
		}
		return nil

	case "link.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.SyscallEvent.Retval"}
		}
		e.Link.SyscallEvent.Retval = int64(v)
		return nil

	case "link.source.basename":

		var ok bool
		if e.Link.Source.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.BasenameStr"}
		}
		return nil

	case "link.source.container_path":

		var ok bool
		if e.Link.Source.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.ContainerPath"}
		}
		return nil

	case "link.source.filename":

		var ok bool
		if e.Link.Source.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.PathnameStr"}
		}
		return nil

	case "link.source.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Inode"}
		}
		e.Link.Source.FileFields.Inode = uint64(v)
		return nil

	case "link.source.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.OverlayNumLower"}
		}
		e.Link.Source.FileFields.OverlayNumLower = int32(v)
		return nil

	case "link.target.basename":

		var ok bool
		if e.Link.Target.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.BasenameStr"}
		}
		return nil

	case "link.target.container_path":

		var ok bool
		if e.Link.Target.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.ContainerPath"}
		}
		return nil

	case "link.target.filename":

		var ok bool
		if e.Link.Target.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.PathnameStr"}
		}
		return nil

	case "link.target.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Inode"}
		}
		e.Link.Target.FileFields.Inode = uint64(v)
		return nil

	case "link.target.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.OverlayNumLower"}
		}
		e.Link.Target.FileFields.OverlayNumLower = int32(v)
		return nil

	case "mkdir.basename":

		var ok bool
		if e.Mkdir.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.FileEvent.BasenameStr"}
		}
		return nil

	case "mkdir.container_path":

		var ok bool
		if e.Mkdir.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.FileEvent.ContainerPath"}
		}
		return nil

	case "mkdir.filename":

		var ok bool
		if e.Mkdir.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.FileEvent.PathnameStr"}
		}
		return nil

	case "mkdir.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.FileEvent.FileFields.Inode"}
		}
		e.Mkdir.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "mkdir.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Mode"}
		}
		e.Mkdir.Mode = uint32(v)
		return nil

	case "mkdir.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.FileEvent.FileFields.OverlayNumLower"}
		}
		e.Mkdir.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "mkdir.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.SyscallEvent.Retval"}
		}
		e.Mkdir.SyscallEvent.Retval = int64(v)
		return nil

	case "open.basename":

		var ok bool
		if e.Open.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.FileEvent.BasenameStr"}
		}
		return nil

	case "open.container_path":

		var ok bool
		if e.Open.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.FileEvent.ContainerPath"}
		}
		return nil

	case "open.filename":

		var ok bool
		if e.Open.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.FileEvent.PathnameStr"}
		}
		return nil

	case "open.flags":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Flags"}
		}
		e.Open.Flags = uint32(v)
		return nil

	case "open.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.FileEvent.FileFields.Inode"}
		}
		e.Open.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "open.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Mode"}
		}
		e.Open.Mode = uint32(v)
		return nil

	case "open.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.FileEvent.FileFields.OverlayNumLower"}
		}
		e.Open.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "open.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.SyscallEvent.Retval"}
		}
		e.Open.SyscallEvent.Retval = int64(v)
		return nil

	case "process.ancestors.container_path":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.ContainerPath"}
		}
		return nil

	case "process.ancestors.cookie":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Cookie"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Cookie = uint32(v)
		return nil

	case "process.ancestors.filename":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.PathnameStr"}
		}
		return nil

	case "process.ancestors.gid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.GID"}
		}
		e.Process.Ancestor.ProcessContext.GID = uint32(v)
		return nil

	case "process.ancestors.group":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Group"}
		}
		return nil

	case "process.ancestors.id":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ContainerContext.ID, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ContainerContext.ID"}
		}
		return nil

	case "process.ancestors.inode":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.FileFields.Inode"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.FileFields.Inode = uint64(v)
		return nil

	case "process.ancestors.name":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.BasenameStr"}
		}
		return nil

	case "process.ancestors.overlay_numlower":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.FileFields.OverlayNumLower"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "process.ancestors.pid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.Pid"}
		}
		e.Process.Ancestor.ProcessContext.Pid = uint32(v)
		return nil

	case "process.ancestors.ppid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.PPid"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.PPid = uint32(v)
		return nil

	case "process.ancestors.tid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.Tid"}
		}
		e.Process.Ancestor.ProcessContext.Tid = uint32(v)
		return nil

	case "process.ancestors.tty_name":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.TTYName, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.TTYName"}
		}
		return nil

	case "process.ancestors.uid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.UID"}
		}
		e.Process.Ancestor.ProcessContext.UID = uint32(v)
		return nil

	case "process.ancestors.user":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.User"}
		}
		return nil

	case "process.container_path":

		var ok bool
		if e.Process.ExecEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.ContainerPath"}
		}
		return nil

	case "process.cookie":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Cookie"}
		}
		e.Process.ExecEvent.Cookie = uint32(v)
		return nil

	case "process.filename":

		var ok bool
		if e.Process.ExecEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.PathnameStr"}
		}
		return nil

	case "process.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.GID"}
		}
		e.Process.GID = uint32(v)
		return nil

	case "process.group":

		var ok bool
		if e.Process.ExecEvent.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Group"}
		}
		return nil

	case "process.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.Inode"}
		}
		e.Process.ExecEvent.FileFields.Inode = uint64(v)
		return nil

	case "process.name":

		var ok bool
		if e.Process.ExecEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.BasenameStr"}
		}
		return nil

	case "process.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.OverlayNumLower"}
		}
		e.Process.ExecEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "process.pid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Pid"}
		}
		e.Process.Pid = uint32(v)
		return nil

	case "process.ppid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.PPid"}
		}
		e.Process.ExecEvent.PPid = uint32(v)
		return nil

	case "process.tid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Tid"}
		}
		e.Process.Tid = uint32(v)
		return nil

	case "process.tty_name":

		var ok bool
		if e.Process.ExecEvent.TTYName, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.TTYName"}
		}
		return nil

	case "process.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.UID"}
		}
		e.Process.UID = uint32(v)
		return nil

	case "process.user":

		var ok bool
		if e.Process.ExecEvent.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.User"}
		}
		return nil

	case "removexattr.basename":

		var ok bool
		if e.RemoveXAttr.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.FileEvent.BasenameStr"}
		}
		return nil

	case "removexattr.container_path":

		var ok bool
		if e.RemoveXAttr.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.FileEvent.ContainerPath"}
		}
		return nil

	case "removexattr.filename":

		var ok bool
		if e.RemoveXAttr.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.FileEvent.PathnameStr"}
		}
		return nil

	case "removexattr.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.FileEvent.FileFields.Inode"}
		}
		e.RemoveXAttr.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "removexattr.name":

		var ok bool
		if e.RemoveXAttr.Name, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Name"}
		}
		return nil

	case "removexattr.namespace":

		var ok bool
		if e.RemoveXAttr.Namespace, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Namespace"}
		}
		return nil

	case "removexattr.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.FileEvent.FileFields.OverlayNumLower"}
		}
		e.RemoveXAttr.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "removexattr.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.SyscallEvent.Retval"}
		}
		e.RemoveXAttr.SyscallEvent.Retval = int64(v)
		return nil

	case "rename.new.basename":

		var ok bool
		if e.Rename.New.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.BasenameStr"}
		}
		return nil

	case "rename.new.container_path":

		var ok bool
		if e.Rename.New.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.ContainerPath"}
		}
		return nil

	case "rename.new.filename":

		var ok bool
		if e.Rename.New.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.PathnameStr"}
		}
		return nil

	case "rename.new.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Inode"}
		}
		e.Rename.New.FileFields.Inode = uint64(v)
		return nil

	case "rename.new.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.OverlayNumLower"}
		}
		e.Rename.New.FileFields.OverlayNumLower = int32(v)
		return nil

	case "rename.old.basename":

		var ok bool
		if e.Rename.Old.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.BasenameStr"}
		}
		return nil

	case "rename.old.container_path":

		var ok bool
		if e.Rename.Old.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.ContainerPath"}
		}
		return nil

	case "rename.old.filename":

		var ok bool
		if e.Rename.Old.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.PathnameStr"}
		}
		return nil

	case "rename.old.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Inode"}
		}
		e.Rename.Old.FileFields.Inode = uint64(v)
		return nil

	case "rename.old.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.OverlayNumLower"}
		}
		e.Rename.Old.FileFields.OverlayNumLower = int32(v)
		return nil

	case "rename.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.SyscallEvent.Retval"}
		}
		e.Rename.SyscallEvent.Retval = int64(v)
		return nil

	case "rmdir.basename":

		var ok bool
		if e.Rmdir.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.FileEvent.BasenameStr"}
		}
		return nil

	case "rmdir.container_path":

		var ok bool
		if e.Rmdir.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.FileEvent.ContainerPath"}
		}
		return nil

	case "rmdir.filename":

		var ok bool
		if e.Rmdir.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.FileEvent.PathnameStr"}
		}
		return nil

	case "rmdir.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.FileEvent.FileFields.Inode"}
		}
		e.Rmdir.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "rmdir.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.FileEvent.FileFields.OverlayNumLower"}
		}
		e.Rmdir.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "rmdir.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.SyscallEvent.Retval"}
		}
		e.Rmdir.SyscallEvent.Retval = int64(v)
		return nil

	case "setxattr.basename":

		var ok bool
		if e.SetXAttr.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.FileEvent.BasenameStr"}
		}
		return nil

	case "setxattr.container_path":

		var ok bool
		if e.SetXAttr.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.FileEvent.ContainerPath"}
		}
		return nil

	case "setxattr.filename":

		var ok bool
		if e.SetXAttr.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.FileEvent.PathnameStr"}
		}
		return nil

	case "setxattr.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.FileEvent.FileFields.Inode"}
		}
		e.SetXAttr.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "setxattr.name":

		var ok bool
		if e.SetXAttr.Name, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Name"}
		}
		return nil

	case "setxattr.namespace":

		var ok bool
		if e.SetXAttr.Namespace, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Namespace"}
		}
		return nil

	case "setxattr.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.FileEvent.FileFields.OverlayNumLower"}
		}
		e.SetXAttr.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "setxattr.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.SyscallEvent.Retval"}
		}
		e.SetXAttr.SyscallEvent.Retval = int64(v)
		return nil

	case "unlink.basename":

		var ok bool
		if e.Unlink.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.FileEvent.BasenameStr"}
		}
		return nil

	case "unlink.container_path":

		var ok bool
		if e.Unlink.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.FileEvent.ContainerPath"}
		}
		return nil

	case "unlink.filename":

		var ok bool
		if e.Unlink.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.FileEvent.PathnameStr"}
		}
		return nil

	case "unlink.flags":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.Flags"}
		}
		e.Unlink.Flags = uint32(v)
		return nil

	case "unlink.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.FileEvent.FileFields.Inode"}
		}
		e.Unlink.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "unlink.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.FileEvent.FileFields.OverlayNumLower"}
		}
		e.Unlink.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "unlink.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.SyscallEvent.Retval"}
		}
		e.Unlink.SyscallEvent.Retval = int64(v)
		return nil

	case "utimes.basename":

		var ok bool
		if e.Utimes.FileEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.FileEvent.BasenameStr"}
		}
		return nil

	case "utimes.container_path":

		var ok bool
		if e.Utimes.FileEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.FileEvent.ContainerPath"}
		}
		return nil

	case "utimes.filename":

		var ok bool
		if e.Utimes.FileEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.FileEvent.PathnameStr"}
		}
		return nil

	case "utimes.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.FileEvent.FileFields.Inode"}
		}
		e.Utimes.FileEvent.FileFields.Inode = uint64(v)
		return nil

	case "utimes.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.FileEvent.FileFields.OverlayNumLower"}
		}
		e.Utimes.FileEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "utimes.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.SyscallEvent.Retval"}
		}
		e.Utimes.SyscallEvent.Retval = int64(v)
		return nil

	}

	return &eval.ErrFieldNotFound{Field: field}
}
