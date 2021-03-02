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

func (m *Model) GetEventTypes() []eval.EventType {
	return []eval.EventType{

		eval.EventType("capset"),

		eval.EventType("chmod"),

		eval.EventType("chown"),

		eval.EventType("exec"),

		eval.EventType("link"),

		eval.EventType("mkdir"),

		eval.EventType("open"),

		eval.EventType("removexattr"),

		eval.EventType("rename"),

		eval.EventType("rmdir"),

		eval.EventType("setgid"),

		eval.EventType("setuid"),

		eval.EventType("setxattr"),

		eval.EventType("unlink"),

		eval.EventType("utimes"),
	}
}

func (m *Model) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {

	case "capset.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Capset.CapEffective)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "capset.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Capset.CapPermitted)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chmod.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chmod.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chmod.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chmod.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chmod.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chmod.File.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chmod.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.destination.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.destination.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.destination.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.destination.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "chown.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Chown.File.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "chown.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Chown.SyscallEvent.Retval)

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

	case "exec.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Credentials.CapEffective)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Credentials.CapPermitted)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Comm

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

	case "exec.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Credentials.EGID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Credentials.EGroup

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Credentials.EUID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Credentials.EUser

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Credentials.FSGID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Credentials.FSGroup

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Credentials.FSUID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Credentials.FSUser

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Credentials.GID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Credentials.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
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

				return int((*Event)(ctx.Object).Exec.Credentials.UID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "exec.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Credentials.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Source.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.destination.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Target.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.destination.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Target.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.destination.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Target.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.destination.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Target.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Target.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.destination.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Target.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.destination.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Target.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.destination.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Target.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.destination.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Target.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.destination.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Target.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.destination.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Target.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Source.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Source.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Source.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Source.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Source.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Source.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Source.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Source.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Link.Source.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "link.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Link.Source.FileFields.User

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

	case "mkdir.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Mkdir.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Mkdir.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Mkdir.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Mkdir.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "mkdir.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Mkdir.File.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Mkdir.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Open.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "open.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Open.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "open.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Open.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "open.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Open.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "open.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "open.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Open.File.FileFields.User

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

	case "open.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Open.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.ancestors.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.Credentials.CapEffective)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.Credentials.CapPermitted)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.Comm

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

	case "process.ancestors.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.Credentials.EGID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.Credentials.EGroup

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.Credentials.EUID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.Credentials.EUser

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.container_path":
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

	case "process.ancestors.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.FileFields.GID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.FileFields.Group

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.inode":
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

	case "process.ancestors.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.FileFields.Mode)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.FileFields.MountID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.name":
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

	case "process.ancestors.file.overlay_numlower":
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

	case "process.ancestors.file.path":
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

	case "process.ancestors.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.FileFields.UID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.FileFields.User

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.Credentials.FSGID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.Credentials.FSGroup

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				var result int

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = int(element.ProcessContext.ExecEvent.Credentials.FSUID)

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				var result string

				reg := ctx.Registers[regID]
				if reg.Value != nil {
					element := (*ProcessCacheEntry)(reg.Value)

					result = element.ProcessContext.ExecEvent.Credentials.FSUser

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

					result = int(element.ProcessContext.ExecEvent.Credentials.GID)

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

					result = element.ProcessContext.ExecEvent.Credentials.Group

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

					result = int(element.ProcessContext.ExecEvent.Credentials.UID)

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

					result = element.ProcessContext.ExecEvent.Credentials.User

				}

				return result

			},
			Field: field,

			Weight: eval.IteratorWeight,
		}, nil

	case "process.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.Credentials.CapEffective)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.Credentials.CapPermitted)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.Comm

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

	case "process.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.Credentials.EGID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.Credentials.EGroup

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.Credentials.EUID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.Credentials.EUser

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.Credentials.FSGID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.Credentials.FSGroup

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.Credentials.FSUID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.Credentials.FSUser

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Process.ExecEvent.Credentials.GID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.Credentials.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
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

				return int((*Event)(ctx.Object).Process.ExecEvent.Credentials.UID)

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "process.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Process.ExecEvent.Credentials.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.destination.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.Name

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.destination.namespace":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.Namespace

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "removexattr.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "removexattr.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "removexattr.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "removexattr.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "removexattr.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "removexattr.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).RemoveXAttr.File.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).RemoveXAttr.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.Old.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.destination.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.New.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.destination.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.New.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.destination.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.New.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.destination.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.New.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.New.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.destination.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.New.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.destination.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.New.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.destination.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.New.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.destination.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.New.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.destination.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.New.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.destination.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.New.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.Old.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.Old.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.Old.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.Old.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.Old.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.Old.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.Old.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.Old.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.Old.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rename.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rename.Old.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rename.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rename.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rmdir.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rmdir.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rmdir.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rmdir.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "rmdir.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Rmdir.File.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Rmdir.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setgid.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetGID.EGID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setgid.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetGID.EGroup

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setgid.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetGID.FSGID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setgid.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetGID.FSGroup

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setgid.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetGID.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setgid.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetGID.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setuid.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetUID.EUID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setuid.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetUID.EUser

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setuid.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetUID.FSUID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setuid.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetUID.FSUser

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setuid.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetUID.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setuid.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetUID.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.destination.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.Name

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.destination.namespace":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.Namespace

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "setxattr.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).SetXAttr.File.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).SetXAttr.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Unlink.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Unlink.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Unlink.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Unlink.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "unlink.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Unlink.File.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Unlink.SyscallEvent.Retval)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.file.container_path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Utimes.File.ContainerPath

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.File.FileFields.GID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Utimes.File.FileFields.Group

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.File.FileFields.Inode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.File.FileFields.Mode)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.File.FileFields.MountID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Utimes.File.BasenameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.file.overlay_numlower":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.File.FileFields.OverlayNumLower)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Utimes.File.PathnameStr

			},
			Field: field,

			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Utimes.File.FileFields.UID)

			},
			Field: field,

			Weight: eval.FunctionWeight,
		}, nil

	case "utimes.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Utimes.File.FileFields.User

			},
			Field: field,

			Weight: eval.HandlerWeight,
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

		"capset.cap_effective",

		"capset.cap_permitted",

		"chmod.file.container_path",

		"chmod.file.destination.mode",

		"chmod.file.gid",

		"chmod.file.group",

		"chmod.file.inode",

		"chmod.file.mode",

		"chmod.file.mount_id",

		"chmod.file.name",

		"chmod.file.overlay_numlower",

		"chmod.file.path",

		"chmod.file.uid",

		"chmod.file.user",

		"chmod.retval",

		"chown.file.container_path",

		"chown.file.destination.gid",

		"chown.file.destination.group",

		"chown.file.destination.uid",

		"chown.file.destination.user",

		"chown.file.gid",

		"chown.file.group",

		"chown.file.inode",

		"chown.file.mode",

		"chown.file.mount_id",

		"chown.file.name",

		"chown.file.overlay_numlower",

		"chown.file.path",

		"chown.file.uid",

		"chown.file.user",

		"chown.retval",

		"container.id",

		"exec.cap_effective",

		"exec.cap_permitted",

		"exec.comm",

		"exec.cookie",

		"exec.egid",

		"exec.egroup",

		"exec.euid",

		"exec.euser",

		"exec.file.container_path",

		"exec.file.gid",

		"exec.file.group",

		"exec.file.inode",

		"exec.file.mode",

		"exec.file.mount_id",

		"exec.file.name",

		"exec.file.overlay_numlower",

		"exec.file.path",

		"exec.file.uid",

		"exec.file.user",

		"exec.fsgid",

		"exec.fsgroup",

		"exec.fsuid",

		"exec.fsuser",

		"exec.gid",

		"exec.group",

		"exec.ppid",

		"exec.tty_name",

		"exec.uid",

		"exec.user",

		"link.file.container_path",

		"link.file.destination.container_path",

		"link.file.destination.gid",

		"link.file.destination.group",

		"link.file.destination.inode",

		"link.file.destination.mode",

		"link.file.destination.mount_id",

		"link.file.destination.name",

		"link.file.destination.overlay_numlower",

		"link.file.destination.path",

		"link.file.destination.uid",

		"link.file.destination.user",

		"link.file.gid",

		"link.file.group",

		"link.file.inode",

		"link.file.mode",

		"link.file.mount_id",

		"link.file.name",

		"link.file.overlay_numlower",

		"link.file.path",

		"link.file.uid",

		"link.file.user",

		"link.retval",

		"mkdir.file.container_path",

		"mkdir.file.destination.mode",

		"mkdir.file.gid",

		"mkdir.file.group",

		"mkdir.file.inode",

		"mkdir.file.mode",

		"mkdir.file.mount_id",

		"mkdir.file.name",

		"mkdir.file.overlay_numlower",

		"mkdir.file.path",

		"mkdir.file.uid",

		"mkdir.file.user",

		"mkdir.retval",

		"open.file.container_path",

		"open.file.destination.mode",

		"open.file.gid",

		"open.file.group",

		"open.file.inode",

		"open.file.mode",

		"open.file.mount_id",

		"open.file.name",

		"open.file.overlay_numlower",

		"open.file.path",

		"open.file.uid",

		"open.file.user",

		"open.flags",

		"open.retval",

		"process.ancestors.cap_effective",

		"process.ancestors.cap_permitted",

		"process.ancestors.comm",

		"process.ancestors.cookie",

		"process.ancestors.egid",

		"process.ancestors.egroup",

		"process.ancestors.euid",

		"process.ancestors.euser",

		"process.ancestors.file.container_path",

		"process.ancestors.file.gid",

		"process.ancestors.file.group",

		"process.ancestors.file.inode",

		"process.ancestors.file.mode",

		"process.ancestors.file.mount_id",

		"process.ancestors.file.name",

		"process.ancestors.file.overlay_numlower",

		"process.ancestors.file.path",

		"process.ancestors.file.uid",

		"process.ancestors.file.user",

		"process.ancestors.fsgid",

		"process.ancestors.fsgroup",

		"process.ancestors.fsuid",

		"process.ancestors.fsuser",

		"process.ancestors.gid",

		"process.ancestors.group",

		"process.ancestors.id",

		"process.ancestors.pid",

		"process.ancestors.ppid",

		"process.ancestors.tid",

		"process.ancestors.tty_name",

		"process.ancestors.uid",

		"process.ancestors.user",

		"process.cap_effective",

		"process.cap_permitted",

		"process.comm",

		"process.cookie",

		"process.egid",

		"process.egroup",

		"process.euid",

		"process.euser",

		"process.file.container_path",

		"process.file.gid",

		"process.file.group",

		"process.file.inode",

		"process.file.mode",

		"process.file.mount_id",

		"process.file.name",

		"process.file.overlay_numlower",

		"process.file.path",

		"process.file.uid",

		"process.file.user",

		"process.fsgid",

		"process.fsgroup",

		"process.fsuid",

		"process.fsuser",

		"process.gid",

		"process.group",

		"process.pid",

		"process.ppid",

		"process.tid",

		"process.tty_name",

		"process.uid",

		"process.user",

		"removexattr.file.container_path",

		"removexattr.file.destination.name",

		"removexattr.file.destination.namespace",

		"removexattr.file.gid",

		"removexattr.file.group",

		"removexattr.file.inode",

		"removexattr.file.mode",

		"removexattr.file.mount_id",

		"removexattr.file.name",

		"removexattr.file.overlay_numlower",

		"removexattr.file.path",

		"removexattr.file.uid",

		"removexattr.file.user",

		"removexattr.retval",

		"rename.file.container_path",

		"rename.file.destination.container_path",

		"rename.file.destination.gid",

		"rename.file.destination.group",

		"rename.file.destination.inode",

		"rename.file.destination.mode",

		"rename.file.destination.mount_id",

		"rename.file.destination.name",

		"rename.file.destination.overlay_numlower",

		"rename.file.destination.path",

		"rename.file.destination.uid",

		"rename.file.destination.user",

		"rename.file.gid",

		"rename.file.group",

		"rename.file.inode",

		"rename.file.mode",

		"rename.file.mount_id",

		"rename.file.name",

		"rename.file.overlay_numlower",

		"rename.file.path",

		"rename.file.uid",

		"rename.file.user",

		"rename.retval",

		"rmdir.file.container_path",

		"rmdir.file.gid",

		"rmdir.file.group",

		"rmdir.file.inode",

		"rmdir.file.mode",

		"rmdir.file.mount_id",

		"rmdir.file.name",

		"rmdir.file.overlay_numlower",

		"rmdir.file.path",

		"rmdir.file.uid",

		"rmdir.file.user",

		"rmdir.retval",

		"setgid.egid",

		"setgid.egroup",

		"setgid.fsgid",

		"setgid.fsgroup",

		"setgid.gid",

		"setgid.group",

		"setuid.euid",

		"setuid.euser",

		"setuid.fsuid",

		"setuid.fsuser",

		"setuid.uid",

		"setuid.user",

		"setxattr.file.container_path",

		"setxattr.file.destination.name",

		"setxattr.file.destination.namespace",

		"setxattr.file.gid",

		"setxattr.file.group",

		"setxattr.file.inode",

		"setxattr.file.mode",

		"setxattr.file.mount_id",

		"setxattr.file.name",

		"setxattr.file.overlay_numlower",

		"setxattr.file.path",

		"setxattr.file.uid",

		"setxattr.file.user",

		"setxattr.retval",

		"unlink.file.container_path",

		"unlink.file.gid",

		"unlink.file.group",

		"unlink.file.inode",

		"unlink.file.mode",

		"unlink.file.mount_id",

		"unlink.file.name",

		"unlink.file.overlay_numlower",

		"unlink.file.path",

		"unlink.file.uid",

		"unlink.file.user",

		"unlink.retval",

		"utimes.file.container_path",

		"utimes.file.gid",

		"utimes.file.group",

		"utimes.file.inode",

		"utimes.file.mode",

		"utimes.file.mount_id",

		"utimes.file.name",

		"utimes.file.overlay_numlower",

		"utimes.file.path",

		"utimes.file.uid",

		"utimes.file.user",

		"utimes.retval",
	}
}

