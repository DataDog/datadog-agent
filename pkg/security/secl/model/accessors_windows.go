//go:build windows && npm
// +build windows,npm

// Code generated - DO NOT EDIT.
package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"reflect"
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
		eval.EventType("exec"),
		eval.EventType("exit"),
	}
}
func (m *Model) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {
	case "async":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.Async
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exec.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exec.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exec.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exec.Process.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exec.Process.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return false
				}
				return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exec.Process.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exec.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exec.Process.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exec.Process.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.FileEvent.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exec.Process.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return false
				}
				return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exec.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exit.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exit.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exit.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.cause":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Cause)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.code":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Code)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exit.Process.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exit.Process.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return false
				}
				return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exit.Process.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exit.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exit.Process.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exit.Process.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.FileEvent.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return 0
				}
				return int(ev.Exit.Process.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return false
				}
				return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return 0
				}
				return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.Exit.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.ancestors.args":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.args_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.BoolCache[field]; ok {
					return result
				}
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.BoolCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.argv0":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.cap_effective":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.CapEffective)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.cap_permitted":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.CapPermitted)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.comm":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Comm
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.ContainerID
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.cookie":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Cookie)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.egid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.EGID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.egroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.EGroup
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.envs_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.BoolCache[field]; ok {
					return result
				}
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.BoolCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.euid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.EUID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.euser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.EUser
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.BoolCache[field]; ok {
					return result
				}
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, false)
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.BoolCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Inode)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.FileEvent.FileFields.MountID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.IsNotKworker() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.fsgid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.FSGID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.fsgroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.FSGroup
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.fsuid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.FSUID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.fsuser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.FSUser
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.Group
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.BoolCache[field]; ok {
					return result
				}
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, false)
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.BoolCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, 0)
						value = iterator.Next()
						continue
					}
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					if !element.ProcessContext.Process.HasInterpreter() {
						results = append(results, "")
						value = iterator.Next()
						continue
					}
					result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.is_kworker":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if result, ok := ctx.BoolCache[field]; ok {
					return result
				}
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.PIDContext.IsKworker
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.BoolCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.is_thread":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if result, ok := ctx.BoolCache[field]; ok {
					return result
				}
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.IsThread
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.BoolCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PIDContext.Pid)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PPid)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.tid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PIDContext.Tid)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.tty_name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.TTYName
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.User
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgv(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.ProcessContext.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Process.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Process.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return false
				}
				return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Process.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Process.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Process.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveRights(ev, &ev.ProcessContext.Process.FileEvent.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Process.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Process.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return false
				}
				return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveRights(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.Process.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveProcessArgs(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.parent.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return false
				}
				return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessArgv(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.parent.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.parent.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.ProcessContext.Parent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return false
				}
				return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.ProcessContext.Parent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Parent.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Parent.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Parent.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Parent.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return false
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return false
				}
				return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Parent.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Parent.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Parent.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Parent.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Parent.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveRights(ev, &ev.ProcessContext.Parent.FileEvent.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return 0
				}
				return int(ev.ProcessContext.Parent.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.IsNotKworker() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Parent.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return false
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return false
				}
				return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveRights(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return 0
				}
				return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				if !ev.ProcessContext.Parent.HasInterpreter() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return false
				}
				return ev.ProcessContext.Parent.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return false
				}
				return ev.ProcessContext.Parent.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.ProcessContext.Parent.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.ProcessContext.HasParent() {
					return ""
				}
				return ev.ProcessContext.Parent.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.ProcessContext.Process.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ProcessContext.Process.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	}
	return nil, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFields() []eval.Field {
	return []eval.Field{
		"async",
		"exec.args",
		"exec.args_flags",
		"exec.args_options",
		"exec.args_truncated",
		"exec.argv",
		"exec.argv0",
		"exec.cap_effective",
		"exec.cap_permitted",
		"exec.comm",
		"exec.container.id",
		"exec.cookie",
		"exec.created_at",
		"exec.egid",
		"exec.egroup",
		"exec.envp",
		"exec.envs",
		"exec.envs_truncated",
		"exec.euid",
		"exec.euser",
		"exec.file.change_time",
		"exec.file.filesystem",
		"exec.file.gid",
		"exec.file.group",
		"exec.file.in_upper_layer",
		"exec.file.inode",
		"exec.file.mode",
		"exec.file.modification_time",
		"exec.file.mount_id",
		"exec.file.name",
		"exec.file.name.length",
		"exec.file.path",
		"exec.file.path.length",
		"exec.file.rights",
		"exec.file.uid",
		"exec.file.user",
		"exec.fsgid",
		"exec.fsgroup",
		"exec.fsuid",
		"exec.fsuser",
		"exec.gid",
		"exec.group",
		"exec.interpreter.file.change_time",
		"exec.interpreter.file.filesystem",
		"exec.interpreter.file.gid",
		"exec.interpreter.file.group",
		"exec.interpreter.file.in_upper_layer",
		"exec.interpreter.file.inode",
		"exec.interpreter.file.mode",
		"exec.interpreter.file.modification_time",
		"exec.interpreter.file.mount_id",
		"exec.interpreter.file.name",
		"exec.interpreter.file.name.length",
		"exec.interpreter.file.path",
		"exec.interpreter.file.path.length",
		"exec.interpreter.file.rights",
		"exec.interpreter.file.uid",
		"exec.interpreter.file.user",
		"exec.is_kworker",
		"exec.is_thread",
		"exec.pid",
		"exec.ppid",
		"exec.tid",
		"exec.tty_name",
		"exec.uid",
		"exec.user",
		"exit.args",
		"exit.args_flags",
		"exit.args_options",
		"exit.args_truncated",
		"exit.argv",
		"exit.argv0",
		"exit.cap_effective",
		"exit.cap_permitted",
		"exit.cause",
		"exit.code",
		"exit.comm",
		"exit.container.id",
		"exit.cookie",
		"exit.created_at",
		"exit.egid",
		"exit.egroup",
		"exit.envp",
		"exit.envs",
		"exit.envs_truncated",
		"exit.euid",
		"exit.euser",
		"exit.file.change_time",
		"exit.file.filesystem",
		"exit.file.gid",
		"exit.file.group",
		"exit.file.in_upper_layer",
		"exit.file.inode",
		"exit.file.mode",
		"exit.file.modification_time",
		"exit.file.mount_id",
		"exit.file.name",
		"exit.file.name.length",
		"exit.file.path",
		"exit.file.path.length",
		"exit.file.rights",
		"exit.file.uid",
		"exit.file.user",
		"exit.fsgid",
		"exit.fsgroup",
		"exit.fsuid",
		"exit.fsuser",
		"exit.gid",
		"exit.group",
		"exit.interpreter.file.change_time",
		"exit.interpreter.file.filesystem",
		"exit.interpreter.file.gid",
		"exit.interpreter.file.group",
		"exit.interpreter.file.in_upper_layer",
		"exit.interpreter.file.inode",
		"exit.interpreter.file.mode",
		"exit.interpreter.file.modification_time",
		"exit.interpreter.file.mount_id",
		"exit.interpreter.file.name",
		"exit.interpreter.file.name.length",
		"exit.interpreter.file.path",
		"exit.interpreter.file.path.length",
		"exit.interpreter.file.rights",
		"exit.interpreter.file.uid",
		"exit.interpreter.file.user",
		"exit.is_kworker",
		"exit.is_thread",
		"exit.pid",
		"exit.ppid",
		"exit.tid",
		"exit.tty_name",
		"exit.uid",
		"exit.user",
		"process.ancestors.args",
		"process.ancestors.args_flags",
		"process.ancestors.args_options",
		"process.ancestors.args_truncated",
		"process.ancestors.argv",
		"process.ancestors.argv0",
		"process.ancestors.cap_effective",
		"process.ancestors.cap_permitted",
		"process.ancestors.comm",
		"process.ancestors.container.id",
		"process.ancestors.cookie",
		"process.ancestors.created_at",
		"process.ancestors.egid",
		"process.ancestors.egroup",
		"process.ancestors.envp",
		"process.ancestors.envs",
		"process.ancestors.envs_truncated",
		"process.ancestors.euid",
		"process.ancestors.euser",
		"process.ancestors.file.change_time",
		"process.ancestors.file.filesystem",
		"process.ancestors.file.gid",
		"process.ancestors.file.group",
		"process.ancestors.file.in_upper_layer",
		"process.ancestors.file.inode",
		"process.ancestors.file.mode",
		"process.ancestors.file.modification_time",
		"process.ancestors.file.mount_id",
		"process.ancestors.file.name",
		"process.ancestors.file.name.length",
		"process.ancestors.file.path",
		"process.ancestors.file.path.length",
		"process.ancestors.file.rights",
		"process.ancestors.file.uid",
		"process.ancestors.file.user",
		"process.ancestors.fsgid",
		"process.ancestors.fsgroup",
		"process.ancestors.fsuid",
		"process.ancestors.fsuser",
		"process.ancestors.gid",
		"process.ancestors.group",
		"process.ancestors.interpreter.file.change_time",
		"process.ancestors.interpreter.file.filesystem",
		"process.ancestors.interpreter.file.gid",
		"process.ancestors.interpreter.file.group",
		"process.ancestors.interpreter.file.in_upper_layer",
		"process.ancestors.interpreter.file.inode",
		"process.ancestors.interpreter.file.mode",
		"process.ancestors.interpreter.file.modification_time",
		"process.ancestors.interpreter.file.mount_id",
		"process.ancestors.interpreter.file.name",
		"process.ancestors.interpreter.file.name.length",
		"process.ancestors.interpreter.file.path",
		"process.ancestors.interpreter.file.path.length",
		"process.ancestors.interpreter.file.rights",
		"process.ancestors.interpreter.file.uid",
		"process.ancestors.interpreter.file.user",
		"process.ancestors.is_kworker",
		"process.ancestors.is_thread",
		"process.ancestors.pid",
		"process.ancestors.ppid",
		"process.ancestors.tid",
		"process.ancestors.tty_name",
		"process.ancestors.uid",
		"process.ancestors.user",
		"process.args",
		"process.args_flags",
		"process.args_options",
		"process.args_truncated",
		"process.argv",
		"process.argv0",
		"process.cap_effective",
		"process.cap_permitted",
		"process.comm",
		"process.container.id",
		"process.cookie",
		"process.created_at",
		"process.egid",
		"process.egroup",
		"process.envp",
		"process.envs",
		"process.envs_truncated",
		"process.euid",
		"process.euser",
		"process.file.change_time",
		"process.file.filesystem",
		"process.file.gid",
		"process.file.group",
		"process.file.in_upper_layer",
		"process.file.inode",
		"process.file.mode",
		"process.file.modification_time",
		"process.file.mount_id",
		"process.file.name",
		"process.file.name.length",
		"process.file.path",
		"process.file.path.length",
		"process.file.rights",
		"process.file.uid",
		"process.file.user",
		"process.fsgid",
		"process.fsgroup",
		"process.fsuid",
		"process.fsuser",
		"process.gid",
		"process.group",
		"process.interpreter.file.change_time",
		"process.interpreter.file.filesystem",
		"process.interpreter.file.gid",
		"process.interpreter.file.group",
		"process.interpreter.file.in_upper_layer",
		"process.interpreter.file.inode",
		"process.interpreter.file.mode",
		"process.interpreter.file.modification_time",
		"process.interpreter.file.mount_id",
		"process.interpreter.file.name",
		"process.interpreter.file.name.length",
		"process.interpreter.file.path",
		"process.interpreter.file.path.length",
		"process.interpreter.file.rights",
		"process.interpreter.file.uid",
		"process.interpreter.file.user",
		"process.is_kworker",
		"process.is_thread",
		"process.parent.args",
		"process.parent.args_flags",
		"process.parent.args_options",
		"process.parent.args_truncated",
		"process.parent.argv",
		"process.parent.argv0",
		"process.parent.cap_effective",
		"process.parent.cap_permitted",
		"process.parent.comm",
		"process.parent.container.id",
		"process.parent.cookie",
		"process.parent.created_at",
		"process.parent.egid",
		"process.parent.egroup",
		"process.parent.envp",
		"process.parent.envs",
		"process.parent.envs_truncated",
		"process.parent.euid",
		"process.parent.euser",
		"process.parent.file.change_time",
		"process.parent.file.filesystem",
		"process.parent.file.gid",
		"process.parent.file.group",
		"process.parent.file.in_upper_layer",
		"process.parent.file.inode",
		"process.parent.file.mode",
		"process.parent.file.modification_time",
		"process.parent.file.mount_id",
		"process.parent.file.name",
		"process.parent.file.name.length",
		"process.parent.file.path",
		"process.parent.file.path.length",
		"process.parent.file.rights",
		"process.parent.file.uid",
		"process.parent.file.user",
		"process.parent.fsgid",
		"process.parent.fsgroup",
		"process.parent.fsuid",
		"process.parent.fsuser",
		"process.parent.gid",
		"process.parent.group",
		"process.parent.interpreter.file.change_time",
		"process.parent.interpreter.file.filesystem",
		"process.parent.interpreter.file.gid",
		"process.parent.interpreter.file.group",
		"process.parent.interpreter.file.in_upper_layer",
		"process.parent.interpreter.file.inode",
		"process.parent.interpreter.file.mode",
		"process.parent.interpreter.file.modification_time",
		"process.parent.interpreter.file.mount_id",
		"process.parent.interpreter.file.name",
		"process.parent.interpreter.file.name.length",
		"process.parent.interpreter.file.path",
		"process.parent.interpreter.file.path.length",
		"process.parent.interpreter.file.rights",
		"process.parent.interpreter.file.uid",
		"process.parent.interpreter.file.user",
		"process.parent.is_kworker",
		"process.parent.is_thread",
		"process.parent.pid",
		"process.parent.ppid",
		"process.parent.tid",
		"process.parent.tty_name",
		"process.parent.uid",
		"process.parent.user",
		"process.pid",
		"process.ppid",
		"process.tid",
		"process.tty_name",
		"process.uid",
		"process.user",
	}
}
func (ev *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
	case "async":
		return ev.Async, nil
	case "exec.args":
		return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process), nil
	case "exec.args_flags":
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exec.Process), nil
	case "exec.args_options":
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exec.Process), nil
	case "exec.args_truncated":
		return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process), nil
	case "exec.argv":
		return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process), nil
	case "exec.argv0":
		return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process), nil
	case "exec.cap_effective":
		return int(ev.Exec.Process.Credentials.CapEffective), nil
	case "exec.cap_permitted":
		return int(ev.Exec.Process.Credentials.CapPermitted), nil
	case "exec.comm":
		return ev.Exec.Process.Comm, nil
	case "exec.container.id":
		return ev.Exec.Process.ContainerID, nil
	case "exec.cookie":
		return int(ev.Exec.Process.Cookie), nil
	case "exec.created_at":
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)), nil
	case "exec.egid":
		return int(ev.Exec.Process.Credentials.EGID), nil
	case "exec.egroup":
		return ev.Exec.Process.Credentials.EGroup, nil
	case "exec.envp":
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process), nil
	case "exec.envs":
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process), nil
	case "exec.envs_truncated":
		return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process), nil
	case "exec.euid":
		return int(ev.Exec.Process.Credentials.EUID), nil
	case "exec.euser":
		return ev.Exec.Process.Credentials.EUser, nil
	case "exec.file.change_time":
		return int(ev.Exec.Process.FileEvent.FileFields.CTime), nil
	case "exec.file.filesystem":
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.file.gid":
		return int(ev.Exec.Process.FileEvent.FileFields.GID), nil
	case "exec.file.group":
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields), nil
	case "exec.file.in_upper_layer":
		return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.FileEvent.FileFields), nil
	case "exec.file.inode":
		return int(ev.Exec.Process.FileEvent.FileFields.Inode), nil
	case "exec.file.mode":
		return int(ev.Exec.Process.FileEvent.FileFields.Mode), nil
	case "exec.file.modification_time":
		return int(ev.Exec.Process.FileEvent.FileFields.MTime), nil
	case "exec.file.mount_id":
		return int(ev.Exec.Process.FileEvent.FileFields.MountID), nil
	case "exec.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.file.rights":
		return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.FileEvent.FileFields)), nil
	case "exec.file.uid":
		return int(ev.Exec.Process.FileEvent.FileFields.UID), nil
	case "exec.file.user":
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields), nil
	case "exec.fsgid":
		return int(ev.Exec.Process.Credentials.FSGID), nil
	case "exec.fsgroup":
		return ev.Exec.Process.Credentials.FSGroup, nil
	case "exec.fsuid":
		return int(ev.Exec.Process.Credentials.FSUID), nil
	case "exec.fsuser":
		return ev.Exec.Process.Credentials.FSUser, nil
	case "exec.gid":
		return int(ev.Exec.Process.Credentials.GID), nil
	case "exec.group":
		return ev.Exec.Process.Credentials.Group, nil
	case "exec.interpreter.file.change_time":
		return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "exec.interpreter.file.filesystem":
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent), nil
	case "exec.interpreter.file.gid":
		return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "exec.interpreter.file.group":
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "exec.interpreter.file.in_upper_layer":
		return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "exec.interpreter.file.inode":
		return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "exec.interpreter.file.mode":
		return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "exec.interpreter.file.modification_time":
		return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "exec.interpreter.file.mount_id":
		return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "exec.interpreter.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent), nil
	case "exec.interpreter.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent), nil
	case "exec.interpreter.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent), nil
	case "exec.interpreter.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent), nil
	case "exec.interpreter.file.rights":
		return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)), nil
	case "exec.interpreter.file.uid":
		return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "exec.interpreter.file.user":
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "exec.is_kworker":
		return ev.Exec.Process.PIDContext.IsKworker, nil
	case "exec.is_thread":
		return ev.Exec.Process.IsThread, nil
	case "exec.pid":
		return int(ev.Exec.Process.PIDContext.Pid), nil
	case "exec.ppid":
		return int(ev.Exec.Process.PPid), nil
	case "exec.tid":
		return int(ev.Exec.Process.PIDContext.Tid), nil
	case "exec.tty_name":
		return ev.Exec.Process.TTYName, nil
	case "exec.uid":
		return int(ev.Exec.Process.Credentials.UID), nil
	case "exec.user":
		return ev.Exec.Process.Credentials.User, nil
	case "exit.args":
		return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process), nil
	case "exit.args_flags":
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exit.Process), nil
	case "exit.args_options":
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exit.Process), nil
	case "exit.args_truncated":
		return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process), nil
	case "exit.argv":
		return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process), nil
	case "exit.argv0":
		return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process), nil
	case "exit.cap_effective":
		return int(ev.Exit.Process.Credentials.CapEffective), nil
	case "exit.cap_permitted":
		return int(ev.Exit.Process.Credentials.CapPermitted), nil
	case "exit.cause":
		return int(ev.Exit.Cause), nil
	case "exit.code":
		return int(ev.Exit.Code), nil
	case "exit.comm":
		return ev.Exit.Process.Comm, nil
	case "exit.container.id":
		return ev.Exit.Process.ContainerID, nil
	case "exit.cookie":
		return int(ev.Exit.Process.Cookie), nil
	case "exit.created_at":
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)), nil
	case "exit.egid":
		return int(ev.Exit.Process.Credentials.EGID), nil
	case "exit.egroup":
		return ev.Exit.Process.Credentials.EGroup, nil
	case "exit.envp":
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process), nil
	case "exit.envs":
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process), nil
	case "exit.envs_truncated":
		return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process), nil
	case "exit.euid":
		return int(ev.Exit.Process.Credentials.EUID), nil
	case "exit.euser":
		return ev.Exit.Process.Credentials.EUser, nil
	case "exit.file.change_time":
		return int(ev.Exit.Process.FileEvent.FileFields.CTime), nil
	case "exit.file.filesystem":
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.file.gid":
		return int(ev.Exit.Process.FileEvent.FileFields.GID), nil
	case "exit.file.group":
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields), nil
	case "exit.file.in_upper_layer":
		return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.FileEvent.FileFields), nil
	case "exit.file.inode":
		return int(ev.Exit.Process.FileEvent.FileFields.Inode), nil
	case "exit.file.mode":
		return int(ev.Exit.Process.FileEvent.FileFields.Mode), nil
	case "exit.file.modification_time":
		return int(ev.Exit.Process.FileEvent.FileFields.MTime), nil
	case "exit.file.mount_id":
		return int(ev.Exit.Process.FileEvent.FileFields.MountID), nil
	case "exit.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.file.rights":
		return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.FileEvent.FileFields)), nil
	case "exit.file.uid":
		return int(ev.Exit.Process.FileEvent.FileFields.UID), nil
	case "exit.file.user":
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields), nil
	case "exit.fsgid":
		return int(ev.Exit.Process.Credentials.FSGID), nil
	case "exit.fsgroup":
		return ev.Exit.Process.Credentials.FSGroup, nil
	case "exit.fsuid":
		return int(ev.Exit.Process.Credentials.FSUID), nil
	case "exit.fsuser":
		return ev.Exit.Process.Credentials.FSUser, nil
	case "exit.gid":
		return int(ev.Exit.Process.Credentials.GID), nil
	case "exit.group":
		return ev.Exit.Process.Credentials.Group, nil
	case "exit.interpreter.file.change_time":
		return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "exit.interpreter.file.filesystem":
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent), nil
	case "exit.interpreter.file.gid":
		return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "exit.interpreter.file.group":
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "exit.interpreter.file.in_upper_layer":
		return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "exit.interpreter.file.inode":
		return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "exit.interpreter.file.mode":
		return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "exit.interpreter.file.modification_time":
		return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "exit.interpreter.file.mount_id":
		return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "exit.interpreter.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent), nil
	case "exit.interpreter.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent), nil
	case "exit.interpreter.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent), nil
	case "exit.interpreter.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent), nil
	case "exit.interpreter.file.rights":
		return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)), nil
	case "exit.interpreter.file.uid":
		return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "exit.interpreter.file.user":
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "exit.is_kworker":
		return ev.Exit.Process.PIDContext.IsKworker, nil
	case "exit.is_thread":
		return ev.Exit.Process.IsThread, nil
	case "exit.pid":
		return int(ev.Exit.Process.PIDContext.Pid), nil
	case "exit.ppid":
		return int(ev.Exit.Process.PPid), nil
	case "exit.tid":
		return int(ev.Exit.Process.PIDContext.Tid), nil
	case "exit.tty_name":
		return ev.Exit.Process.TTYName, nil
	case "exit.uid":
		return int(ev.Exit.Process.Credentials.UID), nil
	case "exit.user":
		return ev.Exit.Process.Credentials.User, nil
	case "process.ancestors.args":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.args_flags":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process)
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.args_options":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process)
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.args_truncated":
		var values []bool
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.argv":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process)
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.argv0":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.cap_effective":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.CapEffective)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.cap_permitted":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.CapPermitted)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.comm":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Comm
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.container.id":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.ContainerID
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.cookie":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Cookie)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.created_at":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.egid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.EGID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.egroup":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.EGroup
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.envp":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.envs":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.envs_truncated":
		var values []bool
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.euid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.EUID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.euser":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.EUser
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.change_time":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.filesystem":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.gid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.group":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.in_upper_layer":
		var values []bool
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.inode":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Inode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.mode":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.modification_time":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.mount_id":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.MountID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.name":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.name.length":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.path":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.path.length":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.rights":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.uid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.user":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.fsgid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.FSGID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.fsgroup":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.FSGroup
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.fsuid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.FSUID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.fsuser":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.FSUser
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.gid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.group":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.change_time":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.filesystem":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.gid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.group":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.in_upper_layer":
		var values []bool
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.inode":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.mode":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.modification_time":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.mount_id":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.name":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.name.length":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.path":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.path.length":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.rights":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.uid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.user":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.is_kworker":
		var values []bool
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.PIDContext.IsKworker
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.is_thread":
		var values []bool
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.IsThread
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.pid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PIDContext.Pid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.ppid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PPid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.tid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PIDContext.Tid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.tty_name":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.TTYName
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.uid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.user":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.args":
		return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.ProcessContext.Process), nil
	case "process.args_flags":
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.ProcessContext.Process), nil
	case "process.args_options":
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.ProcessContext.Process), nil
	case "process.args_truncated":
		return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.ProcessContext.Process), nil
	case "process.argv":
		return ev.FieldHandlers.ResolveProcessArgv(ev, &ev.ProcessContext.Process), nil
	case "process.argv0":
		return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.ProcessContext.Process), nil
	case "process.cap_effective":
		return int(ev.ProcessContext.Process.Credentials.CapEffective), nil
	case "process.cap_permitted":
		return int(ev.ProcessContext.Process.Credentials.CapPermitted), nil
	case "process.comm":
		return ev.ProcessContext.Process.Comm, nil
	case "process.container.id":
		return ev.ProcessContext.Process.ContainerID, nil
	case "process.cookie":
		return int(ev.ProcessContext.Process.Cookie), nil
	case "process.created_at":
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.ProcessContext.Process)), nil
	case "process.egid":
		return int(ev.ProcessContext.Process.Credentials.EGID), nil
	case "process.egroup":
		return ev.ProcessContext.Process.Credentials.EGroup, nil
	case "process.envp":
		return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.ProcessContext.Process), nil
	case "process.envs":
		return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.ProcessContext.Process), nil
	case "process.envs_truncated":
		return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.ProcessContext.Process), nil
	case "process.euid":
		return int(ev.ProcessContext.Process.Credentials.EUID), nil
	case "process.euser":
		return ev.ProcessContext.Process.Credentials.EUser, nil
	case "process.file.change_time":
		return int(ev.ProcessContext.Process.FileEvent.FileFields.CTime), nil
	case "process.file.filesystem":
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Process.FileEvent), nil
	case "process.file.gid":
		return int(ev.ProcessContext.Process.FileEvent.FileFields.GID), nil
	case "process.file.group":
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Process.FileEvent.FileFields), nil
	case "process.file.in_upper_layer":
		return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Process.FileEvent.FileFields), nil
	case "process.file.inode":
		return int(ev.ProcessContext.Process.FileEvent.FileFields.Inode), nil
	case "process.file.mode":
		return int(ev.ProcessContext.Process.FileEvent.FileFields.Mode), nil
	case "process.file.modification_time":
		return int(ev.ProcessContext.Process.FileEvent.FileFields.MTime), nil
	case "process.file.mount_id":
		return int(ev.ProcessContext.Process.FileEvent.FileFields.MountID), nil
	case "process.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.FileEvent), nil
	case "process.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.FileEvent), nil
	case "process.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.FileEvent), nil
	case "process.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.FileEvent), nil
	case "process.file.rights":
		return int(ev.FieldHandlers.ResolveRights(ev, &ev.ProcessContext.Process.FileEvent.FileFields)), nil
	case "process.file.uid":
		return int(ev.ProcessContext.Process.FileEvent.FileFields.UID), nil
	case "process.file.user":
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Process.FileEvent.FileFields), nil
	case "process.fsgid":
		return int(ev.ProcessContext.Process.Credentials.FSGID), nil
	case "process.fsgroup":
		return ev.ProcessContext.Process.Credentials.FSGroup, nil
	case "process.fsuid":
		return int(ev.ProcessContext.Process.Credentials.FSUID), nil
	case "process.fsuser":
		return ev.ProcessContext.Process.Credentials.FSUser, nil
	case "process.gid":
		return int(ev.ProcessContext.Process.Credentials.GID), nil
	case "process.group":
		return ev.ProcessContext.Process.Credentials.Group, nil
	case "process.interpreter.file.change_time":
		return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "process.interpreter.file.filesystem":
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent), nil
	case "process.interpreter.file.gid":
		return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "process.interpreter.file.group":
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "process.interpreter.file.in_upper_layer":
		return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "process.interpreter.file.inode":
		return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "process.interpreter.file.mode":
		return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "process.interpreter.file.modification_time":
		return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "process.interpreter.file.mount_id":
		return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "process.interpreter.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent), nil
	case "process.interpreter.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent), nil
	case "process.interpreter.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent), nil
	case "process.interpreter.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent), nil
	case "process.interpreter.file.rights":
		return int(ev.FieldHandlers.ResolveRights(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)), nil
	case "process.interpreter.file.uid":
		return int(ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "process.interpreter.file.user":
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields), nil
	case "process.is_kworker":
		return ev.ProcessContext.Process.PIDContext.IsKworker, nil
	case "process.is_thread":
		return ev.ProcessContext.Process.IsThread, nil
	case "process.parent.args":
		return ev.FieldHandlers.ResolveProcessArgs(ev, ev.ProcessContext.Parent), nil
	case "process.parent.args_flags":
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.ProcessContext.Parent), nil
	case "process.parent.args_options":
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.ProcessContext.Parent), nil
	case "process.parent.args_truncated":
		return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.ProcessContext.Parent), nil
	case "process.parent.argv":
		return ev.FieldHandlers.ResolveProcessArgv(ev, ev.ProcessContext.Parent), nil
	case "process.parent.argv0":
		return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.ProcessContext.Parent), nil
	case "process.parent.cap_effective":
		return int(ev.ProcessContext.Parent.Credentials.CapEffective), nil
	case "process.parent.cap_permitted":
		return int(ev.ProcessContext.Parent.Credentials.CapPermitted), nil
	case "process.parent.comm":
		return ev.ProcessContext.Parent.Comm, nil
	case "process.parent.container.id":
		return ev.ProcessContext.Parent.ContainerID, nil
	case "process.parent.cookie":
		return int(ev.ProcessContext.Parent.Cookie), nil
	case "process.parent.created_at":
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.ProcessContext.Parent)), nil
	case "process.parent.egid":
		return int(ev.ProcessContext.Parent.Credentials.EGID), nil
	case "process.parent.egroup":
		return ev.ProcessContext.Parent.Credentials.EGroup, nil
	case "process.parent.envp":
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.ProcessContext.Parent), nil
	case "process.parent.envs":
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.ProcessContext.Parent), nil
	case "process.parent.envs_truncated":
		return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.ProcessContext.Parent), nil
	case "process.parent.euid":
		return int(ev.ProcessContext.Parent.Credentials.EUID), nil
	case "process.parent.euser":
		return ev.ProcessContext.Parent.Credentials.EUser, nil
	case "process.parent.file.change_time":
		return int(ev.ProcessContext.Parent.FileEvent.FileFields.CTime), nil
	case "process.parent.file.filesystem":
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Parent.FileEvent), nil
	case "process.parent.file.gid":
		return int(ev.ProcessContext.Parent.FileEvent.FileFields.GID), nil
	case "process.parent.file.group":
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Parent.FileEvent.FileFields), nil
	case "process.parent.file.in_upper_layer":
		return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Parent.FileEvent.FileFields), nil
	case "process.parent.file.inode":
		return int(ev.ProcessContext.Parent.FileEvent.FileFields.Inode), nil
	case "process.parent.file.mode":
		return int(ev.ProcessContext.Parent.FileEvent.FileFields.Mode), nil
	case "process.parent.file.modification_time":
		return int(ev.ProcessContext.Parent.FileEvent.FileFields.MTime), nil
	case "process.parent.file.mount_id":
		return int(ev.ProcessContext.Parent.FileEvent.FileFields.MountID), nil
	case "process.parent.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.FileEvent), nil
	case "process.parent.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.FileEvent), nil
	case "process.parent.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.FileEvent), nil
	case "process.parent.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.FileEvent), nil
	case "process.parent.file.rights":
		return int(ev.FieldHandlers.ResolveRights(ev, &ev.ProcessContext.Parent.FileEvent.FileFields)), nil
	case "process.parent.file.uid":
		return int(ev.ProcessContext.Parent.FileEvent.FileFields.UID), nil
	case "process.parent.file.user":
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Parent.FileEvent.FileFields), nil
	case "process.parent.fsgid":
		return int(ev.ProcessContext.Parent.Credentials.FSGID), nil
	case "process.parent.fsgroup":
		return ev.ProcessContext.Parent.Credentials.FSGroup, nil
	case "process.parent.fsuid":
		return int(ev.ProcessContext.Parent.Credentials.FSUID), nil
	case "process.parent.fsuser":
		return ev.ProcessContext.Parent.Credentials.FSUser, nil
	case "process.parent.gid":
		return int(ev.ProcessContext.Parent.Credentials.GID), nil
	case "process.parent.group":
		return ev.ProcessContext.Parent.Credentials.Group, nil
	case "process.parent.interpreter.file.change_time":
		return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "process.parent.interpreter.file.filesystem":
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent), nil
	case "process.parent.interpreter.file.gid":
		return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "process.parent.interpreter.file.group":
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields), nil
	case "process.parent.interpreter.file.in_upper_layer":
		return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields), nil
	case "process.parent.interpreter.file.inode":
		return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "process.parent.interpreter.file.mode":
		return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "process.parent.interpreter.file.modification_time":
		return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "process.parent.interpreter.file.mount_id":
		return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "process.parent.interpreter.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent), nil
	case "process.parent.interpreter.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent), nil
	case "process.parent.interpreter.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent), nil
	case "process.parent.interpreter.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent), nil
	case "process.parent.interpreter.file.rights":
		return int(ev.FieldHandlers.ResolveRights(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)), nil
	case "process.parent.interpreter.file.uid":
		return int(ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "process.parent.interpreter.file.user":
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields), nil
	case "process.parent.is_kworker":
		return ev.ProcessContext.Parent.PIDContext.IsKworker, nil
	case "process.parent.is_thread":
		return ev.ProcessContext.Parent.IsThread, nil
	case "process.parent.pid":
		return int(ev.ProcessContext.Parent.PIDContext.Pid), nil
	case "process.parent.ppid":
		return int(ev.ProcessContext.Parent.PPid), nil
	case "process.parent.tid":
		return int(ev.ProcessContext.Parent.PIDContext.Tid), nil
	case "process.parent.tty_name":
		return ev.ProcessContext.Parent.TTYName, nil
	case "process.parent.uid":
		return int(ev.ProcessContext.Parent.Credentials.UID), nil
	case "process.parent.user":
		return ev.ProcessContext.Parent.Credentials.User, nil
	case "process.pid":
		return int(ev.ProcessContext.Process.PIDContext.Pid), nil
	case "process.ppid":
		return int(ev.ProcessContext.Process.PPid), nil
	case "process.tid":
		return int(ev.ProcessContext.Process.PIDContext.Tid), nil
	case "process.tty_name":
		return ev.ProcessContext.Process.TTYName, nil
	case "process.uid":
		return int(ev.ProcessContext.Process.Credentials.UID), nil
	case "process.user":
		return ev.ProcessContext.Process.Credentials.User, nil
	}
	return nil, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {
	case "async":
		return "*", nil
	case "exec.args":
		return "exec", nil
	case "exec.args_flags":
		return "exec", nil
	case "exec.args_options":
		return "exec", nil
	case "exec.args_truncated":
		return "exec", nil
	case "exec.argv":
		return "exec", nil
	case "exec.argv0":
		return "exec", nil
	case "exec.cap_effective":
		return "exec", nil
	case "exec.cap_permitted":
		return "exec", nil
	case "exec.comm":
		return "exec", nil
	case "exec.container.id":
		return "exec", nil
	case "exec.cookie":
		return "exec", nil
	case "exec.created_at":
		return "exec", nil
	case "exec.egid":
		return "exec", nil
	case "exec.egroup":
		return "exec", nil
	case "exec.envp":
		return "exec", nil
	case "exec.envs":
		return "exec", nil
	case "exec.envs_truncated":
		return "exec", nil
	case "exec.euid":
		return "exec", nil
	case "exec.euser":
		return "exec", nil
	case "exec.file.change_time":
		return "exec", nil
	case "exec.file.filesystem":
		return "exec", nil
	case "exec.file.gid":
		return "exec", nil
	case "exec.file.group":
		return "exec", nil
	case "exec.file.in_upper_layer":
		return "exec", nil
	case "exec.file.inode":
		return "exec", nil
	case "exec.file.mode":
		return "exec", nil
	case "exec.file.modification_time":
		return "exec", nil
	case "exec.file.mount_id":
		return "exec", nil
	case "exec.file.name":
		return "exec", nil
	case "exec.file.name.length":
		return "exec", nil
	case "exec.file.path":
		return "exec", nil
	case "exec.file.path.length":
		return "exec", nil
	case "exec.file.rights":
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
	case "exec.interpreter.file.change_time":
		return "exec", nil
	case "exec.interpreter.file.filesystem":
		return "exec", nil
	case "exec.interpreter.file.gid":
		return "exec", nil
	case "exec.interpreter.file.group":
		return "exec", nil
	case "exec.interpreter.file.in_upper_layer":
		return "exec", nil
	case "exec.interpreter.file.inode":
		return "exec", nil
	case "exec.interpreter.file.mode":
		return "exec", nil
	case "exec.interpreter.file.modification_time":
		return "exec", nil
	case "exec.interpreter.file.mount_id":
		return "exec", nil
	case "exec.interpreter.file.name":
		return "exec", nil
	case "exec.interpreter.file.name.length":
		return "exec", nil
	case "exec.interpreter.file.path":
		return "exec", nil
	case "exec.interpreter.file.path.length":
		return "exec", nil
	case "exec.interpreter.file.rights":
		return "exec", nil
	case "exec.interpreter.file.uid":
		return "exec", nil
	case "exec.interpreter.file.user":
		return "exec", nil
	case "exec.is_kworker":
		return "exec", nil
	case "exec.is_thread":
		return "exec", nil
	case "exec.pid":
		return "exec", nil
	case "exec.ppid":
		return "exec", nil
	case "exec.tid":
		return "exec", nil
	case "exec.tty_name":
		return "exec", nil
	case "exec.uid":
		return "exec", nil
	case "exec.user":
		return "exec", nil
	case "exit.args":
		return "exit", nil
	case "exit.args_flags":
		return "exit", nil
	case "exit.args_options":
		return "exit", nil
	case "exit.args_truncated":
		return "exit", nil
	case "exit.argv":
		return "exit", nil
	case "exit.argv0":
		return "exit", nil
	case "exit.cap_effective":
		return "exit", nil
	case "exit.cap_permitted":
		return "exit", nil
	case "exit.cause":
		return "exit", nil
	case "exit.code":
		return "exit", nil
	case "exit.comm":
		return "exit", nil
	case "exit.container.id":
		return "exit", nil
	case "exit.cookie":
		return "exit", nil
	case "exit.created_at":
		return "exit", nil
	case "exit.egid":
		return "exit", nil
	case "exit.egroup":
		return "exit", nil
	case "exit.envp":
		return "exit", nil
	case "exit.envs":
		return "exit", nil
	case "exit.envs_truncated":
		return "exit", nil
	case "exit.euid":
		return "exit", nil
	case "exit.euser":
		return "exit", nil
	case "exit.file.change_time":
		return "exit", nil
	case "exit.file.filesystem":
		return "exit", nil
	case "exit.file.gid":
		return "exit", nil
	case "exit.file.group":
		return "exit", nil
	case "exit.file.in_upper_layer":
		return "exit", nil
	case "exit.file.inode":
		return "exit", nil
	case "exit.file.mode":
		return "exit", nil
	case "exit.file.modification_time":
		return "exit", nil
	case "exit.file.mount_id":
		return "exit", nil
	case "exit.file.name":
		return "exit", nil
	case "exit.file.name.length":
		return "exit", nil
	case "exit.file.path":
		return "exit", nil
	case "exit.file.path.length":
		return "exit", nil
	case "exit.file.rights":
		return "exit", nil
	case "exit.file.uid":
		return "exit", nil
	case "exit.file.user":
		return "exit", nil
	case "exit.fsgid":
		return "exit", nil
	case "exit.fsgroup":
		return "exit", nil
	case "exit.fsuid":
		return "exit", nil
	case "exit.fsuser":
		return "exit", nil
	case "exit.gid":
		return "exit", nil
	case "exit.group":
		return "exit", nil
	case "exit.interpreter.file.change_time":
		return "exit", nil
	case "exit.interpreter.file.filesystem":
		return "exit", nil
	case "exit.interpreter.file.gid":
		return "exit", nil
	case "exit.interpreter.file.group":
		return "exit", nil
	case "exit.interpreter.file.in_upper_layer":
		return "exit", nil
	case "exit.interpreter.file.inode":
		return "exit", nil
	case "exit.interpreter.file.mode":
		return "exit", nil
	case "exit.interpreter.file.modification_time":
		return "exit", nil
	case "exit.interpreter.file.mount_id":
		return "exit", nil
	case "exit.interpreter.file.name":
		return "exit", nil
	case "exit.interpreter.file.name.length":
		return "exit", nil
	case "exit.interpreter.file.path":
		return "exit", nil
	case "exit.interpreter.file.path.length":
		return "exit", nil
	case "exit.interpreter.file.rights":
		return "exit", nil
	case "exit.interpreter.file.uid":
		return "exit", nil
	case "exit.interpreter.file.user":
		return "exit", nil
	case "exit.is_kworker":
		return "exit", nil
	case "exit.is_thread":
		return "exit", nil
	case "exit.pid":
		return "exit", nil
	case "exit.ppid":
		return "exit", nil
	case "exit.tid":
		return "exit", nil
	case "exit.tty_name":
		return "exit", nil
	case "exit.uid":
		return "exit", nil
	case "exit.user":
		return "exit", nil
	case "process.ancestors.args":
		return "*", nil
	case "process.ancestors.args_flags":
		return "*", nil
	case "process.ancestors.args_options":
		return "*", nil
	case "process.ancestors.args_truncated":
		return "*", nil
	case "process.ancestors.argv":
		return "*", nil
	case "process.ancestors.argv0":
		return "*", nil
	case "process.ancestors.cap_effective":
		return "*", nil
	case "process.ancestors.cap_permitted":
		return "*", nil
	case "process.ancestors.comm":
		return "*", nil
	case "process.ancestors.container.id":
		return "*", nil
	case "process.ancestors.cookie":
		return "*", nil
	case "process.ancestors.created_at":
		return "*", nil
	case "process.ancestors.egid":
		return "*", nil
	case "process.ancestors.egroup":
		return "*", nil
	case "process.ancestors.envp":
		return "*", nil
	case "process.ancestors.envs":
		return "*", nil
	case "process.ancestors.envs_truncated":
		return "*", nil
	case "process.ancestors.euid":
		return "*", nil
	case "process.ancestors.euser":
		return "*", nil
	case "process.ancestors.file.change_time":
		return "*", nil
	case "process.ancestors.file.filesystem":
		return "*", nil
	case "process.ancestors.file.gid":
		return "*", nil
	case "process.ancestors.file.group":
		return "*", nil
	case "process.ancestors.file.in_upper_layer":
		return "*", nil
	case "process.ancestors.file.inode":
		return "*", nil
	case "process.ancestors.file.mode":
		return "*", nil
	case "process.ancestors.file.modification_time":
		return "*", nil
	case "process.ancestors.file.mount_id":
		return "*", nil
	case "process.ancestors.file.name":
		return "*", nil
	case "process.ancestors.file.name.length":
		return "*", nil
	case "process.ancestors.file.path":
		return "*", nil
	case "process.ancestors.file.path.length":
		return "*", nil
	case "process.ancestors.file.rights":
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
	case "process.ancestors.interpreter.file.change_time":
		return "*", nil
	case "process.ancestors.interpreter.file.filesystem":
		return "*", nil
	case "process.ancestors.interpreter.file.gid":
		return "*", nil
	case "process.ancestors.interpreter.file.group":
		return "*", nil
	case "process.ancestors.interpreter.file.in_upper_layer":
		return "*", nil
	case "process.ancestors.interpreter.file.inode":
		return "*", nil
	case "process.ancestors.interpreter.file.mode":
		return "*", nil
	case "process.ancestors.interpreter.file.modification_time":
		return "*", nil
	case "process.ancestors.interpreter.file.mount_id":
		return "*", nil
	case "process.ancestors.interpreter.file.name":
		return "*", nil
	case "process.ancestors.interpreter.file.name.length":
		return "*", nil
	case "process.ancestors.interpreter.file.path":
		return "*", nil
	case "process.ancestors.interpreter.file.path.length":
		return "*", nil
	case "process.ancestors.interpreter.file.rights":
		return "*", nil
	case "process.ancestors.interpreter.file.uid":
		return "*", nil
	case "process.ancestors.interpreter.file.user":
		return "*", nil
	case "process.ancestors.is_kworker":
		return "*", nil
	case "process.ancestors.is_thread":
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
	case "process.args":
		return "*", nil
	case "process.args_flags":
		return "*", nil
	case "process.args_options":
		return "*", nil
	case "process.args_truncated":
		return "*", nil
	case "process.argv":
		return "*", nil
	case "process.argv0":
		return "*", nil
	case "process.cap_effective":
		return "*", nil
	case "process.cap_permitted":
		return "*", nil
	case "process.comm":
		return "*", nil
	case "process.container.id":
		return "*", nil
	case "process.cookie":
		return "*", nil
	case "process.created_at":
		return "*", nil
	case "process.egid":
		return "*", nil
	case "process.egroup":
		return "*", nil
	case "process.envp":
		return "*", nil
	case "process.envs":
		return "*", nil
	case "process.envs_truncated":
		return "*", nil
	case "process.euid":
		return "*", nil
	case "process.euser":
		return "*", nil
	case "process.file.change_time":
		return "*", nil
	case "process.file.filesystem":
		return "*", nil
	case "process.file.gid":
		return "*", nil
	case "process.file.group":
		return "*", nil
	case "process.file.in_upper_layer":
		return "*", nil
	case "process.file.inode":
		return "*", nil
	case "process.file.mode":
		return "*", nil
	case "process.file.modification_time":
		return "*", nil
	case "process.file.mount_id":
		return "*", nil
	case "process.file.name":
		return "*", nil
	case "process.file.name.length":
		return "*", nil
	case "process.file.path":
		return "*", nil
	case "process.file.path.length":
		return "*", nil
	case "process.file.rights":
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
	case "process.interpreter.file.change_time":
		return "*", nil
	case "process.interpreter.file.filesystem":
		return "*", nil
	case "process.interpreter.file.gid":
		return "*", nil
	case "process.interpreter.file.group":
		return "*", nil
	case "process.interpreter.file.in_upper_layer":
		return "*", nil
	case "process.interpreter.file.inode":
		return "*", nil
	case "process.interpreter.file.mode":
		return "*", nil
	case "process.interpreter.file.modification_time":
		return "*", nil
	case "process.interpreter.file.mount_id":
		return "*", nil
	case "process.interpreter.file.name":
		return "*", nil
	case "process.interpreter.file.name.length":
		return "*", nil
	case "process.interpreter.file.path":
		return "*", nil
	case "process.interpreter.file.path.length":
		return "*", nil
	case "process.interpreter.file.rights":
		return "*", nil
	case "process.interpreter.file.uid":
		return "*", nil
	case "process.interpreter.file.user":
		return "*", nil
	case "process.is_kworker":
		return "*", nil
	case "process.is_thread":
		return "*", nil
	case "process.parent.args":
		return "*", nil
	case "process.parent.args_flags":
		return "*", nil
	case "process.parent.args_options":
		return "*", nil
	case "process.parent.args_truncated":
		return "*", nil
	case "process.parent.argv":
		return "*", nil
	case "process.parent.argv0":
		return "*", nil
	case "process.parent.cap_effective":
		return "*", nil
	case "process.parent.cap_permitted":
		return "*", nil
	case "process.parent.comm":
		return "*", nil
	case "process.parent.container.id":
		return "*", nil
	case "process.parent.cookie":
		return "*", nil
	case "process.parent.created_at":
		return "*", nil
	case "process.parent.egid":
		return "*", nil
	case "process.parent.egroup":
		return "*", nil
	case "process.parent.envp":
		return "*", nil
	case "process.parent.envs":
		return "*", nil
	case "process.parent.envs_truncated":
		return "*", nil
	case "process.parent.euid":
		return "*", nil
	case "process.parent.euser":
		return "*", nil
	case "process.parent.file.change_time":
		return "*", nil
	case "process.parent.file.filesystem":
		return "*", nil
	case "process.parent.file.gid":
		return "*", nil
	case "process.parent.file.group":
		return "*", nil
	case "process.parent.file.in_upper_layer":
		return "*", nil
	case "process.parent.file.inode":
		return "*", nil
	case "process.parent.file.mode":
		return "*", nil
	case "process.parent.file.modification_time":
		return "*", nil
	case "process.parent.file.mount_id":
		return "*", nil
	case "process.parent.file.name":
		return "*", nil
	case "process.parent.file.name.length":
		return "*", nil
	case "process.parent.file.path":
		return "*", nil
	case "process.parent.file.path.length":
		return "*", nil
	case "process.parent.file.rights":
		return "*", nil
	case "process.parent.file.uid":
		return "*", nil
	case "process.parent.file.user":
		return "*", nil
	case "process.parent.fsgid":
		return "*", nil
	case "process.parent.fsgroup":
		return "*", nil
	case "process.parent.fsuid":
		return "*", nil
	case "process.parent.fsuser":
		return "*", nil
	case "process.parent.gid":
		return "*", nil
	case "process.parent.group":
		return "*", nil
	case "process.parent.interpreter.file.change_time":
		return "*", nil
	case "process.parent.interpreter.file.filesystem":
		return "*", nil
	case "process.parent.interpreter.file.gid":
		return "*", nil
	case "process.parent.interpreter.file.group":
		return "*", nil
	case "process.parent.interpreter.file.in_upper_layer":
		return "*", nil
	case "process.parent.interpreter.file.inode":
		return "*", nil
	case "process.parent.interpreter.file.mode":
		return "*", nil
	case "process.parent.interpreter.file.modification_time":
		return "*", nil
	case "process.parent.interpreter.file.mount_id":
		return "*", nil
	case "process.parent.interpreter.file.name":
		return "*", nil
	case "process.parent.interpreter.file.name.length":
		return "*", nil
	case "process.parent.interpreter.file.path":
		return "*", nil
	case "process.parent.interpreter.file.path.length":
		return "*", nil
	case "process.parent.interpreter.file.rights":
		return "*", nil
	case "process.parent.interpreter.file.uid":
		return "*", nil
	case "process.parent.interpreter.file.user":
		return "*", nil
	case "process.parent.is_kworker":
		return "*", nil
	case "process.parent.is_thread":
		return "*", nil
	case "process.parent.pid":
		return "*", nil
	case "process.parent.ppid":
		return "*", nil
	case "process.parent.tid":
		return "*", nil
	case "process.parent.tty_name":
		return "*", nil
	case "process.parent.uid":
		return "*", nil
	case "process.parent.user":
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
	}
	return "", &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {
	case "async":
		return reflect.Bool, nil
	case "exec.args":
		return reflect.String, nil
	case "exec.args_flags":
		return reflect.String, nil
	case "exec.args_options":
		return reflect.String, nil
	case "exec.args_truncated":
		return reflect.Bool, nil
	case "exec.argv":
		return reflect.String, nil
	case "exec.argv0":
		return reflect.String, nil
	case "exec.cap_effective":
		return reflect.Int, nil
	case "exec.cap_permitted":
		return reflect.Int, nil
	case "exec.comm":
		return reflect.String, nil
	case "exec.container.id":
		return reflect.String, nil
	case "exec.cookie":
		return reflect.Int, nil
	case "exec.created_at":
		return reflect.Int, nil
	case "exec.egid":
		return reflect.Int, nil
	case "exec.egroup":
		return reflect.String, nil
	case "exec.envp":
		return reflect.String, nil
	case "exec.envs":
		return reflect.String, nil
	case "exec.envs_truncated":
		return reflect.Bool, nil
	case "exec.euid":
		return reflect.Int, nil
	case "exec.euser":
		return reflect.String, nil
	case "exec.file.change_time":
		return reflect.Int, nil
	case "exec.file.filesystem":
		return reflect.String, nil
	case "exec.file.gid":
		return reflect.Int, nil
	case "exec.file.group":
		return reflect.String, nil
	case "exec.file.in_upper_layer":
		return reflect.Bool, nil
	case "exec.file.inode":
		return reflect.Int, nil
	case "exec.file.mode":
		return reflect.Int, nil
	case "exec.file.modification_time":
		return reflect.Int, nil
	case "exec.file.mount_id":
		return reflect.Int, nil
	case "exec.file.name":
		return reflect.String, nil
	case "exec.file.name.length":
		return reflect.Int, nil
	case "exec.file.path":
		return reflect.String, nil
	case "exec.file.path.length":
		return reflect.Int, nil
	case "exec.file.rights":
		return reflect.Int, nil
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
	case "exec.interpreter.file.change_time":
		return reflect.Int, nil
	case "exec.interpreter.file.filesystem":
		return reflect.String, nil
	case "exec.interpreter.file.gid":
		return reflect.Int, nil
	case "exec.interpreter.file.group":
		return reflect.String, nil
	case "exec.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "exec.interpreter.file.inode":
		return reflect.Int, nil
	case "exec.interpreter.file.mode":
		return reflect.Int, nil
	case "exec.interpreter.file.modification_time":
		return reflect.Int, nil
	case "exec.interpreter.file.mount_id":
		return reflect.Int, nil
	case "exec.interpreter.file.name":
		return reflect.String, nil
	case "exec.interpreter.file.name.length":
		return reflect.Int, nil
	case "exec.interpreter.file.path":
		return reflect.String, nil
	case "exec.interpreter.file.path.length":
		return reflect.Int, nil
	case "exec.interpreter.file.rights":
		return reflect.Int, nil
	case "exec.interpreter.file.uid":
		return reflect.Int, nil
	case "exec.interpreter.file.user":
		return reflect.String, nil
	case "exec.is_kworker":
		return reflect.Bool, nil
	case "exec.is_thread":
		return reflect.Bool, nil
	case "exec.pid":
		return reflect.Int, nil
	case "exec.ppid":
		return reflect.Int, nil
	case "exec.tid":
		return reflect.Int, nil
	case "exec.tty_name":
		return reflect.String, nil
	case "exec.uid":
		return reflect.Int, nil
	case "exec.user":
		return reflect.String, nil
	case "exit.args":
		return reflect.String, nil
	case "exit.args_flags":
		return reflect.String, nil
	case "exit.args_options":
		return reflect.String, nil
	case "exit.args_truncated":
		return reflect.Bool, nil
	case "exit.argv":
		return reflect.String, nil
	case "exit.argv0":
		return reflect.String, nil
	case "exit.cap_effective":
		return reflect.Int, nil
	case "exit.cap_permitted":
		return reflect.Int, nil
	case "exit.cause":
		return reflect.Int, nil
	case "exit.code":
		return reflect.Int, nil
	case "exit.comm":
		return reflect.String, nil
	case "exit.container.id":
		return reflect.String, nil
	case "exit.cookie":
		return reflect.Int, nil
	case "exit.created_at":
		return reflect.Int, nil
	case "exit.egid":
		return reflect.Int, nil
	case "exit.egroup":
		return reflect.String, nil
	case "exit.envp":
		return reflect.String, nil
	case "exit.envs":
		return reflect.String, nil
	case "exit.envs_truncated":
		return reflect.Bool, nil
	case "exit.euid":
		return reflect.Int, nil
	case "exit.euser":
		return reflect.String, nil
	case "exit.file.change_time":
		return reflect.Int, nil
	case "exit.file.filesystem":
		return reflect.String, nil
	case "exit.file.gid":
		return reflect.Int, nil
	case "exit.file.group":
		return reflect.String, nil
	case "exit.file.in_upper_layer":
		return reflect.Bool, nil
	case "exit.file.inode":
		return reflect.Int, nil
	case "exit.file.mode":
		return reflect.Int, nil
	case "exit.file.modification_time":
		return reflect.Int, nil
	case "exit.file.mount_id":
		return reflect.Int, nil
	case "exit.file.name":
		return reflect.String, nil
	case "exit.file.name.length":
		return reflect.Int, nil
	case "exit.file.path":
		return reflect.String, nil
	case "exit.file.path.length":
		return reflect.Int, nil
	case "exit.file.rights":
		return reflect.Int, nil
	case "exit.file.uid":
		return reflect.Int, nil
	case "exit.file.user":
		return reflect.String, nil
	case "exit.fsgid":
		return reflect.Int, nil
	case "exit.fsgroup":
		return reflect.String, nil
	case "exit.fsuid":
		return reflect.Int, nil
	case "exit.fsuser":
		return reflect.String, nil
	case "exit.gid":
		return reflect.Int, nil
	case "exit.group":
		return reflect.String, nil
	case "exit.interpreter.file.change_time":
		return reflect.Int, nil
	case "exit.interpreter.file.filesystem":
		return reflect.String, nil
	case "exit.interpreter.file.gid":
		return reflect.Int, nil
	case "exit.interpreter.file.group":
		return reflect.String, nil
	case "exit.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "exit.interpreter.file.inode":
		return reflect.Int, nil
	case "exit.interpreter.file.mode":
		return reflect.Int, nil
	case "exit.interpreter.file.modification_time":
		return reflect.Int, nil
	case "exit.interpreter.file.mount_id":
		return reflect.Int, nil
	case "exit.interpreter.file.name":
		return reflect.String, nil
	case "exit.interpreter.file.name.length":
		return reflect.Int, nil
	case "exit.interpreter.file.path":
		return reflect.String, nil
	case "exit.interpreter.file.path.length":
		return reflect.Int, nil
	case "exit.interpreter.file.rights":
		return reflect.Int, nil
	case "exit.interpreter.file.uid":
		return reflect.Int, nil
	case "exit.interpreter.file.user":
		return reflect.String, nil
	case "exit.is_kworker":
		return reflect.Bool, nil
	case "exit.is_thread":
		return reflect.Bool, nil
	case "exit.pid":
		return reflect.Int, nil
	case "exit.ppid":
		return reflect.Int, nil
	case "exit.tid":
		return reflect.Int, nil
	case "exit.tty_name":
		return reflect.String, nil
	case "exit.uid":
		return reflect.Int, nil
	case "exit.user":
		return reflect.String, nil
	case "process.ancestors.args":
		return reflect.String, nil
	case "process.ancestors.args_flags":
		return reflect.String, nil
	case "process.ancestors.args_options":
		return reflect.String, nil
	case "process.ancestors.args_truncated":
		return reflect.Bool, nil
	case "process.ancestors.argv":
		return reflect.String, nil
	case "process.ancestors.argv0":
		return reflect.String, nil
	case "process.ancestors.cap_effective":
		return reflect.Int, nil
	case "process.ancestors.cap_permitted":
		return reflect.Int, nil
	case "process.ancestors.comm":
		return reflect.String, nil
	case "process.ancestors.container.id":
		return reflect.String, nil
	case "process.ancestors.cookie":
		return reflect.Int, nil
	case "process.ancestors.created_at":
		return reflect.Int, nil
	case "process.ancestors.egid":
		return reflect.Int, nil
	case "process.ancestors.egroup":
		return reflect.String, nil
	case "process.ancestors.envp":
		return reflect.String, nil
	case "process.ancestors.envs":
		return reflect.String, nil
	case "process.ancestors.envs_truncated":
		return reflect.Bool, nil
	case "process.ancestors.euid":
		return reflect.Int, nil
	case "process.ancestors.euser":
		return reflect.String, nil
	case "process.ancestors.file.change_time":
		return reflect.Int, nil
	case "process.ancestors.file.filesystem":
		return reflect.String, nil
	case "process.ancestors.file.gid":
		return reflect.Int, nil
	case "process.ancestors.file.group":
		return reflect.String, nil
	case "process.ancestors.file.in_upper_layer":
		return reflect.Bool, nil
	case "process.ancestors.file.inode":
		return reflect.Int, nil
	case "process.ancestors.file.mode":
		return reflect.Int, nil
	case "process.ancestors.file.modification_time":
		return reflect.Int, nil
	case "process.ancestors.file.mount_id":
		return reflect.Int, nil
	case "process.ancestors.file.name":
		return reflect.String, nil
	case "process.ancestors.file.name.length":
		return reflect.Int, nil
	case "process.ancestors.file.path":
		return reflect.String, nil
	case "process.ancestors.file.path.length":
		return reflect.Int, nil
	case "process.ancestors.file.rights":
		return reflect.Int, nil
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
	case "process.ancestors.interpreter.file.change_time":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.filesystem":
		return reflect.String, nil
	case "process.ancestors.interpreter.file.gid":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.group":
		return reflect.String, nil
	case "process.ancestors.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "process.ancestors.interpreter.file.inode":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.mode":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.modification_time":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.mount_id":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.name":
		return reflect.String, nil
	case "process.ancestors.interpreter.file.name.length":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.path":
		return reflect.String, nil
	case "process.ancestors.interpreter.file.path.length":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.rights":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.uid":
		return reflect.Int, nil
	case "process.ancestors.interpreter.file.user":
		return reflect.String, nil
	case "process.ancestors.is_kworker":
		return reflect.Bool, nil
	case "process.ancestors.is_thread":
		return reflect.Bool, nil
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
	case "process.args":
		return reflect.String, nil
	case "process.args_flags":
		return reflect.String, nil
	case "process.args_options":
		return reflect.String, nil
	case "process.args_truncated":
		return reflect.Bool, nil
	case "process.argv":
		return reflect.String, nil
	case "process.argv0":
		return reflect.String, nil
	case "process.cap_effective":
		return reflect.Int, nil
	case "process.cap_permitted":
		return reflect.Int, nil
	case "process.comm":
		return reflect.String, nil
	case "process.container.id":
		return reflect.String, nil
	case "process.cookie":
		return reflect.Int, nil
	case "process.created_at":
		return reflect.Int, nil
	case "process.egid":
		return reflect.Int, nil
	case "process.egroup":
		return reflect.String, nil
	case "process.envp":
		return reflect.String, nil
	case "process.envs":
		return reflect.String, nil
	case "process.envs_truncated":
		return reflect.Bool, nil
	case "process.euid":
		return reflect.Int, nil
	case "process.euser":
		return reflect.String, nil
	case "process.file.change_time":
		return reflect.Int, nil
	case "process.file.filesystem":
		return reflect.String, nil
	case "process.file.gid":
		return reflect.Int, nil
	case "process.file.group":
		return reflect.String, nil
	case "process.file.in_upper_layer":
		return reflect.Bool, nil
	case "process.file.inode":
		return reflect.Int, nil
	case "process.file.mode":
		return reflect.Int, nil
	case "process.file.modification_time":
		return reflect.Int, nil
	case "process.file.mount_id":
		return reflect.Int, nil
	case "process.file.name":
		return reflect.String, nil
	case "process.file.name.length":
		return reflect.Int, nil
	case "process.file.path":
		return reflect.String, nil
	case "process.file.path.length":
		return reflect.Int, nil
	case "process.file.rights":
		return reflect.Int, nil
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
	case "process.interpreter.file.change_time":
		return reflect.Int, nil
	case "process.interpreter.file.filesystem":
		return reflect.String, nil
	case "process.interpreter.file.gid":
		return reflect.Int, nil
	case "process.interpreter.file.group":
		return reflect.String, nil
	case "process.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "process.interpreter.file.inode":
		return reflect.Int, nil
	case "process.interpreter.file.mode":
		return reflect.Int, nil
	case "process.interpreter.file.modification_time":
		return reflect.Int, nil
	case "process.interpreter.file.mount_id":
		return reflect.Int, nil
	case "process.interpreter.file.name":
		return reflect.String, nil
	case "process.interpreter.file.name.length":
		return reflect.Int, nil
	case "process.interpreter.file.path":
		return reflect.String, nil
	case "process.interpreter.file.path.length":
		return reflect.Int, nil
	case "process.interpreter.file.rights":
		return reflect.Int, nil
	case "process.interpreter.file.uid":
		return reflect.Int, nil
	case "process.interpreter.file.user":
		return reflect.String, nil
	case "process.is_kworker":
		return reflect.Bool, nil
	case "process.is_thread":
		return reflect.Bool, nil
	case "process.parent.args":
		return reflect.String, nil
	case "process.parent.args_flags":
		return reflect.String, nil
	case "process.parent.args_options":
		return reflect.String, nil
	case "process.parent.args_truncated":
		return reflect.Bool, nil
	case "process.parent.argv":
		return reflect.String, nil
	case "process.parent.argv0":
		return reflect.String, nil
	case "process.parent.cap_effective":
		return reflect.Int, nil
	case "process.parent.cap_permitted":
		return reflect.Int, nil
	case "process.parent.comm":
		return reflect.String, nil
	case "process.parent.container.id":
		return reflect.String, nil
	case "process.parent.cookie":
		return reflect.Int, nil
	case "process.parent.created_at":
		return reflect.Int, nil
	case "process.parent.egid":
		return reflect.Int, nil
	case "process.parent.egroup":
		return reflect.String, nil
	case "process.parent.envp":
		return reflect.String, nil
	case "process.parent.envs":
		return reflect.String, nil
	case "process.parent.envs_truncated":
		return reflect.Bool, nil
	case "process.parent.euid":
		return reflect.Int, nil
	case "process.parent.euser":
		return reflect.String, nil
	case "process.parent.file.change_time":
		return reflect.Int, nil
	case "process.parent.file.filesystem":
		return reflect.String, nil
	case "process.parent.file.gid":
		return reflect.Int, nil
	case "process.parent.file.group":
		return reflect.String, nil
	case "process.parent.file.in_upper_layer":
		return reflect.Bool, nil
	case "process.parent.file.inode":
		return reflect.Int, nil
	case "process.parent.file.mode":
		return reflect.Int, nil
	case "process.parent.file.modification_time":
		return reflect.Int, nil
	case "process.parent.file.mount_id":
		return reflect.Int, nil
	case "process.parent.file.name":
		return reflect.String, nil
	case "process.parent.file.name.length":
		return reflect.Int, nil
	case "process.parent.file.path":
		return reflect.String, nil
	case "process.parent.file.path.length":
		return reflect.Int, nil
	case "process.parent.file.rights":
		return reflect.Int, nil
	case "process.parent.file.uid":
		return reflect.Int, nil
	case "process.parent.file.user":
		return reflect.String, nil
	case "process.parent.fsgid":
		return reflect.Int, nil
	case "process.parent.fsgroup":
		return reflect.String, nil
	case "process.parent.fsuid":
		return reflect.Int, nil
	case "process.parent.fsuser":
		return reflect.String, nil
	case "process.parent.gid":
		return reflect.Int, nil
	case "process.parent.group":
		return reflect.String, nil
	case "process.parent.interpreter.file.change_time":
		return reflect.Int, nil
	case "process.parent.interpreter.file.filesystem":
		return reflect.String, nil
	case "process.parent.interpreter.file.gid":
		return reflect.Int, nil
	case "process.parent.interpreter.file.group":
		return reflect.String, nil
	case "process.parent.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "process.parent.interpreter.file.inode":
		return reflect.Int, nil
	case "process.parent.interpreter.file.mode":
		return reflect.Int, nil
	case "process.parent.interpreter.file.modification_time":
		return reflect.Int, nil
	case "process.parent.interpreter.file.mount_id":
		return reflect.Int, nil
	case "process.parent.interpreter.file.name":
		return reflect.String, nil
	case "process.parent.interpreter.file.name.length":
		return reflect.Int, nil
	case "process.parent.interpreter.file.path":
		return reflect.String, nil
	case "process.parent.interpreter.file.path.length":
		return reflect.Int, nil
	case "process.parent.interpreter.file.rights":
		return reflect.Int, nil
	case "process.parent.interpreter.file.uid":
		return reflect.Int, nil
	case "process.parent.interpreter.file.user":
		return reflect.String, nil
	case "process.parent.is_kworker":
		return reflect.Bool, nil
	case "process.parent.is_thread":
		return reflect.Bool, nil
	case "process.parent.pid":
		return reflect.Int, nil
	case "process.parent.ppid":
		return reflect.Int, nil
	case "process.parent.tid":
		return reflect.Int, nil
	case "process.parent.tty_name":
		return reflect.String, nil
	case "process.parent.uid":
		return reflect.Int, nil
	case "process.parent.user":
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
	}
	return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) SetFieldValue(field eval.Field, value interface{}) error {
	switch field {
	case "async":
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Async"}
		}
		ev.Async = rv
		return nil
	case "exec.args":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Args"}
		}
		ev.Exec.Process.Args = rv
		return nil
	case "exec.args_flags":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exec.Process.Argv = append(ev.Exec.Process.Argv, rv)
		case []string:
			ev.Exec.Process.Argv = append(ev.Exec.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		return nil
	case "exec.args_options":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exec.Process.Argv = append(ev.Exec.Process.Argv, rv)
		case []string:
			ev.Exec.Process.Argv = append(ev.Exec.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		return nil
	case "exec.args_truncated":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.ArgsTruncated"}
		}
		ev.Exec.Process.ArgsTruncated = rv
		return nil
	case "exec.argv":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exec.Process.Argv = append(ev.Exec.Process.Argv, rv)
		case []string:
			ev.Exec.Process.Argv = append(ev.Exec.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		return nil
	case "exec.argv0":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv0"}
		}
		ev.Exec.Process.Argv0 = rv
		return nil
	case "exec.cap_effective":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.CapEffective"}
		}
		ev.Exec.Process.Credentials.CapEffective = uint64(rv)
		return nil
	case "exec.cap_permitted":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.CapPermitted"}
		}
		ev.Exec.Process.Credentials.CapPermitted = uint64(rv)
		return nil
	case "exec.comm":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Comm"}
		}
		ev.Exec.Process.Comm = rv
		return nil
	case "exec.container.id":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.ContainerID"}
		}
		ev.Exec.Process.ContainerID = rv
		return nil
	case "exec.cookie":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Cookie"}
		}
		ev.Exec.Process.Cookie = uint32(rv)
		return nil
	case "exec.created_at":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.CreatedAt"}
		}
		ev.Exec.Process.CreatedAt = uint64(rv)
		return nil
	case "exec.egid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EGID"}
		}
		ev.Exec.Process.Credentials.EGID = uint32(rv)
		return nil
	case "exec.egroup":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EGroup"}
		}
		ev.Exec.Process.Credentials.EGroup = rv
		return nil
	case "exec.envp":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exec.Process.Envp = append(ev.Exec.Process.Envp, rv)
		case []string:
			ev.Exec.Process.Envp = append(ev.Exec.Process.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Envp"}
		}
		return nil
	case "exec.envs":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exec.Process.Envs = append(ev.Exec.Process.Envs, rv)
		case []string:
			ev.Exec.Process.Envs = append(ev.Exec.Process.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Envs"}
		}
		return nil
	case "exec.envs_truncated":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.EnvsTruncated"}
		}
		ev.Exec.Process.EnvsTruncated = rv
		return nil
	case "exec.euid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EUID"}
		}
		ev.Exec.Process.Credentials.EUID = uint32(rv)
		return nil
	case "exec.euser":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EUser"}
		}
		ev.Exec.Process.Credentials.EUser = rv
		return nil
	case "exec.file.change_time":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.CTime"}
		}
		ev.Exec.Process.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "exec.file.filesystem":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.Filesystem"}
		}
		ev.Exec.Process.FileEvent.Filesystem = rv
		return nil
	case "exec.file.gid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.GID"}
		}
		ev.Exec.Process.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "exec.file.group":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.Group"}
		}
		ev.Exec.Process.FileEvent.FileFields.Group = rv
		return nil
	case "exec.file.in_upper_layer":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.InUpperLayer"}
		}
		ev.Exec.Process.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "exec.file.inode":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.Inode"}
		}
		ev.Exec.Process.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "exec.file.mode":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.Mode"}
		}
		ev.Exec.Process.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "exec.file.modification_time":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.MTime"}
		}
		ev.Exec.Process.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "exec.file.mount_id":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.MountID"}
		}
		ev.Exec.Process.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "exec.file.name":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.BasenameStr"}
		}
		ev.Exec.Process.FileEvent.BasenameStr = rv
		return nil
	case "exec.file.name.length":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.file.name.length"}
	case "exec.file.path":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.PathnameStr"}
		}
		ev.Exec.Process.FileEvent.PathnameStr = rv
		return nil
	case "exec.file.path.length":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.file.path.length"}
	case "exec.file.rights":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.Mode"}
		}
		ev.Exec.Process.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "exec.file.uid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.UID"}
		}
		ev.Exec.Process.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "exec.file.user":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.User"}
		}
		ev.Exec.Process.FileEvent.FileFields.User = rv
		return nil
	case "exec.fsgid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSGID"}
		}
		ev.Exec.Process.Credentials.FSGID = uint32(rv)
		return nil
	case "exec.fsgroup":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSGroup"}
		}
		ev.Exec.Process.Credentials.FSGroup = rv
		return nil
	case "exec.fsuid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSUID"}
		}
		ev.Exec.Process.Credentials.FSUID = uint32(rv)
		return nil
	case "exec.fsuser":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSUser"}
		}
		ev.Exec.Process.Credentials.FSUser = rv
		return nil
	case "exec.gid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.GID"}
		}
		ev.Exec.Process.Credentials.GID = uint32(rv)
		return nil
	case "exec.group":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.Group"}
		}
		ev.Exec.Process.Credentials.Group = rv
		return nil
	case "exec.interpreter.file.change_time":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "exec.interpreter.file.filesystem":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.Filesystem = rv
		return nil
	case "exec.interpreter.file.gid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "exec.interpreter.file.group":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Group = rv
		return nil
	case "exec.interpreter.file.in_upper_layer":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "exec.interpreter.file.inode":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "exec.interpreter.file.mode":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "exec.interpreter.file.modification_time":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "exec.interpreter.file.mount_id":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "exec.interpreter.file.name":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.BasenameStr = rv
		return nil
	case "exec.interpreter.file.name.length":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.interpreter.file.name.length"}
	case "exec.interpreter.file.path":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.PathnameStr = rv
		return nil
	case "exec.interpreter.file.path.length":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.interpreter.file.path.length"}
	case "exec.interpreter.file.rights":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "exec.interpreter.file.uid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "exec.interpreter.file.user":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.User = rv
		return nil
	case "exec.is_kworker":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PIDContext.IsKworker"}
		}
		ev.Exec.Process.PIDContext.IsKworker = rv
		return nil
	case "exec.is_thread":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.IsThread"}
		}
		ev.Exec.Process.IsThread = rv
		return nil
	case "exec.pid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PIDContext.Pid"}
		}
		ev.Exec.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "exec.ppid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PPid"}
		}
		ev.Exec.Process.PPid = uint32(rv)
		return nil
	case "exec.tid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PIDContext.Tid"}
		}
		ev.Exec.Process.PIDContext.Tid = uint32(rv)
		return nil
	case "exec.tty_name":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.TTYName"}
		}
		ev.Exec.Process.TTYName = rv
		return nil
	case "exec.uid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.UID"}
		}
		ev.Exec.Process.Credentials.UID = uint32(rv)
		return nil
	case "exec.user":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.User"}
		}
		ev.Exec.Process.Credentials.User = rv
		return nil
	case "exit.args":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Args"}
		}
		ev.Exit.Process.Args = rv
		return nil
	case "exit.args_flags":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exit.Process.Argv = append(ev.Exit.Process.Argv, rv)
		case []string:
			ev.Exit.Process.Argv = append(ev.Exit.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Argv"}
		}
		return nil
	case "exit.args_options":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exit.Process.Argv = append(ev.Exit.Process.Argv, rv)
		case []string:
			ev.Exit.Process.Argv = append(ev.Exit.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Argv"}
		}
		return nil
	case "exit.args_truncated":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.ArgsTruncated"}
		}
		ev.Exit.Process.ArgsTruncated = rv
		return nil
	case "exit.argv":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exit.Process.Argv = append(ev.Exit.Process.Argv, rv)
		case []string:
			ev.Exit.Process.Argv = append(ev.Exit.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Argv"}
		}
		return nil
	case "exit.argv0":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Argv0"}
		}
		ev.Exit.Process.Argv0 = rv
		return nil
	case "exit.cap_effective":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.CapEffective"}
		}
		ev.Exit.Process.Credentials.CapEffective = uint64(rv)
		return nil
	case "exit.cap_permitted":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.CapPermitted"}
		}
		ev.Exit.Process.Credentials.CapPermitted = uint64(rv)
		return nil
	case "exit.cause":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Cause"}
		}
		ev.Exit.Cause = uint32(rv)
		return nil
	case "exit.code":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Code"}
		}
		ev.Exit.Code = uint32(rv)
		return nil
	case "exit.comm":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Comm"}
		}
		ev.Exit.Process.Comm = rv
		return nil
	case "exit.container.id":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.ContainerID"}
		}
		ev.Exit.Process.ContainerID = rv
		return nil
	case "exit.cookie":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Cookie"}
		}
		ev.Exit.Process.Cookie = uint32(rv)
		return nil
	case "exit.created_at":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.CreatedAt"}
		}
		ev.Exit.Process.CreatedAt = uint64(rv)
		return nil
	case "exit.egid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.EGID"}
		}
		ev.Exit.Process.Credentials.EGID = uint32(rv)
		return nil
	case "exit.egroup":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.EGroup"}
		}
		ev.Exit.Process.Credentials.EGroup = rv
		return nil
	case "exit.envp":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exit.Process.Envp = append(ev.Exit.Process.Envp, rv)
		case []string:
			ev.Exit.Process.Envp = append(ev.Exit.Process.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Envp"}
		}
		return nil
	case "exit.envs":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exit.Process.Envs = append(ev.Exit.Process.Envs, rv)
		case []string:
			ev.Exit.Process.Envs = append(ev.Exit.Process.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Envs"}
		}
		return nil
	case "exit.envs_truncated":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.EnvsTruncated"}
		}
		ev.Exit.Process.EnvsTruncated = rv
		return nil
	case "exit.euid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.EUID"}
		}
		ev.Exit.Process.Credentials.EUID = uint32(rv)
		return nil
	case "exit.euser":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.EUser"}
		}
		ev.Exit.Process.Credentials.EUser = rv
		return nil
	case "exit.file.change_time":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.CTime"}
		}
		ev.Exit.Process.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "exit.file.filesystem":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.Filesystem"}
		}
		ev.Exit.Process.FileEvent.Filesystem = rv
		return nil
	case "exit.file.gid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.GID"}
		}
		ev.Exit.Process.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "exit.file.group":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.Group"}
		}
		ev.Exit.Process.FileEvent.FileFields.Group = rv
		return nil
	case "exit.file.in_upper_layer":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.InUpperLayer"}
		}
		ev.Exit.Process.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "exit.file.inode":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.Inode"}
		}
		ev.Exit.Process.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "exit.file.mode":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.Mode"}
		}
		ev.Exit.Process.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "exit.file.modification_time":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.MTime"}
		}
		ev.Exit.Process.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "exit.file.mount_id":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.MountID"}
		}
		ev.Exit.Process.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "exit.file.name":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.BasenameStr"}
		}
		ev.Exit.Process.FileEvent.BasenameStr = rv
		return nil
	case "exit.file.name.length":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.file.name.length"}
	case "exit.file.path":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.PathnameStr"}
		}
		ev.Exit.Process.FileEvent.PathnameStr = rv
		return nil
	case "exit.file.path.length":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.file.path.length"}
	case "exit.file.rights":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.Mode"}
		}
		ev.Exit.Process.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "exit.file.uid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.UID"}
		}
		ev.Exit.Process.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "exit.file.user":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.User"}
		}
		ev.Exit.Process.FileEvent.FileFields.User = rv
		return nil
	case "exit.fsgid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.FSGID"}
		}
		ev.Exit.Process.Credentials.FSGID = uint32(rv)
		return nil
	case "exit.fsgroup":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.FSGroup"}
		}
		ev.Exit.Process.Credentials.FSGroup = rv
		return nil
	case "exit.fsuid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.FSUID"}
		}
		ev.Exit.Process.Credentials.FSUID = uint32(rv)
		return nil
	case "exit.fsuser":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.FSUser"}
		}
		ev.Exit.Process.Credentials.FSUser = rv
		return nil
	case "exit.gid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.GID"}
		}
		ev.Exit.Process.Credentials.GID = uint32(rv)
		return nil
	case "exit.group":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.Group"}
		}
		ev.Exit.Process.Credentials.Group = rv
		return nil
	case "exit.interpreter.file.change_time":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "exit.interpreter.file.filesystem":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.Filesystem = rv
		return nil
	case "exit.interpreter.file.gid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "exit.interpreter.file.group":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Group = rv
		return nil
	case "exit.interpreter.file.in_upper_layer":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "exit.interpreter.file.inode":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "exit.interpreter.file.mode":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "exit.interpreter.file.modification_time":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "exit.interpreter.file.mount_id":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "exit.interpreter.file.name":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.BasenameStr = rv
		return nil
	case "exit.interpreter.file.name.length":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.interpreter.file.name.length"}
	case "exit.interpreter.file.path":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.PathnameStr = rv
		return nil
	case "exit.interpreter.file.path.length":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.interpreter.file.path.length"}
	case "exit.interpreter.file.rights":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "exit.interpreter.file.uid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "exit.interpreter.file.user":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.User = rv
		return nil
	case "exit.is_kworker":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PIDContext.IsKworker"}
		}
		ev.Exit.Process.PIDContext.IsKworker = rv
		return nil
	case "exit.is_thread":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.IsThread"}
		}
		ev.Exit.Process.IsThread = rv
		return nil
	case "exit.pid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PIDContext.Pid"}
		}
		ev.Exit.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "exit.ppid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PPid"}
		}
		ev.Exit.Process.PPid = uint32(rv)
		return nil
	case "exit.tid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PIDContext.Tid"}
		}
		ev.Exit.Process.PIDContext.Tid = uint32(rv)
		return nil
	case "exit.tty_name":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.TTYName"}
		}
		ev.Exit.Process.TTYName = rv
		return nil
	case "exit.uid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.UID"}
		}
		ev.Exit.Process.Credentials.UID = uint32(rv)
		return nil
	case "exit.user":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.User"}
		}
		ev.Exit.Process.Credentials.User = rv
		return nil
	case "process.ancestors.args":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Args"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Args = rv
		return nil
	case "process.ancestors.args_flags":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Argv, rv)
		case []string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		return nil
	case "process.ancestors.args_options":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Argv, rv)
		case []string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		return nil
	case "process.ancestors.args_truncated":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.ArgsTruncated"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.ArgsTruncated = rv
		return nil
	case "process.ancestors.argv":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Argv, rv)
		case []string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		return nil
	case "process.ancestors.argv0":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv0"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Argv0 = rv
		return nil
	case "process.ancestors.cap_effective":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapEffective"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapEffective = uint64(rv)
		return nil
	case "process.ancestors.cap_permitted":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapPermitted"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapPermitted = uint64(rv)
		return nil
	case "process.ancestors.comm":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Comm"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Comm = rv
		return nil
	case "process.ancestors.container.id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.ContainerID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.ContainerID = rv
		return nil
	case "process.ancestors.cookie":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Cookie"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Cookie = uint32(rv)
		return nil
	case "process.ancestors.created_at":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.CreatedAt"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.CreatedAt = uint64(rv)
		return nil
	case "process.ancestors.egid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGID = uint32(rv)
		return nil
	case "process.ancestors.egroup":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGroup"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGroup = rv
		return nil
	case "process.ancestors.envp":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Envp = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Envp, rv)
		case []string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Envp = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Envp"}
		}
		return nil
	case "process.ancestors.envs":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Envs = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Envs, rv)
		case []string:
			ev.ProcessContext.Ancestor.ProcessContext.Process.Envs = append(ev.ProcessContext.Ancestor.ProcessContext.Process.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Envs"}
		}
		return nil
	case "process.ancestors.envs_truncated":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.EnvsTruncated"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.EnvsTruncated = rv
		return nil
	case "process.ancestors.euid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUID = uint32(rv)
		return nil
	case "process.ancestors.euser":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUser"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUser = rv
		return nil
	case "process.ancestors.file.change_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.CTime"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "process.ancestors.file.filesystem":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.Filesystem"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.Filesystem = rv
		return nil
	case "process.ancestors.file.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.GID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "process.ancestors.file.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Group"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Group = rv
		return nil
	case "process.ancestors.file.in_upper_layer":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.InUpperLayer"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "process.ancestors.file.inode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Inode"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "process.ancestors.file.mode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.ancestors.file.modification_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.MTime"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "process.ancestors.file.mount_id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.MountID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "process.ancestors.file.name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.BasenameStr"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.BasenameStr = rv
		return nil
	case "process.ancestors.file.name.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.file.name.length"}
	case "process.ancestors.file.path":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.PathnameStr"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.PathnameStr = rv
		return nil
	case "process.ancestors.file.path.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.file.path.length"}
	case "process.ancestors.file.rights":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.ancestors.file.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.UID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "process.ancestors.file.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.User"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.User = rv
		return nil
	case "process.ancestors.fsgid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGID = uint32(rv)
		return nil
	case "process.ancestors.fsgroup":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGroup"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGroup = rv
		return nil
	case "process.ancestors.fsuid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUID = uint32(rv)
		return nil
	case "process.ancestors.fsuser":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUser"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUser = rv
		return nil
	case "process.ancestors.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.GID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.GID = uint32(rv)
		return nil
	case "process.ancestors.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.Group"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.Group = rv
		return nil
	case "process.ancestors.interpreter.file.change_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "process.ancestors.interpreter.file.filesystem":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem = rv
		return nil
	case "process.ancestors.interpreter.file.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "process.ancestors.interpreter.file.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group = rv
		return nil
	case "process.ancestors.interpreter.file.in_upper_layer":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "process.ancestors.interpreter.file.inode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "process.ancestors.interpreter.file.mode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.ancestors.interpreter.file.modification_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "process.ancestors.interpreter.file.mount_id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "process.ancestors.interpreter.file.name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr = rv
		return nil
	case "process.ancestors.interpreter.file.name.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.interpreter.file.name.length"}
	case "process.ancestors.interpreter.file.path":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr = rv
		return nil
	case "process.ancestors.interpreter.file.path.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.interpreter.file.path.length"}
	case "process.ancestors.interpreter.file.rights":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.ancestors.interpreter.file.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "process.ancestors.interpreter.file.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User = rv
		return nil
	case "process.ancestors.is_kworker":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PIDContext.IsKworker"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.IsKworker = rv
		return nil
	case "process.ancestors.is_thread":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.IsThread"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.IsThread = rv
		return nil
	case "process.ancestors.pid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Pid"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "process.ancestors.ppid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PPid"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.PPid = uint32(rv)
		return nil
	case "process.ancestors.tid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Tid"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Tid = uint32(rv)
		return nil
	case "process.ancestors.tty_name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.TTYName"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.TTYName = rv
		return nil
	case "process.ancestors.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.UID"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.UID = uint32(rv)
		return nil
	case "process.ancestors.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Ancestor == nil {
			ev.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.User"}
		}
		ev.ProcessContext.Ancestor.ProcessContext.Process.Credentials.User = rv
		return nil
	case "process.args":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Args"}
		}
		ev.ProcessContext.Process.Args = rv
		return nil
	case "process.args_flags":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Process.Argv = append(ev.ProcessContext.Process.Argv, rv)
		case []string:
			ev.ProcessContext.Process.Argv = append(ev.ProcessContext.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		return nil
	case "process.args_options":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Process.Argv = append(ev.ProcessContext.Process.Argv, rv)
		case []string:
			ev.ProcessContext.Process.Argv = append(ev.ProcessContext.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		return nil
	case "process.args_truncated":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.ArgsTruncated"}
		}
		ev.ProcessContext.Process.ArgsTruncated = rv
		return nil
	case "process.argv":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Process.Argv = append(ev.ProcessContext.Process.Argv, rv)
		case []string:
			ev.ProcessContext.Process.Argv = append(ev.ProcessContext.Process.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		return nil
	case "process.argv0":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv0"}
		}
		ev.ProcessContext.Process.Argv0 = rv
		return nil
	case "process.cap_effective":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.CapEffective"}
		}
		ev.ProcessContext.Process.Credentials.CapEffective = uint64(rv)
		return nil
	case "process.cap_permitted":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.CapPermitted"}
		}
		ev.ProcessContext.Process.Credentials.CapPermitted = uint64(rv)
		return nil
	case "process.comm":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Comm"}
		}
		ev.ProcessContext.Process.Comm = rv
		return nil
	case "process.container.id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.ContainerID"}
		}
		ev.ProcessContext.Process.ContainerID = rv
		return nil
	case "process.cookie":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Cookie"}
		}
		ev.ProcessContext.Process.Cookie = uint32(rv)
		return nil
	case "process.created_at":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.CreatedAt"}
		}
		ev.ProcessContext.Process.CreatedAt = uint64(rv)
		return nil
	case "process.egid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EGID"}
		}
		ev.ProcessContext.Process.Credentials.EGID = uint32(rv)
		return nil
	case "process.egroup":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EGroup"}
		}
		ev.ProcessContext.Process.Credentials.EGroup = rv
		return nil
	case "process.envp":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Process.Envp = append(ev.ProcessContext.Process.Envp, rv)
		case []string:
			ev.ProcessContext.Process.Envp = append(ev.ProcessContext.Process.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Envp"}
		}
		return nil
	case "process.envs":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Process.Envs = append(ev.ProcessContext.Process.Envs, rv)
		case []string:
			ev.ProcessContext.Process.Envs = append(ev.ProcessContext.Process.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Envs"}
		}
		return nil
	case "process.envs_truncated":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.EnvsTruncated"}
		}
		ev.ProcessContext.Process.EnvsTruncated = rv
		return nil
	case "process.euid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EUID"}
		}
		ev.ProcessContext.Process.Credentials.EUID = uint32(rv)
		return nil
	case "process.euser":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EUser"}
		}
		ev.ProcessContext.Process.Credentials.EUser = rv
		return nil
	case "process.file.change_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.CTime"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "process.file.filesystem":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.Filesystem"}
		}
		ev.ProcessContext.Process.FileEvent.Filesystem = rv
		return nil
	case "process.file.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.GID"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "process.file.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.Group"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.Group = rv
		return nil
	case "process.file.in_upper_layer":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.InUpperLayer"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "process.file.inode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.Inode"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "process.file.mode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.file.modification_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.MTime"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "process.file.mount_id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.MountID"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "process.file.name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.BasenameStr"}
		}
		ev.ProcessContext.Process.FileEvent.BasenameStr = rv
		return nil
	case "process.file.name.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.file.name.length"}
	case "process.file.path":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.PathnameStr"}
		}
		ev.ProcessContext.Process.FileEvent.PathnameStr = rv
		return nil
	case "process.file.path.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.file.path.length"}
	case "process.file.rights":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.file.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.UID"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "process.file.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.User"}
		}
		ev.ProcessContext.Process.FileEvent.FileFields.User = rv
		return nil
	case "process.fsgid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSGID"}
		}
		ev.ProcessContext.Process.Credentials.FSGID = uint32(rv)
		return nil
	case "process.fsgroup":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSGroup"}
		}
		ev.ProcessContext.Process.Credentials.FSGroup = rv
		return nil
	case "process.fsuid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSUID"}
		}
		ev.ProcessContext.Process.Credentials.FSUID = uint32(rv)
		return nil
	case "process.fsuser":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSUser"}
		}
		ev.ProcessContext.Process.Credentials.FSUser = rv
		return nil
	case "process.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.GID"}
		}
		ev.ProcessContext.Process.Credentials.GID = uint32(rv)
		return nil
	case "process.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.Group"}
		}
		ev.ProcessContext.Process.Credentials.Group = rv
		return nil
	case "process.interpreter.file.change_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "process.interpreter.file.filesystem":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem = rv
		return nil
	case "process.interpreter.file.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "process.interpreter.file.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group = rv
		return nil
	case "process.interpreter.file.in_upper_layer":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "process.interpreter.file.inode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "process.interpreter.file.mode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.interpreter.file.modification_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "process.interpreter.file.mount_id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "process.interpreter.file.name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr = rv
		return nil
	case "process.interpreter.file.name.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.interpreter.file.name.length"}
	case "process.interpreter.file.path":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr = rv
		return nil
	case "process.interpreter.file.path.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.interpreter.file.path.length"}
	case "process.interpreter.file.rights":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.interpreter.file.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "process.interpreter.file.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User = rv
		return nil
	case "process.is_kworker":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PIDContext.IsKworker"}
		}
		ev.ProcessContext.Process.PIDContext.IsKworker = rv
		return nil
	case "process.is_thread":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.IsThread"}
		}
		ev.ProcessContext.Process.IsThread = rv
		return nil
	case "process.parent.args":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Args"}
		}
		ev.ProcessContext.Parent.Args = rv
		return nil
	case "process.parent.args_flags":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Parent.Argv = append(ev.ProcessContext.Parent.Argv, rv)
		case []string:
			ev.ProcessContext.Parent.Argv = append(ev.ProcessContext.Parent.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Argv"}
		}
		return nil
	case "process.parent.args_options":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Parent.Argv = append(ev.ProcessContext.Parent.Argv, rv)
		case []string:
			ev.ProcessContext.Parent.Argv = append(ev.ProcessContext.Parent.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Argv"}
		}
		return nil
	case "process.parent.args_truncated":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.ArgsTruncated"}
		}
		ev.ProcessContext.Parent.ArgsTruncated = rv
		return nil
	case "process.parent.argv":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Parent.Argv = append(ev.ProcessContext.Parent.Argv, rv)
		case []string:
			ev.ProcessContext.Parent.Argv = append(ev.ProcessContext.Parent.Argv, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Argv"}
		}
		return nil
	case "process.parent.argv0":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Argv0"}
		}
		ev.ProcessContext.Parent.Argv0 = rv
		return nil
	case "process.parent.cap_effective":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.CapEffective"}
		}
		ev.ProcessContext.Parent.Credentials.CapEffective = uint64(rv)
		return nil
	case "process.parent.cap_permitted":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.CapPermitted"}
		}
		ev.ProcessContext.Parent.Credentials.CapPermitted = uint64(rv)
		return nil
	case "process.parent.comm":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Comm"}
		}
		ev.ProcessContext.Parent.Comm = rv
		return nil
	case "process.parent.container.id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.ContainerID"}
		}
		ev.ProcessContext.Parent.ContainerID = rv
		return nil
	case "process.parent.cookie":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Cookie"}
		}
		ev.ProcessContext.Parent.Cookie = uint32(rv)
		return nil
	case "process.parent.created_at":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.CreatedAt"}
		}
		ev.ProcessContext.Parent.CreatedAt = uint64(rv)
		return nil
	case "process.parent.egid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.EGID"}
		}
		ev.ProcessContext.Parent.Credentials.EGID = uint32(rv)
		return nil
	case "process.parent.egroup":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.EGroup"}
		}
		ev.ProcessContext.Parent.Credentials.EGroup = rv
		return nil
	case "process.parent.envp":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Parent.Envp = append(ev.ProcessContext.Parent.Envp, rv)
		case []string:
			ev.ProcessContext.Parent.Envp = append(ev.ProcessContext.Parent.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Envp"}
		}
		return nil
	case "process.parent.envs":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.ProcessContext.Parent.Envs = append(ev.ProcessContext.Parent.Envs, rv)
		case []string:
			ev.ProcessContext.Parent.Envs = append(ev.ProcessContext.Parent.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Envs"}
		}
		return nil
	case "process.parent.envs_truncated":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.EnvsTruncated"}
		}
		ev.ProcessContext.Parent.EnvsTruncated = rv
		return nil
	case "process.parent.euid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.EUID"}
		}
		ev.ProcessContext.Parent.Credentials.EUID = uint32(rv)
		return nil
	case "process.parent.euser":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.EUser"}
		}
		ev.ProcessContext.Parent.Credentials.EUser = rv
		return nil
	case "process.parent.file.change_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.CTime"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "process.parent.file.filesystem":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.Filesystem"}
		}
		ev.ProcessContext.Parent.FileEvent.Filesystem = rv
		return nil
	case "process.parent.file.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.GID"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "process.parent.file.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.Group"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.Group = rv
		return nil
	case "process.parent.file.in_upper_layer":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.InUpperLayer"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "process.parent.file.inode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.Inode"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "process.parent.file.mode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.parent.file.modification_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.MTime"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "process.parent.file.mount_id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.MountID"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "process.parent.file.name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.BasenameStr"}
		}
		ev.ProcessContext.Parent.FileEvent.BasenameStr = rv
		return nil
	case "process.parent.file.name.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.parent.file.name.length"}
	case "process.parent.file.path":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.PathnameStr"}
		}
		ev.ProcessContext.Parent.FileEvent.PathnameStr = rv
		return nil
	case "process.parent.file.path.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.parent.file.path.length"}
	case "process.parent.file.rights":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.parent.file.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.UID"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "process.parent.file.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.FileEvent.FileFields.User"}
		}
		ev.ProcessContext.Parent.FileEvent.FileFields.User = rv
		return nil
	case "process.parent.fsgid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.FSGID"}
		}
		ev.ProcessContext.Parent.Credentials.FSGID = uint32(rv)
		return nil
	case "process.parent.fsgroup":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.FSGroup"}
		}
		ev.ProcessContext.Parent.Credentials.FSGroup = rv
		return nil
	case "process.parent.fsuid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.FSUID"}
		}
		ev.ProcessContext.Parent.Credentials.FSUID = uint32(rv)
		return nil
	case "process.parent.fsuser":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.FSUser"}
		}
		ev.ProcessContext.Parent.Credentials.FSUser = rv
		return nil
	case "process.parent.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.GID"}
		}
		ev.ProcessContext.Parent.Credentials.GID = uint32(rv)
		return nil
	case "process.parent.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.Group"}
		}
		ev.ProcessContext.Parent.Credentials.Group = rv
		return nil
	case "process.parent.interpreter.file.change_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.CTime = uint64(rv)
		return nil
	case "process.parent.interpreter.file.filesystem":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.Filesystem"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.Filesystem = rv
		return nil
	case "process.parent.interpreter.file.gid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.GID = uint32(rv)
		return nil
	case "process.parent.interpreter.file.group":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Group = rv
		return nil
	case "process.parent.interpreter.file.in_upper_layer":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.InUpperLayer = rv
		return nil
	case "process.parent.interpreter.file.inode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Inode = uint64(rv)
		return nil
	case "process.parent.interpreter.file.mode":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.parent.interpreter.file.modification_time":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MTime = uint64(rv)
		return nil
	case "process.parent.interpreter.file.mount_id":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MountID = uint32(rv)
		return nil
	case "process.parent.interpreter.file.name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.BasenameStr"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.BasenameStr = rv
		return nil
	case "process.parent.interpreter.file.name.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.parent.interpreter.file.name.length"}
	case "process.parent.interpreter.file.path":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.PathnameStr"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.PathnameStr = rv
		return nil
	case "process.parent.interpreter.file.path.length":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.parent.interpreter.file.path.length"}
	case "process.parent.interpreter.file.rights":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode = uint16(rv)
		return nil
	case "process.parent.interpreter.file.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.UID = uint32(rv)
		return nil
	case "process.parent.interpreter.file.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.User"}
		}
		ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.User = rv
		return nil
	case "process.parent.is_kworker":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.PIDContext.IsKworker"}
		}
		ev.ProcessContext.Parent.PIDContext.IsKworker = rv
		return nil
	case "process.parent.is_thread":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(bool)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.IsThread"}
		}
		ev.ProcessContext.Parent.IsThread = rv
		return nil
	case "process.parent.pid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.PIDContext.Pid"}
		}
		ev.ProcessContext.Parent.PIDContext.Pid = uint32(rv)
		return nil
	case "process.parent.ppid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.PPid"}
		}
		ev.ProcessContext.Parent.PPid = uint32(rv)
		return nil
	case "process.parent.tid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.PIDContext.Tid"}
		}
		ev.ProcessContext.Parent.PIDContext.Tid = uint32(rv)
		return nil
	case "process.parent.tty_name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.TTYName"}
		}
		ev.ProcessContext.Parent.TTYName = rv
		return nil
	case "process.parent.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.UID"}
		}
		ev.ProcessContext.Parent.Credentials.UID = uint32(rv)
		return nil
	case "process.parent.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		if ev.ProcessContext.Parent == nil {
			ev.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Parent.Credentials.User"}
		}
		ev.ProcessContext.Parent.Credentials.User = rv
		return nil
	case "process.pid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PIDContext.Pid"}
		}
		ev.ProcessContext.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "process.ppid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PPid"}
		}
		ev.ProcessContext.Process.PPid = uint32(rv)
		return nil
	case "process.tid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PIDContext.Tid"}
		}
		ev.ProcessContext.Process.PIDContext.Tid = uint32(rv)
		return nil
	case "process.tty_name":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.TTYName"}
		}
		ev.ProcessContext.Process.TTYName = rv
		return nil
	case "process.uid":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.UID"}
		}
		ev.ProcessContext.Process.Credentials.UID = uint32(rv)
		return nil
	case "process.user":
		if ev.ProcessContext == nil {
			ev.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.User"}
		}
		ev.ProcessContext.Process.Credentials.User = rv
		return nil
	}
	return &eval.ErrFieldNotFound{Field: field}
}