func (e *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {

	case "capset.cap_effective":

		return int(e.Capset.CapEffective), nil

	case "capset.cap_permitted":

		return int(e.Capset.CapPermitted), nil

	case "chmod.file.container_path":

		return e.Chmod.File.ContainerPath, nil

	case "chmod.file.destination.mode":

		return int(e.Chmod.Mode), nil

	case "chmod.file.gid":

		return int(e.Chmod.File.FileFields.GID), nil

	case "chmod.file.group":

		return e.Chmod.File.FileFields.Group, nil

	case "chmod.file.inode":

		return int(e.Chmod.File.FileFields.Inode), nil

	case "chmod.file.mode":

		return int(e.Chmod.File.FileFields.Mode), nil

	case "chmod.file.mount_id":

		return int(e.Chmod.File.FileFields.MountID), nil

	case "chmod.file.name":

		return e.Chmod.File.BasenameStr, nil

	case "chmod.file.overlay_numlower":

		return int(e.Chmod.File.FileFields.OverlayNumLower), nil

	case "chmod.file.path":

		return e.Chmod.File.PathnameStr, nil

	case "chmod.file.uid":

		return int(e.Chmod.File.FileFields.UID), nil

	case "chmod.file.user":

		return e.Chmod.File.FileFields.User, nil

	case "chmod.retval":

		return int(e.Chmod.SyscallEvent.Retval), nil

	case "chown.file.container_path":

		return e.Chown.File.ContainerPath, nil

	case "chown.file.destination.gid":

		return int(e.Chown.GID), nil

	case "chown.file.destination.group":

		return e.Chown.Group, nil

	case "chown.file.destination.uid":

		return int(e.Chown.UID), nil

	case "chown.file.destination.user":

		return e.Chown.User, nil

	case "chown.file.gid":

		return int(e.Chown.File.FileFields.GID), nil

	case "chown.file.group":

		return e.Chown.File.FileFields.Group, nil

	case "chown.file.inode":

		return int(e.Chown.File.FileFields.Inode), nil

	case "chown.file.mode":

		return int(e.Chown.File.FileFields.Mode), nil

	case "chown.file.mount_id":

		return int(e.Chown.File.FileFields.MountID), nil

	case "chown.file.name":

		return e.Chown.File.BasenameStr, nil

	case "chown.file.overlay_numlower":

		return int(e.Chown.File.FileFields.OverlayNumLower), nil

	case "chown.file.path":

		return e.Chown.File.PathnameStr, nil

	case "chown.file.uid":

		return int(e.Chown.File.FileFields.UID), nil

	case "chown.file.user":

		return e.Chown.File.FileFields.User, nil

	case "chown.retval":

		return int(e.Chown.SyscallEvent.Retval), nil

	case "container.id":

		return e.Container.ID, nil

	case "exec.cap_effective":

		return int(e.Exec.Credentials.CapEffective), nil

	case "exec.cap_permitted":

		return int(e.Exec.Credentials.CapPermitted), nil

	case "exec.comm":

		return e.Exec.Comm, nil

	case "exec.cookie":

		return int(e.Exec.Cookie), nil

	case "exec.egid":

		return int(e.Exec.Credentials.EGID), nil

	case "exec.egroup":

		return e.Exec.Credentials.EGroup, nil

	case "exec.euid":

		return int(e.Exec.Credentials.EUID), nil

	case "exec.euser":

		return e.Exec.Credentials.EUser, nil

	case "exec.file.container_path":

		return e.Exec.ContainerPath, nil

	case "exec.file.gid":

		return int(e.Exec.FileFields.GID), nil

	case "exec.file.group":

		return e.Exec.FileFields.Group, nil

	case "exec.file.inode":

		return int(e.Exec.FileFields.Inode), nil

	case "exec.file.mode":

		return int(e.Exec.FileFields.Mode), nil

	case "exec.file.mount_id":

		return int(e.Exec.FileFields.MountID), nil

	case "exec.file.name":

		return e.Exec.BasenameStr, nil

	case "exec.file.overlay_numlower":

		return int(e.Exec.FileFields.OverlayNumLower), nil

	case "exec.file.path":

		return e.Exec.PathnameStr, nil

	case "exec.file.uid":

		return int(e.Exec.FileFields.UID), nil

	case "exec.file.user":

		return e.Exec.FileFields.User, nil

	case "exec.fsgid":

		return int(e.Exec.Credentials.FSGID), nil

	case "exec.fsgroup":

		return e.Exec.Credentials.FSGroup, nil

	case "exec.fsuid":

		return int(e.Exec.Credentials.FSUID), nil

	case "exec.fsuser":

		return e.Exec.Credentials.FSUser, nil

	case "exec.gid":

		return int(e.Exec.Credentials.GID), nil

	case "exec.group":

		return e.Exec.Credentials.Group, nil

	case "exec.ppid":

		return int(e.Exec.PPid), nil

	case "exec.tty_name":

		return e.Exec.TTYName, nil

	case "exec.uid":

		return int(e.Exec.Credentials.UID), nil

	case "exec.user":

		return e.Exec.Credentials.User, nil

	case "link.file.container_path":

		return e.Link.Source.ContainerPath, nil

	case "link.file.destination.container_path":

		return e.Link.Target.ContainerPath, nil

	case "link.file.destination.gid":

		return int(e.Link.Target.FileFields.GID), nil

	case "link.file.destination.group":

		return e.Link.Target.FileFields.Group, nil

	case "link.file.destination.inode":

		return int(e.Link.Target.FileFields.Inode), nil

	case "link.file.destination.mode":

		return int(e.Link.Target.FileFields.Mode), nil

	case "link.file.destination.mount_id":

		return int(e.Link.Target.FileFields.MountID), nil

	case "link.file.destination.name":

		return e.Link.Target.BasenameStr, nil

	case "link.file.destination.overlay_numlower":

		return int(e.Link.Target.FileFields.OverlayNumLower), nil

	case "link.file.destination.path":

		return e.Link.Target.PathnameStr, nil

	case "link.file.destination.uid":

		return int(e.Link.Target.FileFields.UID), nil

	case "link.file.destination.user":

		return e.Link.Target.FileFields.User, nil

	case "link.file.gid":

		return int(e.Link.Source.FileFields.GID), nil

	case "link.file.group":

		return e.Link.Source.FileFields.Group, nil

	case "link.file.inode":

		return int(e.Link.Source.FileFields.Inode), nil

	case "link.file.mode":

		return int(e.Link.Source.FileFields.Mode), nil

	case "link.file.mount_id":

		return int(e.Link.Source.FileFields.MountID), nil

	case "link.file.name":

		return e.Link.Source.BasenameStr, nil

	case "link.file.overlay_numlower":

		return int(e.Link.Source.FileFields.OverlayNumLower), nil

	case "link.file.path":

		return e.Link.Source.PathnameStr, nil

	case "link.file.uid":

		return int(e.Link.Source.FileFields.UID), nil

	case "link.file.user":

		return e.Link.Source.FileFields.User, nil

	case "link.retval":

		return int(e.Link.SyscallEvent.Retval), nil

	case "mkdir.file.container_path":

		return e.Mkdir.File.ContainerPath, nil

	case "mkdir.file.destination.mode":

		return int(e.Mkdir.Mode), nil

	case "mkdir.file.gid":

		return int(e.Mkdir.File.FileFields.GID), nil

	case "mkdir.file.group":

		return e.Mkdir.File.FileFields.Group, nil

	case "mkdir.file.inode":

		return int(e.Mkdir.File.FileFields.Inode), nil

	case "mkdir.file.mode":

		return int(e.Mkdir.File.FileFields.Mode), nil

	case "mkdir.file.mount_id":

		return int(e.Mkdir.File.FileFields.MountID), nil

	case "mkdir.file.name":

		return e.Mkdir.File.BasenameStr, nil

	case "mkdir.file.overlay_numlower":

		return int(e.Mkdir.File.FileFields.OverlayNumLower), nil

	case "mkdir.file.path":

		return e.Mkdir.File.PathnameStr, nil

	case "mkdir.file.uid":

		return int(e.Mkdir.File.FileFields.UID), nil

	case "mkdir.file.user":

		return e.Mkdir.File.FileFields.User, nil

	case "mkdir.retval":

		return int(e.Mkdir.SyscallEvent.Retval), nil

	case "open.file.container_path":

		return e.Open.File.ContainerPath, nil

	case "open.file.destination.mode":

		return int(e.Open.Mode), nil

	case "open.file.gid":

		return int(e.Open.File.FileFields.GID), nil

	case "open.file.group":

		return e.Open.File.FileFields.Group, nil

	case "open.file.inode":

		return int(e.Open.File.FileFields.Inode), nil

	case "open.file.mode":

		return int(e.Open.File.FileFields.Mode), nil

	case "open.file.mount_id":

		return int(e.Open.File.FileFields.MountID), nil

	case "open.file.name":

		return e.Open.File.BasenameStr, nil

	case "open.file.overlay_numlower":

		return int(e.Open.File.FileFields.OverlayNumLower), nil

	case "open.file.path":

		return e.Open.File.PathnameStr, nil

	case "open.file.uid":

		return int(e.Open.File.FileFields.UID), nil

	case "open.file.user":

		return e.Open.File.FileFields.User, nil

	case "open.flags":

		return int(e.Open.Flags), nil

	case "open.retval":

		return int(e.Open.SyscallEvent.Retval), nil

	case "process.ancestors.cap_effective":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.Credentials.CapEffective)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.cap_permitted":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.Credentials.CapPermitted)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.comm":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.Comm

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

	case "process.ancestors.egid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.Credentials.EGID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.egroup":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.Credentials.EGroup

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.euid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.Credentials.EUID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.euser":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.Credentials.EUser

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.container_path":

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

	case "process.ancestors.file.gid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.FileFields.GID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.group":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.FileFields.Group

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.inode":

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

	case "process.ancestors.file.mode":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.FileFields.Mode)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.mount_id":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.FileFields.MountID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.name":

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

	case "process.ancestors.file.overlay_numlower":

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

	case "process.ancestors.file.path":

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

	case "process.ancestors.file.uid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.FileFields.UID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.user":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.FileFields.User

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.fsgid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.Credentials.FSGID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.fsgroup":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.Credentials.FSGroup

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.fsuid":

		var values []int

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.ExecEvent.Credentials.FSUID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.fsuser":

		var values []string

		ctx := &eval.Context{}
		ctx.SetObject(unsafe.Pointer(e))

		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)

			result := element.ProcessContext.ExecEvent.Credentials.FSUser

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

			result := int(element.ProcessContext.ExecEvent.Credentials.GID)

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

			result := element.ProcessContext.ExecEvent.Credentials.Group

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

			result := int(element.ProcessContext.ExecEvent.Credentials.UID)

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

			result := element.ProcessContext.ExecEvent.Credentials.User

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.cap_effective":

		return int(e.Process.ExecEvent.Credentials.CapEffective), nil

	case "process.cap_permitted":

		return int(e.Process.ExecEvent.Credentials.CapPermitted), nil

	case "process.comm":

		return e.Process.ExecEvent.Comm, nil

	case "process.cookie":

		return int(e.Process.ExecEvent.Cookie), nil

	case "process.egid":

		return int(e.Process.ExecEvent.Credentials.EGID), nil

	case "process.egroup":

		return e.Process.ExecEvent.Credentials.EGroup, nil

	case "process.euid":

		return int(e.Process.ExecEvent.Credentials.EUID), nil

	case "process.euser":

		return e.Process.ExecEvent.Credentials.EUser, nil

	case "process.file.container_path":

		return e.Process.ExecEvent.ContainerPath, nil

	case "process.file.gid":

		return int(e.Process.ExecEvent.FileFields.GID), nil

	case "process.file.group":

		return e.Process.ExecEvent.FileFields.Group, nil

	case "process.file.inode":

		return int(e.Process.ExecEvent.FileFields.Inode), nil

	case "process.file.mode":

		return int(e.Process.ExecEvent.FileFields.Mode), nil

	case "process.file.mount_id":

		return int(e.Process.ExecEvent.FileFields.MountID), nil

	case "process.file.name":

		return e.Process.ExecEvent.BasenameStr, nil

	case "process.file.overlay_numlower":

		return int(e.Process.ExecEvent.FileFields.OverlayNumLower), nil

	case "process.file.path":

		return e.Process.ExecEvent.PathnameStr, nil

	case "process.file.uid":

		return int(e.Process.ExecEvent.FileFields.UID), nil

	case "process.file.user":

		return e.Process.ExecEvent.FileFields.User, nil

	case "process.fsgid":

		return int(e.Process.ExecEvent.Credentials.FSGID), nil

	case "process.fsgroup":

		return e.Process.ExecEvent.Credentials.FSGroup, nil

	case "process.fsuid":

		return int(e.Process.ExecEvent.Credentials.FSUID), nil

	case "process.fsuser":

		return e.Process.ExecEvent.Credentials.FSUser, nil

	case "process.gid":

		return int(e.Process.ExecEvent.Credentials.GID), nil

	case "process.group":

		return e.Process.ExecEvent.Credentials.Group, nil

	case "process.pid":

		return int(e.Process.Pid), nil

	case "process.ppid":

		return int(e.Process.ExecEvent.PPid), nil

	case "process.tid":

		return int(e.Process.Tid), nil

	case "process.tty_name":

		return e.Process.ExecEvent.TTYName, nil

	case "process.uid":

		return int(e.Process.ExecEvent.Credentials.UID), nil

	case "process.user":

		return e.Process.ExecEvent.Credentials.User, nil

	case "removexattr.file.container_path":

		return e.RemoveXAttr.File.ContainerPath, nil

	case "removexattr.file.destination.name":

		return e.RemoveXAttr.Name, nil

	case "removexattr.file.destination.namespace":

		return e.RemoveXAttr.Namespace, nil

	case "removexattr.file.gid":

		return int(e.RemoveXAttr.File.FileFields.GID), nil

	case "removexattr.file.group":

		return e.RemoveXAttr.File.FileFields.Group, nil

	case "removexattr.file.inode":

		return int(e.RemoveXAttr.File.FileFields.Inode), nil

	case "removexattr.file.mode":

		return int(e.RemoveXAttr.File.FileFields.Mode), nil

	case "removexattr.file.mount_id":

		return int(e.RemoveXAttr.File.FileFields.MountID), nil

	case "removexattr.file.name":

		return e.RemoveXAttr.File.BasenameStr, nil

	case "removexattr.file.overlay_numlower":

		return int(e.RemoveXAttr.File.FileFields.OverlayNumLower), nil

	case "removexattr.file.path":

		return e.RemoveXAttr.File.PathnameStr, nil

	case "removexattr.file.uid":

		return int(e.RemoveXAttr.File.FileFields.UID), nil

	case "removexattr.file.user":

		return e.RemoveXAttr.File.FileFields.User, nil

	case "removexattr.retval":

		return int(e.RemoveXAttr.SyscallEvent.Retval), nil

	case "rename.file.container_path":

		return e.Rename.Old.ContainerPath, nil

	case "rename.file.destination.container_path":

		return e.Rename.New.ContainerPath, nil

	case "rename.file.destination.gid":

		return int(e.Rename.New.FileFields.GID), nil

	case "rename.file.destination.group":

		return e.Rename.New.FileFields.Group, nil

	case "rename.file.destination.inode":

		return int(e.Rename.New.FileFields.Inode), nil

	case "rename.file.destination.mode":

		return int(e.Rename.New.FileFields.Mode), nil

	case "rename.file.destination.mount_id":

		return int(e.Rename.New.FileFields.MountID), nil

	case "rename.file.destination.name":

		return e.Rename.New.BasenameStr, nil

	case "rename.file.destination.overlay_numlower":

		return int(e.Rename.New.FileFields.OverlayNumLower), nil

	case "rename.file.destination.path":

		return e.Rename.New.PathnameStr, nil

	case "rename.file.destination.uid":

		return int(e.Rename.New.FileFields.UID), nil

	case "rename.file.destination.user":

		return e.Rename.New.FileFields.User, nil

	case "rename.file.gid":

		return int(e.Rename.Old.FileFields.GID), nil

	case "rename.file.group":

		return e.Rename.Old.FileFields.Group, nil

	case "rename.file.inode":

		return int(e.Rename.Old.FileFields.Inode), nil

	case "rename.file.mode":

		return int(e.Rename.Old.FileFields.Mode), nil

	case "rename.file.mount_id":

		return int(e.Rename.Old.FileFields.MountID), nil

	case "rename.file.name":

		return e.Rename.Old.BasenameStr, nil

	case "rename.file.overlay_numlower":

		return int(e.Rename.Old.FileFields.OverlayNumLower), nil

	case "rename.file.path":

		return e.Rename.Old.PathnameStr, nil

	case "rename.file.uid":

		return int(e.Rename.Old.FileFields.UID), nil

	case "rename.file.user":

		return e.Rename.Old.FileFields.User, nil

	case "rename.retval":

		return int(e.Rename.SyscallEvent.Retval), nil

	case "rmdir.file.container_path":

		return e.Rmdir.File.ContainerPath, nil

	case "rmdir.file.gid":

		return int(e.Rmdir.File.FileFields.GID), nil

	case "rmdir.file.group":

		return e.Rmdir.File.FileFields.Group, nil

	case "rmdir.file.inode":

		return int(e.Rmdir.File.FileFields.Inode), nil

	case "rmdir.file.mode":

		return int(e.Rmdir.File.FileFields.Mode), nil

	case "rmdir.file.mount_id":

		return int(e.Rmdir.File.FileFields.MountID), nil

	case "rmdir.file.name":

		return e.Rmdir.File.BasenameStr, nil

	case "rmdir.file.overlay_numlower":

		return int(e.Rmdir.File.FileFields.OverlayNumLower), nil

	case "rmdir.file.path":

		return e.Rmdir.File.PathnameStr, nil

	case "rmdir.file.uid":

		return int(e.Rmdir.File.FileFields.UID), nil

	case "rmdir.file.user":

		return e.Rmdir.File.FileFields.User, nil

	case "rmdir.retval":

		return int(e.Rmdir.SyscallEvent.Retval), nil

	case "setgid.egid":

		return int(e.SetGID.EGID), nil

	case "setgid.egroup":

		return e.SetGID.EGroup, nil

	case "setgid.fsgid":

		return int(e.SetGID.FSGID), nil

	case "setgid.fsgroup":

		return e.SetGID.FSGroup, nil

	case "setgid.gid":

		return int(e.SetGID.GID), nil

	case "setgid.group":

		return e.SetGID.Group, nil

	case "setuid.euid":

		return int(e.SetUID.EUID), nil

	case "setuid.euser":

		return e.SetUID.EUser, nil

	case "setuid.fsuid":

		return int(e.SetUID.FSUID), nil

	case "setuid.fsuser":

		return e.SetUID.FSUser, nil

	case "setuid.uid":

		return int(e.SetUID.UID), nil

	case "setuid.user":

		return e.SetUID.User, nil

	case "setxattr.file.container_path":

		return e.SetXAttr.File.ContainerPath, nil

	case "setxattr.file.destination.name":

		return e.SetXAttr.Name, nil

	case "setxattr.file.destination.namespace":

		return e.SetXAttr.Namespace, nil

	case "setxattr.file.gid":

		return int(e.SetXAttr.File.FileFields.GID), nil

	case "setxattr.file.group":

		return e.SetXAttr.File.FileFields.Group, nil

	case "setxattr.file.inode":

		return int(e.SetXAttr.File.FileFields.Inode), nil

	case "setxattr.file.mode":

		return int(e.SetXAttr.File.FileFields.Mode), nil

	case "setxattr.file.mount_id":

		return int(e.SetXAttr.File.FileFields.MountID), nil

	case "setxattr.file.name":

		return e.SetXAttr.File.BasenameStr, nil

	case "setxattr.file.overlay_numlower":

		return int(e.SetXAttr.File.FileFields.OverlayNumLower), nil

	case "setxattr.file.path":

		return e.SetXAttr.File.PathnameStr, nil

	case "setxattr.file.uid":

		return int(e.SetXAttr.File.FileFields.UID), nil

	case "setxattr.file.user":

		return e.SetXAttr.File.FileFields.User, nil

	case "setxattr.retval":

		return int(e.SetXAttr.SyscallEvent.Retval), nil

	case "unlink.file.container_path":

		return e.Unlink.File.ContainerPath, nil

	case "unlink.file.gid":

		return int(e.Unlink.File.FileFields.GID), nil

	case "unlink.file.group":

		return e.Unlink.File.FileFields.Group, nil

	case "unlink.file.inode":

		return int(e.Unlink.File.FileFields.Inode), nil

	case "unlink.file.mode":

		return int(e.Unlink.File.FileFields.Mode), nil

	case "unlink.file.mount_id":

		return int(e.Unlink.File.FileFields.MountID), nil

	case "unlink.file.name":

		return e.Unlink.File.BasenameStr, nil

	case "unlink.file.overlay_numlower":

		return int(e.Unlink.File.FileFields.OverlayNumLower), nil

	case "unlink.file.path":

		return e.Unlink.File.PathnameStr, nil

	case "unlink.file.uid":

		return int(e.Unlink.File.FileFields.UID), nil

	case "unlink.file.user":

		return e.Unlink.File.FileFields.User, nil

	case "unlink.retval":

		return int(e.Unlink.SyscallEvent.Retval), nil

	case "utimes.file.container_path":

		return e.Utimes.File.ContainerPath, nil

	case "utimes.file.gid":

		return int(e.Utimes.File.FileFields.GID), nil

	case "utimes.file.group":

		return e.Utimes.File.FileFields.Group, nil

	case "utimes.file.inode":

		return int(e.Utimes.File.FileFields.Inode), nil

	case "utimes.file.mode":

		return int(e.Utimes.File.FileFields.Mode), nil

	case "utimes.file.mount_id":

		return int(e.Utimes.File.FileFields.MountID), nil

	case "utimes.file.name":

		return e.Utimes.File.BasenameStr, nil

	case "utimes.file.overlay_numlower":

		return int(e.Utimes.File.FileFields.OverlayNumLower), nil

	case "utimes.file.path":

		return e.Utimes.File.PathnameStr, nil

	case "utimes.file.uid":

		return int(e.Utimes.File.FileFields.UID), nil

	case "utimes.file.user":

		return e.Utimes.File.FileFields.User, nil

	case "utimes.retval":

		return int(e.Utimes.SyscallEvent.Retval), nil

	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {

	case "capset.cap_effective":
		return "capset", nil

	case "capset.cap_permitted":
		return "capset", nil

	case "chmod.file.container_path":
		return "chmod", nil

	case "chmod.file.destination.mode":
		return "chmod", nil

	case "chmod.file.gid":
		return "chmod", nil

	case "chmod.file.group":
		return "chmod", nil

	case "chmod.file.inode":
		return "chmod", nil

	case "chmod.file.mode":
		return "chmod", nil

	case "chmod.file.mount_id":
		return "chmod", nil

	case "chmod.file.name":
		return "chmod", nil

	case "chmod.file.overlay_numlower":
		return "chmod", nil

	case "chmod.file.path":
		return "chmod", nil

	case "chmod.file.uid":
		return "chmod", nil

	case "chmod.file.user":
		return "chmod", nil

	case "chmod.retval":
		return "chmod", nil

	case "chown.file.container_path":
		return "chown", nil

	case "chown.file.destination.gid":
		return "chown", nil

	case "chown.file.destination.group":
		return "chown", nil

	case "chown.file.destination.uid":
		return "chown", nil

	case "chown.file.destination.user":
		return "chown", nil

	case "chown.file.gid":
		return "chown", nil

	case "chown.file.group":
		return "chown", nil

	case "chown.file.inode":
		return "chown", nil

	case "chown.file.mode":
		return "chown", nil

	case "chown.file.mount_id":
		return "chown", nil

	case "chown.file.name":
		return "chown", nil

	case "chown.file.overlay_numlower":
		return "chown", nil

	case "chown.file.path":
		return "chown", nil

	case "chown.file.uid":
		return "chown", nil

	case "chown.file.user":
		return "chown", nil

	case "chown.retval":
		return "chown", nil

	case "container.id":
		return "*", nil

	case "exec.cap_effective":
		return "exec", nil

	case "exec.cap_permitted":
		return "exec", nil

	case "exec.comm":
		return "exec", nil

	case "exec.cookie":
		return "exec", nil

	case "exec.egid":
		return "exec", nil

	case "exec.egroup":
		return "exec", nil

	case "exec.euid":
		return "exec", nil

	case "exec.euser":
		return "exec", nil

	case "exec.file.container_path":
		return "exec", nil

	case "exec.file.gid":
		return "exec", nil

	case "exec.file.group":
		return "exec", nil

	case "exec.file.inode":
		return "exec", nil

	case "exec.file.mode":
		return "exec", nil

	case "exec.file.mount_id":
		return "exec", nil

	case "exec.file.name":
		return "exec", nil

	case "exec.file.overlay_numlower":
		return "exec", nil

	case "exec.file.path":
		return "exec", nil

	case "exec.file.uid":
		return "exec", nil

	case "exec.file.user":
		return "exec", nil

	case "exec.fsgid":
		return "exec", nil

	case "exec.fsgroup":
		return "exec", nil

	case "exec.fsuid":
		return "exec", nil

	case "exec.fsuser":
		return "exec", nil

	case "exec.gid":
		return "exec", nil

	case "exec.group":
		return "exec", nil

	case "exec.ppid":
		return "exec", nil

	case "exec.tty_name":
		return "exec", nil

	case "exec.uid":
		return "exec", nil

	case "exec.user":
		return "exec", nil

	case "link.file.container_path":
		return "link", nil

	case "link.file.destination.container_path":
		return "link", nil

	case "link.file.destination.gid":
		return "link", nil

	case "link.file.destination.group":
		return "link", nil

	case "link.file.destination.inode":
		return "link", nil

	case "link.file.destination.mode":
		return "link", nil

	case "link.file.destination.mount_id":
		return "link", nil

	case "link.file.destination.name":
		return "link", nil

	case "link.file.destination.overlay_numlower":
		return "link", nil

	case "link.file.destination.path":
		return "link", nil

	case "link.file.destination.uid":
		return "link", nil

	case "link.file.destination.user":
		return "link", nil

	case "link.file.gid":
		return "link", nil

	case "link.file.group":
		return "link", nil

	case "link.file.inode":
		return "link", nil

	case "link.file.mode":
		return "link", nil

	case "link.file.mount_id":
		return "link", nil

	case "link.file.name":
		return "link", nil

	case "link.file.overlay_numlower":
		return "link", nil

	case "link.file.path":
		return "link", nil

	case "link.file.uid":
		return "link", nil

	case "link.file.user":
		return "link", nil

	case "link.retval":
		return "link", nil

	case "mkdir.file.container_path":
		return "mkdir", nil

	case "mkdir.file.destination.mode":
		return "mkdir", nil

	case "mkdir.file.gid":
		return "mkdir", nil

	case "mkdir.file.group":
		return "mkdir", nil

	case "mkdir.file.inode":
		return "mkdir", nil

	case "mkdir.file.mode":
		return "mkdir", nil

	case "mkdir.file.mount_id":
		return "mkdir", nil

	case "mkdir.file.name":
		return "mkdir", nil

	case "mkdir.file.overlay_numlower":
		return "mkdir", nil

	case "mkdir.file.path":
		return "mkdir", nil

	case "mkdir.file.uid":
		return "mkdir", nil

	case "mkdir.file.user":
		return "mkdir", nil

	case "mkdir.retval":
		return "mkdir", nil

	case "open.file.container_path":
		return "open", nil

	case "open.file.destination.mode":
		return "open", nil

	case "open.file.gid":
		return "open", nil

	case "open.file.group":
		return "open", nil

	case "open.file.inode":
		return "open", nil

	case "open.file.mode":
		return "open", nil

	case "open.file.mount_id":
		return "open", nil

	case "open.file.name":
		return "open", nil

	case "open.file.overlay_numlower":
		return "open", nil

	case "open.file.path":
		return "open", nil

	case "open.file.uid":
		return "open", nil

	case "open.file.user":
		return "open", nil

	case "open.flags":
		return "open", nil

	case "open.retval":
		return "open", nil

	case "process.ancestors.cap_effective":
		return "*", nil

	case "process.ancestors.cap_permitted":
		return "*", nil

	case "process.ancestors.comm":
		return "*", nil

	case "process.ancestors.cookie":
		return "*", nil

	case "process.ancestors.egid":
		return "*", nil

	case "process.ancestors.egroup":
		return "*", nil

	case "process.ancestors.euid":
		return "*", nil

	case "process.ancestors.euser":
		return "*", nil

	case "process.ancestors.file.container_path":
		return "*", nil

	case "process.ancestors.file.gid":
		return "*", nil

	case "process.ancestors.file.group":
		return "*", nil

	case "process.ancestors.file.inode":
		return "*", nil

	case "process.ancestors.file.mode":
		return "*", nil

	case "process.ancestors.file.mount_id":
		return "*", nil

	case "process.ancestors.file.name":
		return "*", nil

	case "process.ancestors.file.overlay_numlower":
		return "*", nil

	case "process.ancestors.file.path":
		return "*", nil

	case "process.ancestors.file.uid":
		return "*", nil

	case "process.ancestors.file.user":
		return "*", nil

	case "process.ancestors.fsgid":
		return "*", nil

	case "process.ancestors.fsgroup":
		return "*", nil

	case "process.ancestors.fsuid":
		return "*", nil

	case "process.ancestors.fsuser":
		return "*", nil

	case "process.ancestors.gid":
		return "*", nil

	case "process.ancestors.group":
		return "*", nil

	case "process.ancestors.id":
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

	case "process.cap_effective":
		return "*", nil

	case "process.cap_permitted":
		return "*", nil

	case "process.comm":
		return "*", nil

	case "process.cookie":
		return "*", nil

	case "process.egid":
		return "*", nil

	case "process.egroup":
		return "*", nil

	case "process.euid":
		return "*", nil

	case "process.euser":
		return "*", nil

	case "process.file.container_path":
		return "*", nil

	case "process.file.gid":
		return "*", nil

	case "process.file.group":
		return "*", nil

	case "process.file.inode":
		return "*", nil

	case "process.file.mode":
		return "*", nil

	case "process.file.mount_id":
		return "*", nil

	case "process.file.name":
		return "*", nil

	case "process.file.overlay_numlower":
		return "*", nil

	case "process.file.path":
		return "*", nil

	case "process.file.uid":
		return "*", nil

	case "process.file.user":
		return "*", nil

	case "process.fsgid":
		return "*", nil

	case "process.fsgroup":
		return "*", nil

	case "process.fsuid":
		return "*", nil

	case "process.fsuser":
		return "*", nil

	case "process.gid":
		return "*", nil

	case "process.group":
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

	case "removexattr.file.container_path":
		return "removexattr", nil

	case "removexattr.file.destination.name":
		return "removexattr", nil

	case "removexattr.file.destination.namespace":
		return "removexattr", nil

	case "removexattr.file.gid":
		return "removexattr", nil

	case "removexattr.file.group":
		return "removexattr", nil

	case "removexattr.file.inode":
		return "removexattr", nil

	case "removexattr.file.mode":
		return "removexattr", nil

	case "removexattr.file.mount_id":
		return "removexattr", nil

	case "removexattr.file.name":
		return "removexattr", nil

	case "removexattr.file.overlay_numlower":
		return "removexattr", nil

	case "removexattr.file.path":
		return "removexattr", nil

	case "removexattr.file.uid":
		return "removexattr", nil

	case "removexattr.file.user":
		return "removexattr", nil

	case "removexattr.retval":
		return "removexattr", nil

	case "rename.file.container_path":
		return "rename", nil

	case "rename.file.destination.container_path":
		return "rename", nil

	case "rename.file.destination.gid":
		return "rename", nil

	case "rename.file.destination.group":
		return "rename", nil

	case "rename.file.destination.inode":
		return "rename", nil

	case "rename.file.destination.mode":
		return "rename", nil

	case "rename.file.destination.mount_id":
		return "rename", nil

	case "rename.file.destination.name":
		return "rename", nil

	case "rename.file.destination.overlay_numlower":
		return "rename", nil

	case "rename.file.destination.path":
		return "rename", nil

	case "rename.file.destination.uid":
		return "rename", nil

	case "rename.file.destination.user":
		return "rename", nil

	case "rename.file.gid":
		return "rename", nil

	case "rename.file.group":
		return "rename", nil

	case "rename.file.inode":
		return "rename", nil

	case "rename.file.mode":
		return "rename", nil

	case "rename.file.mount_id":
		return "rename", nil

	case "rename.file.name":
		return "rename", nil

	case "rename.file.overlay_numlower":
		return "rename", nil

	case "rename.file.path":
		return "rename", nil

	case "rename.file.uid":
		return "rename", nil

	case "rename.file.user":
		return "rename", nil

	case "rename.retval":
		return "rename", nil

	case "rmdir.file.container_path":
		return "rmdir", nil

	case "rmdir.file.gid":
		return "rmdir", nil

	case "rmdir.file.group":
		return "rmdir", nil

	case "rmdir.file.inode":
		return "rmdir", nil

	case "rmdir.file.mode":
		return "rmdir", nil

	case "rmdir.file.mount_id":
		return "rmdir", nil

	case "rmdir.file.name":
		return "rmdir", nil

	case "rmdir.file.overlay_numlower":
		return "rmdir", nil

	case "rmdir.file.path":
		return "rmdir", nil

	case "rmdir.file.uid":
		return "rmdir", nil

	case "rmdir.file.user":
		return "rmdir", nil

	case "rmdir.retval":
		return "rmdir", nil

	case "setgid.egid":
		return "setgid", nil

	case "setgid.egroup":
		return "setgid", nil

	case "setgid.fsgid":
		return "setgid", nil

	case "setgid.fsgroup":
		return "setgid", nil

	case "setgid.gid":
		return "setgid", nil

	case "setgid.group":
		return "setgid", nil

	case "setuid.euid":
		return "setuid", nil

	case "setuid.euser":
		return "setuid", nil

	case "setuid.fsuid":
		return "setuid", nil

	case "setuid.fsuser":
		return "setuid", nil

	case "setuid.uid":
		return "setuid", nil

	case "setuid.user":
		return "setuid", nil

	case "setxattr.file.container_path":
		return "setxattr", nil

	case "setxattr.file.destination.name":
		return "setxattr", nil

	case "setxattr.file.destination.namespace":
		return "setxattr", nil

	case "setxattr.file.gid":
		return "setxattr", nil

	case "setxattr.file.group":
		return "setxattr", nil

	case "setxattr.file.inode":
		return "setxattr", nil

	case "setxattr.file.mode":
		return "setxattr", nil

	case "setxattr.file.mount_id":
		return "setxattr", nil

	case "setxattr.file.name":
		return "setxattr", nil

	case "setxattr.file.overlay_numlower":
		return "setxattr", nil

	case "setxattr.file.path":
		return "setxattr", nil

	case "setxattr.file.uid":
		return "setxattr", nil

	case "setxattr.file.user":
		return "setxattr", nil

	case "setxattr.retval":
		return "setxattr", nil

	case "unlink.file.container_path":
		return "unlink", nil

	case "unlink.file.gid":
		return "unlink", nil

	case "unlink.file.group":
		return "unlink", nil

	case "unlink.file.inode":
		return "unlink", nil

	case "unlink.file.mode":
		return "unlink", nil

	case "unlink.file.mount_id":
		return "unlink", nil

	case "unlink.file.name":
		return "unlink", nil

	case "unlink.file.overlay_numlower":
		return "unlink", nil

	case "unlink.file.path":
		return "unlink", nil

	case "unlink.file.uid":
		return "unlink", nil

	case "unlink.file.user":
		return "unlink", nil

	case "unlink.retval":
		return "unlink", nil

	case "utimes.file.container_path":
		return "utimes", nil

	case "utimes.file.gid":
		return "utimes", nil

	case "utimes.file.group":
		return "utimes", nil

	case "utimes.file.inode":
		return "utimes", nil

	case "utimes.file.mode":
		return "utimes", nil

	case "utimes.file.mount_id":
		return "utimes", nil

	case "utimes.file.name":
		return "utimes", nil

	case "utimes.file.overlay_numlower":
		return "utimes", nil

	case "utimes.file.path":
		return "utimes", nil

	case "utimes.file.uid":
		return "utimes", nil

	case "utimes.file.user":
		return "utimes", nil

	case "utimes.retval":
		return "utimes", nil

	}

	return "", &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {

	case "capset.cap_effective":

		return reflect.Int, nil

	case "capset.cap_permitted":

		return reflect.Int, nil

	case "chmod.file.container_path":

		return reflect.String, nil

	case "chmod.file.destination.mode":

		return reflect.Int, nil

	case "chmod.file.gid":

		return reflect.Int, nil

	case "chmod.file.group":

		return reflect.String, nil

	case "chmod.file.inode":

		return reflect.Int, nil

	case "chmod.file.mode":

		return reflect.Int, nil

	case "chmod.file.mount_id":

		return reflect.Int, nil

	case "chmod.file.name":

		return reflect.String, nil

	case "chmod.file.overlay_numlower":

		return reflect.Int, nil

	case "chmod.file.path":

		return reflect.String, nil

	case "chmod.file.uid":

		return reflect.Int, nil

	case "chmod.file.user":

		return reflect.String, nil

	case "chmod.retval":

		return reflect.Int, nil

	case "chown.file.container_path":

		return reflect.String, nil

	case "chown.file.destination.gid":

		return reflect.Int, nil

	case "chown.file.destination.group":

		return reflect.String, nil

	case "chown.file.destination.uid":

		return reflect.Int, nil

	case "chown.file.destination.user":

		return reflect.String, nil

	case "chown.file.gid":

		return reflect.Int, nil

	case "chown.file.group":

		return reflect.String, nil

	case "chown.file.inode":

		return reflect.Int, nil

	case "chown.file.mode":

		return reflect.Int, nil

	case "chown.file.mount_id":

		return reflect.Int, nil

	case "chown.file.name":

		return reflect.String, nil

	case "chown.file.overlay_numlower":

		return reflect.Int, nil

	case "chown.file.path":

		return reflect.String, nil

	case "chown.file.uid":

		return reflect.Int, nil

	case "chown.file.user":

		return reflect.String, nil

	case "chown.retval":

		return reflect.Int, nil

	case "container.id":

		return reflect.String, nil

	case "exec.cap_effective":

		return reflect.Int, nil

	case "exec.cap_permitted":

		return reflect.Int, nil

	case "exec.comm":

		return reflect.String, nil

	case "exec.cookie":

		return reflect.Int, nil

	case "exec.egid":

		return reflect.Int, nil

	case "exec.egroup":

		return reflect.String, nil

	case "exec.euid":

		return reflect.Int, nil

	case "exec.euser":

		return reflect.String, nil

	case "exec.file.container_path":

		return reflect.String, nil

	case "exec.file.gid":

		return reflect.Int, nil

	case "exec.file.group":

		return reflect.String, nil

	case "exec.file.inode":

		return reflect.Int, nil

	case "exec.file.mode":

		return reflect.Int, nil

	case "exec.file.mount_id":

		return reflect.Int, nil

	case "exec.file.name":

		return reflect.String, nil

	case "exec.file.overlay_numlower":

		return reflect.Int, nil

	case "exec.file.path":

		return reflect.String, nil

	case "exec.file.uid":

		return reflect.Int, nil

	case "exec.file.user":

		return reflect.String, nil

	case "exec.fsgid":

		return reflect.Int, nil

	case "exec.fsgroup":

		return reflect.String, nil

	case "exec.fsuid":

		return reflect.Int, nil

	case "exec.fsuser":

		return reflect.String, nil

	case "exec.gid":

		return reflect.Int, nil

	case "exec.group":

		return reflect.String, nil

	case "exec.ppid":

		return reflect.Int, nil

	case "exec.tty_name":

		return reflect.String, nil

	case "exec.uid":

		return reflect.Int, nil

	case "exec.user":

		return reflect.String, nil

	case "link.file.container_path":

		return reflect.String, nil

	case "link.file.destination.container_path":

		return reflect.String, nil

	case "link.file.destination.gid":

		return reflect.Int, nil

	case "link.file.destination.group":

		return reflect.String, nil

	case "link.file.destination.inode":

		return reflect.Int, nil

	case "link.file.destination.mode":

		return reflect.Int, nil

	case "link.file.destination.mount_id":

		return reflect.Int, nil

	case "link.file.destination.name":

		return reflect.String, nil

	case "link.file.destination.overlay_numlower":

		return reflect.Int, nil

	case "link.file.destination.path":

		return reflect.String, nil

	case "link.file.destination.uid":

		return reflect.Int, nil

	case "link.file.destination.user":

		return reflect.String, nil

	case "link.file.gid":

		return reflect.Int, nil

	case "link.file.group":

		return reflect.String, nil

	case "link.file.inode":

		return reflect.Int, nil

	case "link.file.mode":

		return reflect.Int, nil

	case "link.file.mount_id":

		return reflect.Int, nil

	case "link.file.name":

		return reflect.String, nil

	case "link.file.overlay_numlower":

		return reflect.Int, nil

	case "link.file.path":

		return reflect.String, nil

	case "link.file.uid":

		return reflect.Int, nil

	case "link.file.user":

		return reflect.String, nil

	case "link.retval":

		return reflect.Int, nil

	case "mkdir.file.container_path":

		return reflect.String, nil

	case "mkdir.file.destination.mode":

		return reflect.Int, nil

	case "mkdir.file.gid":

		return reflect.Int, nil

	case "mkdir.file.group":

		return reflect.String, nil

	case "mkdir.file.inode":

		return reflect.Int, nil

	case "mkdir.file.mode":

		return reflect.Int, nil

	case "mkdir.file.mount_id":

		return reflect.Int, nil

	case "mkdir.file.name":

		return reflect.String, nil

	case "mkdir.file.overlay_numlower":

		return reflect.Int, nil

	case "mkdir.file.path":

		return reflect.String, nil

	case "mkdir.file.uid":

		return reflect.Int, nil

	case "mkdir.file.user":

		return reflect.String, nil

	case "mkdir.retval":

		return reflect.Int, nil

	case "open.file.container_path":

		return reflect.String, nil

	case "open.file.destination.mode":

		return reflect.Int, nil

	case "open.file.gid":

		return reflect.Int, nil

	case "open.file.group":

		return reflect.String, nil

	case "open.file.inode":

		return reflect.Int, nil

	case "open.file.mode":

		return reflect.Int, nil

	case "open.file.mount_id":

		return reflect.Int, nil

	case "open.file.name":

		return reflect.String, nil

	case "open.file.overlay_numlower":

		return reflect.Int, nil

	case "open.file.path":

		return reflect.String, nil

	case "open.file.uid":

		return reflect.Int, nil

	case "open.file.user":

		return reflect.String, nil

	case "open.flags":

		return reflect.Int, nil

	case "open.retval":

		return reflect.Int, nil

	case "process.ancestors.cap_effective":

		return reflect.Int, nil

	case "process.ancestors.cap_permitted":

		return reflect.Int, nil

	case "process.ancestors.comm":

		return reflect.String, nil

	case "process.ancestors.cookie":

		return reflect.Int, nil

	case "process.ancestors.egid":

		return reflect.Int, nil

	case "process.ancestors.egroup":

		return reflect.String, nil

	case "process.ancestors.euid":

		return reflect.Int, nil

	case "process.ancestors.euser":

		return reflect.String, nil

	case "process.ancestors.file.container_path":

		return reflect.String, nil

	case "process.ancestors.file.gid":

		return reflect.Int, nil

	case "process.ancestors.file.group":

		return reflect.String, nil

	case "process.ancestors.file.inode":

		return reflect.Int, nil

	case "process.ancestors.file.mode":

		return reflect.Int, nil

	case "process.ancestors.file.mount_id":

		return reflect.Int, nil

	case "process.ancestors.file.name":

		return reflect.String, nil

	case "process.ancestors.file.overlay_numlower":

		return reflect.Int, nil

	case "process.ancestors.file.path":

		return reflect.String, nil

	case "process.ancestors.file.uid":

		return reflect.Int, nil

	case "process.ancestors.file.user":

		return reflect.String, nil

	case "process.ancestors.fsgid":

		return reflect.Int, nil

	case "process.ancestors.fsgroup":

		return reflect.String, nil

	case "process.ancestors.fsuid":

		return reflect.Int, nil

	case "process.ancestors.fsuser":

		return reflect.String, nil

	case "process.ancestors.gid":

		return reflect.Int, nil

	case "process.ancestors.group":

		return reflect.String, nil

	case "process.ancestors.id":

		return reflect.String, nil

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

	case "process.cap_effective":

		return reflect.Int, nil

	case "process.cap_permitted":

		return reflect.Int, nil

	case "process.comm":

		return reflect.String, nil

	case "process.cookie":

		return reflect.Int, nil

	case "process.egid":

		return reflect.Int, nil

	case "process.egroup":

		return reflect.String, nil

	case "process.euid":

		return reflect.Int, nil

	case "process.euser":

		return reflect.String, nil

	case "process.file.container_path":

		return reflect.String, nil

	case "process.file.gid":

		return reflect.Int, nil

	case "process.file.group":

		return reflect.String, nil

	case "process.file.inode":

		return reflect.Int, nil

	case "process.file.mode":

		return reflect.Int, nil

	case "process.file.mount_id":

		return reflect.Int, nil

	case "process.file.name":

		return reflect.String, nil

	case "process.file.overlay_numlower":

		return reflect.Int, nil

	case "process.file.path":

		return reflect.String, nil

	case "process.file.uid":

		return reflect.Int, nil

	case "process.file.user":

		return reflect.String, nil

	case "process.fsgid":

		return reflect.Int, nil

	case "process.fsgroup":

		return reflect.String, nil

	case "process.fsuid":

		return reflect.Int, nil

	case "process.fsuser":

		return reflect.String, nil

	case "process.gid":

		return reflect.Int, nil

	case "process.group":

		return reflect.String, nil

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

	case "removexattr.file.container_path":

		return reflect.String, nil

	case "removexattr.file.destination.name":

		return reflect.String, nil

	case "removexattr.file.destination.namespace":

		return reflect.String, nil

	case "removexattr.file.gid":

		return reflect.Int, nil

	case "removexattr.file.group":

		return reflect.String, nil

	case "removexattr.file.inode":

		return reflect.Int, nil

	case "removexattr.file.mode":

		return reflect.Int, nil

	case "removexattr.file.mount_id":

		return reflect.Int, nil

	case "removexattr.file.name":

		return reflect.String, nil

	case "removexattr.file.overlay_numlower":

		return reflect.Int, nil

	case "removexattr.file.path":

		return reflect.String, nil

	case "removexattr.file.uid":

		return reflect.Int, nil

	case "removexattr.file.user":

		return reflect.String, nil

	case "removexattr.retval":

		return reflect.Int, nil

	case "rename.file.container_path":

		return reflect.String, nil

	case "rename.file.destination.container_path":

		return reflect.String, nil

	case "rename.file.destination.gid":

		return reflect.Int, nil

	case "rename.file.destination.group":

		return reflect.String, nil

	case "rename.file.destination.inode":

		return reflect.Int, nil

	case "rename.file.destination.mode":

		return reflect.Int, nil

	case "rename.file.destination.mount_id":

		return reflect.Int, nil

	case "rename.file.destination.name":

		return reflect.String, nil

	case "rename.file.destination.overlay_numlower":

		return reflect.Int, nil

	case "rename.file.destination.path":

		return reflect.String, nil

	case "rename.file.destination.uid":

		return reflect.Int, nil

	case "rename.file.destination.user":

		return reflect.String, nil

	case "rename.file.gid":

		return reflect.Int, nil

	case "rename.file.group":

		return reflect.String, nil

	case "rename.file.inode":

		return reflect.Int, nil

	case "rename.file.mode":

		return reflect.Int, nil

	case "rename.file.mount_id":

		return reflect.Int, nil

	case "rename.file.name":

		return reflect.String, nil

	case "rename.file.overlay_numlower":

		return reflect.Int, nil

	case "rename.file.path":

		return reflect.String, nil

	case "rename.file.uid":

		return reflect.Int, nil

	case "rename.file.user":

		return reflect.String, nil

	case "rename.retval":

		return reflect.Int, nil

	case "rmdir.file.container_path":

		return reflect.String, nil

	case "rmdir.file.gid":

		return reflect.Int, nil

	case "rmdir.file.group":

		return reflect.String, nil

	case "rmdir.file.inode":

		return reflect.Int, nil

	case "rmdir.file.mode":

		return reflect.Int, nil

	case "rmdir.file.mount_id":

		return reflect.Int, nil

	case "rmdir.file.name":

		return reflect.String, nil

	case "rmdir.file.overlay_numlower":

		return reflect.Int, nil

	case "rmdir.file.path":

		return reflect.String, nil

	case "rmdir.file.uid":

		return reflect.Int, nil

	case "rmdir.file.user":

		return reflect.String, nil

	case "rmdir.retval":

		return reflect.Int, nil

	case "setgid.egid":

		return reflect.Int, nil

	case "setgid.egroup":

		return reflect.String, nil

	case "setgid.fsgid":

		return reflect.Int, nil

	case "setgid.fsgroup":

		return reflect.String, nil

	case "setgid.gid":

		return reflect.Int, nil

	case "setgid.group":

		return reflect.String, nil

	case "setuid.euid":

		return reflect.Int, nil

	case "setuid.euser":

		return reflect.String, nil

	case "setuid.fsuid":

		return reflect.Int, nil

	case "setuid.fsuser":

		return reflect.String, nil

	case "setuid.uid":

		return reflect.Int, nil

	case "setuid.user":

		return reflect.String, nil

	case "setxattr.file.container_path":

		return reflect.String, nil

	case "setxattr.file.destination.name":

		return reflect.String, nil

	case "setxattr.file.destination.namespace":

		return reflect.String, nil

	case "setxattr.file.gid":

		return reflect.Int, nil

	case "setxattr.file.group":

		return reflect.String, nil

	case "setxattr.file.inode":

		return reflect.Int, nil

	case "setxattr.file.mode":

		return reflect.Int, nil

	case "setxattr.file.mount_id":

		return reflect.Int, nil

	case "setxattr.file.name":

		return reflect.String, nil

	case "setxattr.file.overlay_numlower":

		return reflect.Int, nil

	case "setxattr.file.path":

		return reflect.String, nil

	case "setxattr.file.uid":

		return reflect.Int, nil

	case "setxattr.file.user":

		return reflect.String, nil

	case "setxattr.retval":

		return reflect.Int, nil

	case "unlink.file.container_path":

		return reflect.String, nil

	case "unlink.file.gid":

		return reflect.Int, nil

	case "unlink.file.group":

		return reflect.String, nil

	case "unlink.file.inode":

		return reflect.Int, nil

	case "unlink.file.mode":

		return reflect.Int, nil

	case "unlink.file.mount_id":

		return reflect.Int, nil

	case "unlink.file.name":

		return reflect.String, nil

	case "unlink.file.overlay_numlower":

		return reflect.Int, nil

	case "unlink.file.path":

		return reflect.String, nil

	case "unlink.file.uid":

		return reflect.Int, nil

	case "unlink.file.user":

		return reflect.String, nil

	case "unlink.retval":

		return reflect.Int, nil

	case "utimes.file.container_path":

		return reflect.String, nil

	case "utimes.file.gid":

		return reflect.Int, nil

	case "utimes.file.group":

		return reflect.String, nil

	case "utimes.file.inode":

		return reflect.Int, nil

	case "utimes.file.mode":

		return reflect.Int, nil

	case "utimes.file.mount_id":

		return reflect.Int, nil

	case "utimes.file.name":

		return reflect.String, nil

	case "utimes.file.overlay_numlower":

		return reflect.Int, nil

	case "utimes.file.path":

		return reflect.String, nil

	case "utimes.file.uid":

		return reflect.Int, nil

	case "utimes.file.user":

		return reflect.String, nil

	case "utimes.retval":

		return reflect.Int, nil

	}

	return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) SetFieldValue(field eval.Field, value interface{}) error {
	switch field {

	case "capset.cap_effective":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Capset.CapEffective"}
		}
		e.Capset.CapEffective = uint64(v)
		return nil

	case "capset.cap_permitted":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Capset.CapPermitted"}
		}
		e.Capset.CapPermitted = uint64(v)
		return nil

	case "chmod.file.container_path":

		var ok bool
		if e.Chmod.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.ContainerPath"}
		}
		return nil

	case "chmod.file.destination.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Mode"}
		}
		e.Chmod.Mode = uint32(v)
		return nil

	case "chmod.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.GID"}
		}
		e.Chmod.File.FileFields.GID = uint32(v)
		return nil

	case "chmod.file.group":

		var ok bool
		if e.Chmod.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.Group"}
		}
		return nil

	case "chmod.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.Inode"}
		}
		e.Chmod.File.FileFields.Inode = uint64(v)
		return nil

	case "chmod.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.Mode"}
		}
		e.Chmod.File.FileFields.Mode = uint16(v)
		return nil

	case "chmod.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.MountID"}
		}
		e.Chmod.File.FileFields.MountID = uint32(v)
		return nil

	case "chmod.file.name":

		var ok bool
		if e.Chmod.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.BasenameStr"}
		}
		return nil

	case "chmod.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.OverlayNumLower"}
		}
		e.Chmod.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "chmod.file.path":

		var ok bool
		if e.Chmod.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.PathnameStr"}
		}
		return nil

	case "chmod.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.UID"}
		}
		e.Chmod.File.FileFields.UID = uint32(v)
		return nil

	case "chmod.file.user":

		var ok bool
		if e.Chmod.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.User"}
		}
		return nil

	case "chmod.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.SyscallEvent.Retval"}
		}
		e.Chmod.SyscallEvent.Retval = int64(v)
		return nil

	case "chown.file.container_path":

		var ok bool
		if e.Chown.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.ContainerPath"}
		}
		return nil

	case "chown.file.destination.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.GID"}
		}
		e.Chown.GID = uint32(v)
		return nil

	case "chown.file.destination.group":

		var ok bool
		if e.Chown.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.Group"}
		}
		return nil

	case "chown.file.destination.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.UID"}
		}
		e.Chown.UID = uint32(v)
		return nil

	case "chown.file.destination.user":

		var ok bool
		if e.Chown.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.User"}
		}
		return nil

	case "chown.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.GID"}
		}
		e.Chown.File.FileFields.GID = uint32(v)
		return nil

	case "chown.file.group":

		var ok bool
		if e.Chown.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.Group"}
		}
		return nil

	case "chown.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.Inode"}
		}
		e.Chown.File.FileFields.Inode = uint64(v)
		return nil

	case "chown.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.Mode"}
		}
		e.Chown.File.FileFields.Mode = uint16(v)
		return nil

	case "chown.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.MountID"}
		}
		e.Chown.File.FileFields.MountID = uint32(v)
		return nil

	case "chown.file.name":

		var ok bool
		if e.Chown.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.BasenameStr"}
		}
		return nil

	case "chown.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.OverlayNumLower"}
		}
		e.Chown.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "chown.file.path":

		var ok bool
		if e.Chown.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.PathnameStr"}
		}
		return nil

	case "chown.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.UID"}
		}
		e.Chown.File.FileFields.UID = uint32(v)
		return nil

	case "chown.file.user":

		var ok bool
		if e.Chown.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.User"}
		}
		return nil

	case "chown.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.SyscallEvent.Retval"}
		}
		e.Chown.SyscallEvent.Retval = int64(v)
		return nil

	case "container.id":

		var ok bool
		if e.Container.ID, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Container.ID"}
		}
		return nil

	case "exec.cap_effective":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.CapEffective"}
		}
		e.Exec.Credentials.CapEffective = uint64(v)
		return nil

	case "exec.cap_permitted":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.CapPermitted"}
		}
		e.Exec.Credentials.CapPermitted = uint64(v)
		return nil

	case "exec.comm":

		var ok bool
		if e.Exec.Comm, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Comm"}
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

	case "exec.egid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.EGID"}
		}
		e.Exec.Credentials.EGID = uint32(v)
		return nil

	case "exec.egroup":

		var ok bool
		if e.Exec.Credentials.EGroup, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.EGroup"}
		}
		return nil

	case "exec.euid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.EUID"}
		}
		e.Exec.Credentials.EUID = uint32(v)
		return nil

	case "exec.euser":

		var ok bool
		if e.Exec.Credentials.EUser, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.EUser"}
		}
		return nil

	case "exec.file.container_path":

		var ok bool
		if e.Exec.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.ContainerPath"}
		}
		return nil

	case "exec.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.GID"}
		}
		e.Exec.FileFields.GID = uint32(v)
		return nil

	case "exec.file.group":

		var ok bool
		if e.Exec.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.Group"}
		}
		return nil

	case "exec.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.Inode"}
		}
		e.Exec.FileFields.Inode = uint64(v)
		return nil

	case "exec.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.Mode"}
		}
		e.Exec.FileFields.Mode = uint16(v)
		return nil

	case "exec.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.MountID"}
		}
		e.Exec.FileFields.MountID = uint32(v)
		return nil

	case "exec.file.name":

		var ok bool
		if e.Exec.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.BasenameStr"}
		}
		return nil

	case "exec.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.OverlayNumLower"}
		}
		e.Exec.FileFields.OverlayNumLower = int32(v)
		return nil

	case "exec.file.path":

		var ok bool
		if e.Exec.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.PathnameStr"}
		}
		return nil

	case "exec.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.UID"}
		}
		e.Exec.FileFields.UID = uint32(v)
		return nil

	case "exec.file.user":

		var ok bool
		if e.Exec.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.FileFields.User"}
		}
		return nil

	case "exec.fsgid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.FSGID"}
		}
		e.Exec.Credentials.FSGID = uint32(v)
		return nil

	case "exec.fsgroup":

		var ok bool
		if e.Exec.Credentials.FSGroup, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.FSGroup"}
		}
		return nil

	case "exec.fsuid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.FSUID"}
		}
		e.Exec.Credentials.FSUID = uint32(v)
		return nil

	case "exec.fsuser":

		var ok bool
		if e.Exec.Credentials.FSUser, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.FSUser"}
		}
		return nil

	case "exec.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.GID"}
		}
		e.Exec.Credentials.GID = uint32(v)
		return nil

	case "exec.group":

		var ok bool
		if e.Exec.Credentials.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.Group"}
		}
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
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.UID"}
		}
		e.Exec.Credentials.UID = uint32(v)
		return nil

	case "exec.user":

		var ok bool
		if e.Exec.Credentials.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Credentials.User"}
		}
		return nil

	case "link.file.container_path":

		var ok bool
		if e.Link.Source.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.ContainerPath"}
		}
		return nil

	case "link.file.destination.container_path":

		var ok bool
		if e.Link.Target.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.ContainerPath"}
		}
		return nil

	case "link.file.destination.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.GID"}
		}
		e.Link.Target.FileFields.GID = uint32(v)
		return nil

	case "link.file.destination.group":

		var ok bool
		if e.Link.Target.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Group"}
		}
		return nil

	case "link.file.destination.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Inode"}
		}
		e.Link.Target.FileFields.Inode = uint64(v)
		return nil

	case "link.file.destination.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Mode"}
		}
		e.Link.Target.FileFields.Mode = uint16(v)
		return nil

	case "link.file.destination.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.MountID"}
		}
		e.Link.Target.FileFields.MountID = uint32(v)
		return nil

	case "link.file.destination.name":

		var ok bool
		if e.Link.Target.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.BasenameStr"}
		}
		return nil

	case "link.file.destination.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.OverlayNumLower"}
		}
		e.Link.Target.FileFields.OverlayNumLower = int32(v)
		return nil

	case "link.file.destination.path":

		var ok bool
		if e.Link.Target.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.PathnameStr"}
		}
		return nil

	case "link.file.destination.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.UID"}
		}
		e.Link.Target.FileFields.UID = uint32(v)
		return nil

	case "link.file.destination.user":

		var ok bool
		if e.Link.Target.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.User"}
		}
		return nil

	case "link.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.GID"}
		}
		e.Link.Source.FileFields.GID = uint32(v)
		return nil

	case "link.file.group":

		var ok bool
		if e.Link.Source.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Group"}
		}
		return nil

	case "link.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Inode"}
		}
		e.Link.Source.FileFields.Inode = uint64(v)
		return nil

	case "link.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Mode"}
		}
		e.Link.Source.FileFields.Mode = uint16(v)
		return nil

	case "link.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.MountID"}
		}
		e.Link.Source.FileFields.MountID = uint32(v)
		return nil

	case "link.file.name":

		var ok bool
		if e.Link.Source.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.BasenameStr"}
		}
		return nil

	case "link.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.OverlayNumLower"}
		}
		e.Link.Source.FileFields.OverlayNumLower = int32(v)
		return nil

	case "link.file.path":

		var ok bool
		if e.Link.Source.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.PathnameStr"}
		}
		return nil

	case "link.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.UID"}
		}
		e.Link.Source.FileFields.UID = uint32(v)
		return nil

	case "link.file.user":

		var ok bool
		if e.Link.Source.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.User"}
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

	case "mkdir.file.container_path":

		var ok bool
		if e.Mkdir.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.ContainerPath"}
		}
		return nil

	case "mkdir.file.destination.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Mode"}
		}
		e.Mkdir.Mode = uint32(v)
		return nil

	case "mkdir.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.GID"}
		}
		e.Mkdir.File.FileFields.GID = uint32(v)
		return nil

	case "mkdir.file.group":

		var ok bool
		if e.Mkdir.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.Group"}
		}
		return nil

	case "mkdir.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.Inode"}
		}
		e.Mkdir.File.FileFields.Inode = uint64(v)
		return nil

	case "mkdir.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.Mode"}
		}
		e.Mkdir.File.FileFields.Mode = uint16(v)
		return nil

	case "mkdir.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.MountID"}
		}
		e.Mkdir.File.FileFields.MountID = uint32(v)
		return nil

	case "mkdir.file.name":

		var ok bool
		if e.Mkdir.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.BasenameStr"}
		}
		return nil

	case "mkdir.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.OverlayNumLower"}
		}
		e.Mkdir.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "mkdir.file.path":

		var ok bool
		if e.Mkdir.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.PathnameStr"}
		}
		return nil

	case "mkdir.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.UID"}
		}
		e.Mkdir.File.FileFields.UID = uint32(v)
		return nil

	case "mkdir.file.user":

		var ok bool
		if e.Mkdir.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.User"}
		}
		return nil

	case "mkdir.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.SyscallEvent.Retval"}
		}
		e.Mkdir.SyscallEvent.Retval = int64(v)
		return nil

	case "open.file.container_path":

		var ok bool
		if e.Open.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.ContainerPath"}
		}
		return nil

	case "open.file.destination.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Mode"}
		}
		e.Open.Mode = uint32(v)
		return nil

	case "open.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.GID"}
		}
		e.Open.File.FileFields.GID = uint32(v)
		return nil

	case "open.file.group":

		var ok bool
		if e.Open.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.Group"}
		}
		return nil

	case "open.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.Inode"}
		}
		e.Open.File.FileFields.Inode = uint64(v)
		return nil

	case "open.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.Mode"}
		}
		e.Open.File.FileFields.Mode = uint16(v)
		return nil

	case "open.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.MountID"}
		}
		e.Open.File.FileFields.MountID = uint32(v)
		return nil

	case "open.file.name":

		var ok bool
		if e.Open.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.BasenameStr"}
		}
		return nil

	case "open.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.OverlayNumLower"}
		}
		e.Open.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "open.file.path":

		var ok bool
		if e.Open.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.PathnameStr"}
		}
		return nil

	case "open.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.UID"}
		}
		e.Open.File.FileFields.UID = uint32(v)
		return nil

	case "open.file.user":

		var ok bool
		if e.Open.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.User"}
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

	case "open.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.SyscallEvent.Retval"}
		}
		e.Open.SyscallEvent.Retval = int64(v)
		return nil

	case "process.ancestors.cap_effective":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.CapEffective"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.CapEffective = uint64(v)
		return nil

	case "process.ancestors.cap_permitted":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.CapPermitted"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.CapPermitted = uint64(v)
		return nil

	case "process.ancestors.comm":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.Comm, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Comm"}
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

	case "process.ancestors.egid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.EGID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.EGID = uint32(v)
		return nil

	case "process.ancestors.egroup":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.EGroup, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.EGroup"}
		}
		return nil

	case "process.ancestors.euid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.EUID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.EUID = uint32(v)
		return nil

	case "process.ancestors.euser":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.EUser, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.EUser"}
		}
		return nil

	case "process.ancestors.file.container_path":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.ContainerPath"}
		}
		return nil

	case "process.ancestors.file.gid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.FileFields.GID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.FileFields.GID = uint32(v)
		return nil

	case "process.ancestors.file.group":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.FileFields.Group"}
		}
		return nil

	case "process.ancestors.file.inode":

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

	case "process.ancestors.file.mode":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.FileFields.Mode"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.FileFields.Mode = uint16(v)
		return nil

	case "process.ancestors.file.mount_id":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.FileFields.MountID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.FileFields.MountID = uint32(v)
		return nil

	case "process.ancestors.file.name":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.BasenameStr"}
		}
		return nil

	case "process.ancestors.file.overlay_numlower":

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

	case "process.ancestors.file.path":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.PathnameStr"}
		}
		return nil

	case "process.ancestors.file.uid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.FileFields.UID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.FileFields.UID = uint32(v)
		return nil

	case "process.ancestors.file.user":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.FileFields.User"}
		}
		return nil

	case "process.ancestors.fsgid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.FSGID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.FSGID = uint32(v)
		return nil

	case "process.ancestors.fsgroup":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.FSGroup, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.FSGroup"}
		}
		return nil

	case "process.ancestors.fsuid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.FSUID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.FSUID = uint32(v)
		return nil

	case "process.ancestors.fsuser":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.FSUser, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.FSUser"}
		}
		return nil

	case "process.ancestors.gid":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.GID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.GID = uint32(v)
		return nil

	case "process.ancestors.group":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.Group"}
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
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.UID"}
		}
		e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.UID = uint32(v)
		return nil

	case "process.ancestors.user":

		if e.Process.Ancestor == nil {
			e.Process.Ancestor = &ProcessCacheEntry{}
		}

		var ok bool
		if e.Process.Ancestor.ProcessContext.ExecEvent.Credentials.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Ancestor.ProcessContext.ExecEvent.Credentials.User"}
		}
		return nil

	case "process.cap_effective":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.CapEffective"}
		}
		e.Process.ExecEvent.Credentials.CapEffective = uint64(v)
		return nil

	case "process.cap_permitted":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.CapPermitted"}
		}
		e.Process.ExecEvent.Credentials.CapPermitted = uint64(v)
		return nil

	case "process.comm":

		var ok bool
		if e.Process.ExecEvent.Comm, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Comm"}
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

	case "process.egid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.EGID"}
		}
		e.Process.ExecEvent.Credentials.EGID = uint32(v)
		return nil

	case "process.egroup":

		var ok bool
		if e.Process.ExecEvent.Credentials.EGroup, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.EGroup"}
		}
		return nil

	case "process.euid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.EUID"}
		}
		e.Process.ExecEvent.Credentials.EUID = uint32(v)
		return nil

	case "process.euser":

		var ok bool
		if e.Process.ExecEvent.Credentials.EUser, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.EUser"}
		}
		return nil

	case "process.file.container_path":

		var ok bool
		if e.Process.ExecEvent.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.ContainerPath"}
		}
		return nil

	case "process.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.GID"}
		}
		e.Process.ExecEvent.FileFields.GID = uint32(v)
		return nil

	case "process.file.group":

		var ok bool
		if e.Process.ExecEvent.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.Group"}
		}
		return nil

	case "process.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.Inode"}
		}
		e.Process.ExecEvent.FileFields.Inode = uint64(v)
		return nil

	case "process.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.Mode"}
		}
		e.Process.ExecEvent.FileFields.Mode = uint16(v)
		return nil

	case "process.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.MountID"}
		}
		e.Process.ExecEvent.FileFields.MountID = uint32(v)
		return nil

	case "process.file.name":

		var ok bool
		if e.Process.ExecEvent.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.BasenameStr"}
		}
		return nil

	case "process.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.OverlayNumLower"}
		}
		e.Process.ExecEvent.FileFields.OverlayNumLower = int32(v)
		return nil

	case "process.file.path":

		var ok bool
		if e.Process.ExecEvent.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.PathnameStr"}
		}
		return nil

	case "process.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.UID"}
		}
		e.Process.ExecEvent.FileFields.UID = uint32(v)
		return nil

	case "process.file.user":

		var ok bool
		if e.Process.ExecEvent.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.FileFields.User"}
		}
		return nil

	case "process.fsgid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.FSGID"}
		}
		e.Process.ExecEvent.Credentials.FSGID = uint32(v)
		return nil

	case "process.fsgroup":

		var ok bool
		if e.Process.ExecEvent.Credentials.FSGroup, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.FSGroup"}
		}
		return nil

	case "process.fsuid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.FSUID"}
		}
		e.Process.ExecEvent.Credentials.FSUID = uint32(v)
		return nil

	case "process.fsuser":

		var ok bool
		if e.Process.ExecEvent.Credentials.FSUser, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.FSUser"}
		}
		return nil

	case "process.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.GID"}
		}
		e.Process.ExecEvent.Credentials.GID = uint32(v)
		return nil

	case "process.group":

		var ok bool
		if e.Process.ExecEvent.Credentials.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.Group"}
		}
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
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.UID"}
		}
		e.Process.ExecEvent.Credentials.UID = uint32(v)
		return nil

	case "process.user":

		var ok bool
		if e.Process.ExecEvent.Credentials.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ExecEvent.Credentials.User"}
		}
		return nil

	case "removexattr.file.container_path":

		var ok bool
		if e.RemoveXAttr.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.ContainerPath"}
		}
		return nil

	case "removexattr.file.destination.name":

		var ok bool
		if e.RemoveXAttr.Name, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Name"}
		}
		return nil

	case "removexattr.file.destination.namespace":

		var ok bool
		if e.RemoveXAttr.Namespace, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Namespace"}
		}
		return nil

	case "removexattr.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.GID"}
		}
		e.RemoveXAttr.File.FileFields.GID = uint32(v)
		return nil

	case "removexattr.file.group":

		var ok bool
		if e.RemoveXAttr.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.Group"}
		}
		return nil

	case "removexattr.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.Inode"}
		}
		e.RemoveXAttr.File.FileFields.Inode = uint64(v)
		return nil

	case "removexattr.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.Mode"}
		}
		e.RemoveXAttr.File.FileFields.Mode = uint16(v)
		return nil

	case "removexattr.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.MountID"}
		}
		e.RemoveXAttr.File.FileFields.MountID = uint32(v)
		return nil

	case "removexattr.file.name":

		var ok bool
		if e.RemoveXAttr.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.BasenameStr"}
		}
		return nil

	case "removexattr.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.OverlayNumLower"}
		}
		e.RemoveXAttr.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "removexattr.file.path":

		var ok bool
		if e.RemoveXAttr.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.PathnameStr"}
		}
		return nil

	case "removexattr.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.UID"}
		}
		e.RemoveXAttr.File.FileFields.UID = uint32(v)
		return nil

	case "removexattr.file.user":

		var ok bool
		if e.RemoveXAttr.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.User"}
		}
		return nil

	case "removexattr.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.SyscallEvent.Retval"}
		}
		e.RemoveXAttr.SyscallEvent.Retval = int64(v)
		return nil

	case "rename.file.container_path":

		var ok bool
		if e.Rename.Old.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.ContainerPath"}
		}
		return nil

	case "rename.file.destination.container_path":

		var ok bool
		if e.Rename.New.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.ContainerPath"}
		}
		return nil

	case "rename.file.destination.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.GID"}
		}
		e.Rename.New.FileFields.GID = uint32(v)
		return nil

	case "rename.file.destination.group":

		var ok bool
		if e.Rename.New.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Group"}
		}
		return nil

	case "rename.file.destination.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Inode"}
		}
		e.Rename.New.FileFields.Inode = uint64(v)
		return nil

	case "rename.file.destination.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Mode"}
		}
		e.Rename.New.FileFields.Mode = uint16(v)
		return nil

	case "rename.file.destination.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.MountID"}
		}
		e.Rename.New.FileFields.MountID = uint32(v)
		return nil

	case "rename.file.destination.name":

		var ok bool
		if e.Rename.New.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.BasenameStr"}
		}
		return nil

	case "rename.file.destination.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.OverlayNumLower"}
		}
		e.Rename.New.FileFields.OverlayNumLower = int32(v)
		return nil

	case "rename.file.destination.path":

		var ok bool
		if e.Rename.New.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.PathnameStr"}
		}
		return nil

	case "rename.file.destination.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.UID"}
		}
		e.Rename.New.FileFields.UID = uint32(v)
		return nil

	case "rename.file.destination.user":

		var ok bool
		if e.Rename.New.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.User"}
		}
		return nil

	case "rename.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.GID"}
		}
		e.Rename.Old.FileFields.GID = uint32(v)
		return nil

	case "rename.file.group":

		var ok bool
		if e.Rename.Old.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Group"}
		}
		return nil

	case "rename.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Inode"}
		}
		e.Rename.Old.FileFields.Inode = uint64(v)
		return nil

	case "rename.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Mode"}
		}
		e.Rename.Old.FileFields.Mode = uint16(v)
		return nil

	case "rename.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.MountID"}
		}
		e.Rename.Old.FileFields.MountID = uint32(v)
		return nil

	case "rename.file.name":

		var ok bool
		if e.Rename.Old.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.BasenameStr"}
		}
		return nil

	case "rename.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.OverlayNumLower"}
		}
		e.Rename.Old.FileFields.OverlayNumLower = int32(v)
		return nil

	case "rename.file.path":

		var ok bool
		if e.Rename.Old.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.PathnameStr"}
		}
		return nil

	case "rename.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.UID"}
		}
		e.Rename.Old.FileFields.UID = uint32(v)
		return nil

	case "rename.file.user":

		var ok bool
		if e.Rename.Old.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.User"}
		}
		return nil

	case "rename.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.SyscallEvent.Retval"}
		}
		e.Rename.SyscallEvent.Retval = int64(v)
		return nil

	case "rmdir.file.container_path":

		var ok bool
		if e.Rmdir.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.ContainerPath"}
		}
		return nil

	case "rmdir.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.GID"}
		}
		e.Rmdir.File.FileFields.GID = uint32(v)
		return nil

	case "rmdir.file.group":

		var ok bool
		if e.Rmdir.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.Group"}
		}
		return nil

	case "rmdir.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.Inode"}
		}
		e.Rmdir.File.FileFields.Inode = uint64(v)
		return nil

	case "rmdir.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.Mode"}
		}
		e.Rmdir.File.FileFields.Mode = uint16(v)
		return nil

	case "rmdir.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.MountID"}
		}
		e.Rmdir.File.FileFields.MountID = uint32(v)
		return nil

	case "rmdir.file.name":

		var ok bool
		if e.Rmdir.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.BasenameStr"}
		}
		return nil

	case "rmdir.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.OverlayNumLower"}
		}
		e.Rmdir.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "rmdir.file.path":

		var ok bool
		if e.Rmdir.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.PathnameStr"}
		}
		return nil

	case "rmdir.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.UID"}
		}
		e.Rmdir.File.FileFields.UID = uint32(v)
		return nil

	case "rmdir.file.user":

		var ok bool
		if e.Rmdir.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.User"}
		}
		return nil

	case "rmdir.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.SyscallEvent.Retval"}
		}
		e.Rmdir.SyscallEvent.Retval = int64(v)
		return nil

	case "setgid.egid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.EGID"}
		}
		e.SetGID.EGID = uint32(v)
		return nil

	case "setgid.egroup":

		var ok bool
		if e.SetGID.EGroup, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.EGroup"}
		}
		return nil

	case "setgid.fsgid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.FSGID"}
		}
		e.SetGID.FSGID = uint32(v)
		return nil

	case "setgid.fsgroup":

		var ok bool
		if e.SetGID.FSGroup, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.FSGroup"}
		}
		return nil

	case "setgid.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.GID"}
		}
		e.SetGID.GID = uint32(v)
		return nil

	case "setgid.group":

		var ok bool
		if e.SetGID.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.Group"}
		}
		return nil

	case "setuid.euid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.EUID"}
		}
		e.SetUID.EUID = uint32(v)
		return nil

	case "setuid.euser":

		var ok bool
		if e.SetUID.EUser, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.EUser"}
		}
		return nil

	case "setuid.fsuid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.FSUID"}
		}
		e.SetUID.FSUID = uint32(v)
		return nil

	case "setuid.fsuser":

		var ok bool
		if e.SetUID.FSUser, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.FSUser"}
		}
		return nil

	case "setuid.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.UID"}
		}
		e.SetUID.UID = uint32(v)
		return nil

	case "setuid.user":

		var ok bool
		if e.SetUID.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.User"}
		}
		return nil

	case "setxattr.file.container_path":

		var ok bool
		if e.SetXAttr.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.ContainerPath"}
		}
		return nil

	case "setxattr.file.destination.name":

		var ok bool
		if e.SetXAttr.Name, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Name"}
		}
		return nil

	case "setxattr.file.destination.namespace":

		var ok bool
		if e.SetXAttr.Namespace, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Namespace"}
		}
		return nil

	case "setxattr.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.GID"}
		}
		e.SetXAttr.File.FileFields.GID = uint32(v)
		return nil

	case "setxattr.file.group":

		var ok bool
		if e.SetXAttr.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.Group"}
		}
		return nil

	case "setxattr.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.Inode"}
		}
		e.SetXAttr.File.FileFields.Inode = uint64(v)
		return nil

	case "setxattr.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.Mode"}
		}
		e.SetXAttr.File.FileFields.Mode = uint16(v)
		return nil

	case "setxattr.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.MountID"}
		}
		e.SetXAttr.File.FileFields.MountID = uint32(v)
		return nil

	case "setxattr.file.name":

		var ok bool
		if e.SetXAttr.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.BasenameStr"}
		}
		return nil

	case "setxattr.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.OverlayNumLower"}
		}
		e.SetXAttr.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "setxattr.file.path":

		var ok bool
		if e.SetXAttr.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.PathnameStr"}
		}
		return nil

	case "setxattr.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.UID"}
		}
		e.SetXAttr.File.FileFields.UID = uint32(v)
		return nil

	case "setxattr.file.user":

		var ok bool
		if e.SetXAttr.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.User"}
		}
		return nil

	case "setxattr.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.SyscallEvent.Retval"}
		}
		e.SetXAttr.SyscallEvent.Retval = int64(v)
		return nil

	case "unlink.file.container_path":

		var ok bool
		if e.Unlink.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.ContainerPath"}
		}
		return nil

	case "unlink.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.GID"}
		}
		e.Unlink.File.FileFields.GID = uint32(v)
		return nil

	case "unlink.file.group":

		var ok bool
		if e.Unlink.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.Group"}
		}
		return nil

	case "unlink.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.Inode"}
		}
		e.Unlink.File.FileFields.Inode = uint64(v)
		return nil

	case "unlink.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.Mode"}
		}
		e.Unlink.File.FileFields.Mode = uint16(v)
		return nil

	case "unlink.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.MountID"}
		}
		e.Unlink.File.FileFields.MountID = uint32(v)
		return nil

	case "unlink.file.name":

		var ok bool
		if e.Unlink.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.BasenameStr"}
		}
		return nil

	case "unlink.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.OverlayNumLower"}
		}
		e.Unlink.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "unlink.file.path":

		var ok bool
		if e.Unlink.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.PathnameStr"}
		}
		return nil

	case "unlink.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.UID"}
		}
		e.Unlink.File.FileFields.UID = uint32(v)
		return nil

	case "unlink.file.user":

		var ok bool
		if e.Unlink.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.User"}
		}
		return nil

	case "unlink.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.SyscallEvent.Retval"}
		}
		e.Unlink.SyscallEvent.Retval = int64(v)
		return nil

	case "utimes.file.container_path":

		var ok bool
		if e.Utimes.File.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.ContainerPath"}
		}
		return nil

	case "utimes.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.GID"}
		}
		e.Utimes.File.FileFields.GID = uint32(v)
		return nil

	case "utimes.file.group":

		var ok bool
		if e.Utimes.File.FileFields.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.Group"}
		}
		return nil

	case "utimes.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.Inode"}
		}
		e.Utimes.File.FileFields.Inode = uint64(v)
		return nil

	case "utimes.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.Mode"}
		}
		e.Utimes.File.FileFields.Mode = uint16(v)
		return nil

	case "utimes.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.MountID"}
		}
		e.Utimes.File.FileFields.MountID = uint32(v)
		return nil

	case "utimes.file.name":

		var ok bool
		if e.Utimes.File.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.BasenameStr"}
		}
		return nil

	case "utimes.file.overlay_numlower":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.OverlayNumLower"}
		}
		e.Utimes.File.FileFields.OverlayNumLower = int32(v)
		return nil

	case "utimes.file.path":

		var ok bool
		if e.Utimes.File.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.PathnameStr"}
		}
		return nil

	case "utimes.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.UID"}
		}
		e.Utimes.File.FileFields.UID = uint32(v)
		return nil

	case "utimes.file.user":

		var ok bool
		if e.Utimes.File.FileFields.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.User"}
		}
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
