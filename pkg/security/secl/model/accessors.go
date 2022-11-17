// Code generated - DO NOT EDIT.
package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"net"
	"reflect"
	"unsafe"
)

// suppress unused package warning
var (
	_ *unsafe.Pointer
)

func (m *Model) GetIterator(field eval.Field) (eval.Iterator, error) {
	switch field {
	case "process.ancestors":
		return &ProcessAncestorsIterator{}, nil
	case "ptrace.tracee.ancestors":
		return &ProcessAncestorsIterator{}, nil
	case "signal.target.ancestors":
		return &ProcessAncestorsIterator{}, nil
	}
	return nil, &eval.ErrIteratorNotSupported{Field: field}
}
func (m *Model) GetEventTypes() []eval.EventType {
	return []eval.EventType{
		eval.EventType("bind"),
		eval.EventType("bpf"),
		eval.EventType("capset"),
		eval.EventType("chmod"),
		eval.EventType("chown"),
		eval.EventType("dns"),
		eval.EventType("exec"),
		eval.EventType("exit"),
		eval.EventType("link"),
		eval.EventType("load_module"),
		eval.EventType("mkdir"),
		eval.EventType("mmap"),
		eval.EventType("mprotect"),
		eval.EventType("open"),
		eval.EventType("ptrace"),
		eval.EventType("removexattr"),
		eval.EventType("rename"),
		eval.EventType("rmdir"),
		eval.EventType("selinux"),
		eval.EventType("setgid"),
		eval.EventType("setuid"),
		eval.EventType("setxattr"),
		eval.EventType("signal"),
		eval.EventType("splice"),
		eval.EventType("unlink"),
		eval.EventType("unload_module"),
		eval.EventType("utimes"),
	}
}
func (m *Model) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {
	case "async":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Async
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bind.addr.family":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Bind.AddrFamily)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bind.addr.ip":
		return &eval.CIDREvaluator{
			EvalFnc: func(ctx *eval.Context) net.IPNet {
				return (*Event)(ctx.Object).Bind.Addr.IPNet
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bind.addr.port":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Bind.Addr.Port)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bind.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Bind.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bpf.cmd":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).BPF.Cmd)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bpf.map.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).BPF.Map.Name
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bpf.map.type":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).BPF.Map.Type)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bpf.prog.attach_type":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).BPF.Program.AttachType)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bpf.prog.helpers":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				result := make([]int, len((*Event)(ctx.Object).BPF.Program.Helpers))
				for i, v := range (*Event)(ctx.Object).BPF.Program.Helpers {
					result[i] = int(v)
				}
				return result
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "bpf.prog.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).BPF.Program.Name
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bpf.prog.tag":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).BPF.Program.Tag
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bpf.prog.type":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).BPF.Program.Type)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "bpf.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).BPF.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "capset.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Capset.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "capset.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Capset.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.destination.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chmod.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chmod.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Chmod.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chmod.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Chmod.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chmod.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Chmod.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chmod.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chmod.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chmod.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chmod.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.destination.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.destination.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.destination.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.destination.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Chown.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Chown.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Chown.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "chown.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "chown.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Chown.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ContainerContext.ID
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).ContainerContext.Tags
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
		}, nil
	case "dns.id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).DNS.ID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "dns.question.class":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).DNS.Class)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "dns.question.count":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).DNS.Count)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "dns.question.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).DNS.Size)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "dns.question.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.DNSNameCmp,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).DNS.Name
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "dns.question.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.DNSNameCmp,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).DNS.Name)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "dns.question.type":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).DNS.Type)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Args
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exec.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exec.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exec.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exec.Process.ArgsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exec.Process.Argv
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exec.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Argv0
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exec.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.CreatedAt)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exec.Process.Envp
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exec.Process.Envs
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exec.Process.EnvsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Exec.Process.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Exec.Process.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.LinuxBinprm.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exec.Process.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exec.Process.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exec.Process.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exec.Process.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Args
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exit.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exit.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exit.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exit.Process.ArgsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exit.Process.Argv
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exit.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Argv0
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exit.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.cause":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Cause)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.code":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Code)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.CreatedAt)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exit.Process.Envp
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Exit.Process.Envs
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exit.Process.EnvsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Exit.Process.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Exit.Process.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.LinuxBinprm.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exit.Process.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Exit.Process.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Exit.Process.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Exit.Process.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Source.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.destination.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Target.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.destination.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Target.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.destination.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Target.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.destination.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Target.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.destination.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Link.Target.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.destination.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Target.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Target.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.destination.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Target.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.destination.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Target.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.destination.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Target.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.destination.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Link.Target.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.destination.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Target.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.destination.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Link.Target.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.destination.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Target.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.destination.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Target.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.destination.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Target.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Source.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Source.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Source.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Link.Source.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Source.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Source.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Source.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Source.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Source.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Link.Source.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Source.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Link.Source.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Source.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.Source.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "link.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Source.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "link.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Link.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).LoadModule.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).LoadModule.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).LoadModule.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).LoadModule.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).LoadModule.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).LoadModule.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).LoadModule.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).LoadModule.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "load_module.loaded_from_memory":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).LoadModule.LoadedFromMemory
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).LoadModule.Name
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "load_module.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).LoadModule.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.destination.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Mkdir.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Mkdir.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Mkdir.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Mkdir.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Mkdir.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Mkdir.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Mkdir.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mkdir.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Mkdir.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mkdir.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Mkdir.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).MMap.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).MMap.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).MMap.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).MMap.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).MMap.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).MMap.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).MMap.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).MMap.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "mmap.flags":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return (*Event)(ctx.Object).MMap.Flags
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.protection":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return (*Event)(ctx.Object).MMap.Protection
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mmap.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MMap.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mprotect.req_protection":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return (*Event)(ctx.Object).MProtect.ReqProtection
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mprotect.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).MProtect.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "mprotect.vm_protection":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return (*Event)(ctx.Object).MProtect.VMProtection
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.destination.ip":
		return &eval.CIDREvaluator{
			EvalFnc: func(ctx *eval.Context) net.IPNet {
				return (*Event)(ctx.Object).NetworkContext.Destination.IPNet
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.destination.port":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).NetworkContext.Destination.Port)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.device.ifindex":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).NetworkContext.Device.IfIndex)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.device.ifname":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).NetworkContext.Device.IfName
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "network.l3_protocol":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).NetworkContext.L3Protocol)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.l4_protocol":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).NetworkContext.L4Protocol)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.size":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).NetworkContext.Size)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.source.ip":
		return &eval.CIDREvaluator{
			EvalFnc: func(ctx *eval.Context) net.IPNet {
				return (*Event)(ctx.Object).NetworkContext.Source.IPNet
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.source.port":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).NetworkContext.Source.Port)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Open.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Open.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Open.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Open.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Open.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Open.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Open.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Open.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "open.flags":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.Flags)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Open.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.ancestors.args":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Args
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.args_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.ArgsTruncated
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.argv0":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv0
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.cap_effective":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.CapEffective)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.cap_permitted":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.CapPermitted)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.comm":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Comm
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.ContainerID
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.cookie":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Cookie)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.CreatedAt)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.egid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.EGID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.egroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.EGroup
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Envp
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Envs
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.envs_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.EnvsTruncated
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.euid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.EUID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.euser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.EUser
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.Filesystem
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.InUpperLayer
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Inode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.MountID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.BasenameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.FileEvent.BasenameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.PathnameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.FileEvent.PathnameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.fsgid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.FSGID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.fsgroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.FSGroup
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.fsuid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.FSUID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.fsuser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.FSUser
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.interpreter.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.is_kworker":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.PIDContext.IsKworker
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.is_thread":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.IsThread
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PIDContext.Pid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PPid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.tid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PIDContext.Tid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.tty_name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.TTYName
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Args
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).ProcessContext.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).ProcessContext.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).ProcessContext.Process.ArgsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).ProcessContext.Process.Argv
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Argv0
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.CreatedAt)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).ProcessContext.Process.Envp
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).ProcessContext.Process.Envs
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).ProcessContext.Process.EnvsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).ProcessContext.Process.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).ProcessContext.Process.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).ProcessContext.Process.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).ProcessContext.Process.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).ProcessContext.Process.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.request":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Request)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.ancestors.args":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Args
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.args_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.ArgsTruncated
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.argv0":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv0
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.cap_effective":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.CapEffective)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.cap_permitted":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.CapPermitted)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.comm":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Comm
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.ContainerID
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.cookie":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Cookie)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.CreatedAt)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.egid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.EGID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.egroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.EGroup
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Envp
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Envs
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.envs_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.EnvsTruncated
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.euid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.EUID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.euser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.EUser
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.Filesystem
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.InUpperLayer
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Inode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.MountID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.BasenameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.FileEvent.BasenameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.PathnameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.FileEvent.PathnameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.fsgid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.FSGID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.fsgroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.FSGroup
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.fsuid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.FSUID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.fsuser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.FSUser
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.interpreter.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.is_kworker":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.PIDContext.IsKworker
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.is_thread":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.IsThread
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PIDContext.Pid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PPid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.tid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PIDContext.Tid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.tty_name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.TTYName
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "ptrace.tracee.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Args
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.ArgsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Argv
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Argv0
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.CreatedAt)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Envp
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Envs
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.EnvsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "ptrace.tracee.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "ptrace.tracee.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).PTrace.Tracee.Process.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "removexattr.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "removexattr.file.destination.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.Name
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.destination.namespace":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.Namespace
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "removexattr.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).RemoveXAttr.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "removexattr.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "removexattr.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "removexattr.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "removexattr.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).RemoveXAttr.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).RemoveXAttr.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "removexattr.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "removexattr.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).RemoveXAttr.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.Old.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.New.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.New.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.New.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.New.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Rename.New.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.New.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.New.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.New.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.New.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.New.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Rename.New.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.New.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Rename.New.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.New.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.New.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.New.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.Old.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.Old.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.Old.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Rename.Old.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.Old.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.Old.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.Old.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.Old.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.Old.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Rename.Old.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.Old.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Rename.Old.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.Old.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.Old.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.Old.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rename.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rmdir.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rmdir.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rmdir.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rmdir.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rmdir.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Rmdir.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rmdir.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rmdir.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rmdir.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rmdir.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rmdir.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Rmdir.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rmdir.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Rmdir.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rmdir.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rmdir.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rmdir.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Rmdir.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "selinux.bool.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SELinux.BoolName
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "selinux.bool.state":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SELinux.BoolChangeValue
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "selinux.bool_commit.state":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).SELinux.BoolCommitValue
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "selinux.enforce.status":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SELinux.EnforceStatus
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setgid.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetGID.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setgid.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetGID.EGroup
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setgid.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetGID.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setgid.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetGID.FSGroup
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setgid.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetGID.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setgid.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetGID.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setuid.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetUID.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setuid.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetUID.EUser
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setuid.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetUID.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setuid.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetUID.FSUser
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setuid.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetUID.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setuid.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetUID.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setxattr.file.destination.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.Name
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.destination.namespace":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.Namespace
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setxattr.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).SetXAttr.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setxattr.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setxattr.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setxattr.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setxattr.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).SetXAttr.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).SetXAttr.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "setxattr.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "setxattr.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).SetXAttr.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.PID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.ancestors.args":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Args
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.args_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.ArgsTruncated
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.argv0":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Argv0
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.cap_effective":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.CapEffective)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.cap_permitted":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.CapPermitted)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.comm":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Comm
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.ContainerID
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.cookie":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Cookie)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.CreatedAt)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.egid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.EGID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.egroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.EGroup
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Envp
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Envs
					results = append(results, result...)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.envs_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.EnvsTruncated
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.euid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.EUID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.euser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.EUser
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.Filesystem
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.InUpperLayer
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Inode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.MountID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.BasenameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.FileEvent.BasenameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.PathnameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.FileEvent.PathnameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.FileEvent.FileFields.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.fsgid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.FSGID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.fsgroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.FSGroup
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.fsuid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.FSUID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.fsuser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.FSUser
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.interpreter.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.is_kworker":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.PIDContext.IsKworker
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.is_thread":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				var results []bool
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.IsThread
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PIDContext.Pid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PPid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.tid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.PIDContext.Tid)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.tty_name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.TTYName
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := int(element.ProcessContext.Process.Credentials.UID)
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := element.ProcessContext.Process.Credentials.User
					results = append(results, result)
					value = iterator.Next()
				}
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "signal.target.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Args
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "signal.target.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Signal.Target.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Signal.Target.Process.Argv
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Signal.Target.Process.ArgsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Signal.Target.Process.Argv
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "signal.target.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Argv0
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "signal.target.cap_effective":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Credentials.CapEffective)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.cap_permitted":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Credentials.CapPermitted)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.comm":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Comm
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.cookie":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Cookie)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.CreatedAt)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.egid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Credentials.EGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.egroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Credentials.EGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Signal.Target.Process.Envp
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return (*Event)(ctx.Object).Signal.Target.Process.Envs
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Signal.Target.Process.EnvsTruncated
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.euid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Credentials.EUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.euser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Credentials.EUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Signal.Target.Process.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Signal.Target.Process.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.fsgid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Credentials.FSGID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.fsgroup":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Credentials.FSGroup
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.fsuid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Credentials.FSUID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.fsuser":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Credentials.FSUser
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Credentials.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Credentials.Group
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.interpreter.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.interpreter.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.interpreter.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.interpreter.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.interpreter.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.interpreter.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.interpreter.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.interpreter.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.interpreter.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.interpreter.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.interpreter.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.interpreter.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.interpreter.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.interpreter.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.interpreter.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.interpreter.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "signal.target.is_kworker":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Signal.Target.Process.PIDContext.IsKworker
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.is_thread":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Signal.Target.Process.IsThread
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.tid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.PIDContext.Tid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.tty_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.TTYName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Target.Process.Credentials.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.target.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Signal.Target.Process.Credentials.User
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "signal.type":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Signal.Type)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Splice.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Splice.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Splice.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Splice.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Splice.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Splice.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Splice.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Splice.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "splice.pipe_entry_flag":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.PipeEntryFlag)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.pipe_exit_flag":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.PipeExitFlag)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "splice.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Splice.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Unlink.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Unlink.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Unlink.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Unlink.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Unlink.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Unlink.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Unlink.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Unlink.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "unlink.flags":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.Flags)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unlink.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Unlink.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unload_module.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).UnloadModule.Name
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "unload_module.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).UnloadModule.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "utimes.file.change_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.File.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "utimes.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Utimes.File.Filesystem
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.File.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "utimes.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Utimes.File.FileFields.Group
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return (*Event)(ctx.Object).Utimes.File.FileFields.InUpperLayer
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.File.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "utimes.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "utimes.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.File.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "utimes.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.File.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "utimes.file.name":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Utimes.File.BasenameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkBasename,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Utimes.File.BasenameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.file.path":
		return &eval.StringEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Utimes.File.PathnameStr
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: ProcessSymlinkPathname,
			EvalFnc: func(ctx *eval.Context) int {
				return len((*Event)(ctx.Object).Utimes.File.PathnameStr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.File.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.File.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "utimes.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Utimes.File.FileFields.User
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "utimes.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).Utimes.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	}
	return nil, &eval.ErrFieldNotFound{Field: field}
}
func (e *Event) GetFields() []eval.Field {
	return []eval.Field{
		"async",
		"bind.addr.family",
		"bind.addr.ip",
		"bind.addr.port",
		"bind.retval",
		"bpf.cmd",
		"bpf.map.name",
		"bpf.map.type",
		"bpf.prog.attach_type",
		"bpf.prog.helpers",
		"bpf.prog.name",
		"bpf.prog.tag",
		"bpf.prog.type",
		"bpf.retval",
		"capset.cap_effective",
		"capset.cap_permitted",
		"chmod.file.change_time",
		"chmod.file.destination.mode",
		"chmod.file.destination.rights",
		"chmod.file.filesystem",
		"chmod.file.gid",
		"chmod.file.group",
		"chmod.file.in_upper_layer",
		"chmod.file.inode",
		"chmod.file.mode",
		"chmod.file.modification_time",
		"chmod.file.mount_id",
		"chmod.file.name",
		"chmod.file.name.length",
		"chmod.file.path",
		"chmod.file.path.length",
		"chmod.file.rights",
		"chmod.file.uid",
		"chmod.file.user",
		"chmod.retval",
		"chown.file.change_time",
		"chown.file.destination.gid",
		"chown.file.destination.group",
		"chown.file.destination.uid",
		"chown.file.destination.user",
		"chown.file.filesystem",
		"chown.file.gid",
		"chown.file.group",
		"chown.file.in_upper_layer",
		"chown.file.inode",
		"chown.file.mode",
		"chown.file.modification_time",
		"chown.file.mount_id",
		"chown.file.name",
		"chown.file.name.length",
		"chown.file.path",
		"chown.file.path.length",
		"chown.file.rights",
		"chown.file.uid",
		"chown.file.user",
		"chown.retval",
		"container.id",
		"container.tags",
		"dns.id",
		"dns.question.class",
		"dns.question.count",
		"dns.question.length",
		"dns.question.name",
		"dns.question.name.length",
		"dns.question.type",
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
		"link.file.change_time",
		"link.file.destination.change_time",
		"link.file.destination.filesystem",
		"link.file.destination.gid",
		"link.file.destination.group",
		"link.file.destination.in_upper_layer",
		"link.file.destination.inode",
		"link.file.destination.mode",
		"link.file.destination.modification_time",
		"link.file.destination.mount_id",
		"link.file.destination.name",
		"link.file.destination.name.length",
		"link.file.destination.path",
		"link.file.destination.path.length",
		"link.file.destination.rights",
		"link.file.destination.uid",
		"link.file.destination.user",
		"link.file.filesystem",
		"link.file.gid",
		"link.file.group",
		"link.file.in_upper_layer",
		"link.file.inode",
		"link.file.mode",
		"link.file.modification_time",
		"link.file.mount_id",
		"link.file.name",
		"link.file.name.length",
		"link.file.path",
		"link.file.path.length",
		"link.file.rights",
		"link.file.uid",
		"link.file.user",
		"link.retval",
		"load_module.file.change_time",
		"load_module.file.filesystem",
		"load_module.file.gid",
		"load_module.file.group",
		"load_module.file.in_upper_layer",
		"load_module.file.inode",
		"load_module.file.mode",
		"load_module.file.modification_time",
		"load_module.file.mount_id",
		"load_module.file.name",
		"load_module.file.name.length",
		"load_module.file.path",
		"load_module.file.path.length",
		"load_module.file.rights",
		"load_module.file.uid",
		"load_module.file.user",
		"load_module.loaded_from_memory",
		"load_module.name",
		"load_module.retval",
		"mkdir.file.change_time",
		"mkdir.file.destination.mode",
		"mkdir.file.destination.rights",
		"mkdir.file.filesystem",
		"mkdir.file.gid",
		"mkdir.file.group",
		"mkdir.file.in_upper_layer",
		"mkdir.file.inode",
		"mkdir.file.mode",
		"mkdir.file.modification_time",
		"mkdir.file.mount_id",
		"mkdir.file.name",
		"mkdir.file.name.length",
		"mkdir.file.path",
		"mkdir.file.path.length",
		"mkdir.file.rights",
		"mkdir.file.uid",
		"mkdir.file.user",
		"mkdir.retval",
		"mmap.file.change_time",
		"mmap.file.filesystem",
		"mmap.file.gid",
		"mmap.file.group",
		"mmap.file.in_upper_layer",
		"mmap.file.inode",
		"mmap.file.mode",
		"mmap.file.modification_time",
		"mmap.file.mount_id",
		"mmap.file.name",
		"mmap.file.name.length",
		"mmap.file.path",
		"mmap.file.path.length",
		"mmap.file.rights",
		"mmap.file.uid",
		"mmap.file.user",
		"mmap.flags",
		"mmap.protection",
		"mmap.retval",
		"mprotect.req_protection",
		"mprotect.retval",
		"mprotect.vm_protection",
		"network.destination.ip",
		"network.destination.port",
		"network.device.ifindex",
		"network.device.ifname",
		"network.l3_protocol",
		"network.l4_protocol",
		"network.size",
		"network.source.ip",
		"network.source.port",
		"open.file.change_time",
		"open.file.destination.mode",
		"open.file.filesystem",
		"open.file.gid",
		"open.file.group",
		"open.file.in_upper_layer",
		"open.file.inode",
		"open.file.mode",
		"open.file.modification_time",
		"open.file.mount_id",
		"open.file.name",
		"open.file.name.length",
		"open.file.path",
		"open.file.path.length",
		"open.file.rights",
		"open.file.uid",
		"open.file.user",
		"open.flags",
		"open.retval",
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
		"process.pid",
		"process.ppid",
		"process.tid",
		"process.tty_name",
		"process.uid",
		"process.user",
		"ptrace.request",
		"ptrace.retval",
		"ptrace.tracee.ancestors.args",
		"ptrace.tracee.ancestors.args_flags",
		"ptrace.tracee.ancestors.args_options",
		"ptrace.tracee.ancestors.args_truncated",
		"ptrace.tracee.ancestors.argv",
		"ptrace.tracee.ancestors.argv0",
		"ptrace.tracee.ancestors.cap_effective",
		"ptrace.tracee.ancestors.cap_permitted",
		"ptrace.tracee.ancestors.comm",
		"ptrace.tracee.ancestors.container.id",
		"ptrace.tracee.ancestors.cookie",
		"ptrace.tracee.ancestors.created_at",
		"ptrace.tracee.ancestors.egid",
		"ptrace.tracee.ancestors.egroup",
		"ptrace.tracee.ancestors.envp",
		"ptrace.tracee.ancestors.envs",
		"ptrace.tracee.ancestors.envs_truncated",
		"ptrace.tracee.ancestors.euid",
		"ptrace.tracee.ancestors.euser",
		"ptrace.tracee.ancestors.file.change_time",
		"ptrace.tracee.ancestors.file.filesystem",
		"ptrace.tracee.ancestors.file.gid",
		"ptrace.tracee.ancestors.file.group",
		"ptrace.tracee.ancestors.file.in_upper_layer",
		"ptrace.tracee.ancestors.file.inode",
		"ptrace.tracee.ancestors.file.mode",
		"ptrace.tracee.ancestors.file.modification_time",
		"ptrace.tracee.ancestors.file.mount_id",
		"ptrace.tracee.ancestors.file.name",
		"ptrace.tracee.ancestors.file.name.length",
		"ptrace.tracee.ancestors.file.path",
		"ptrace.tracee.ancestors.file.path.length",
		"ptrace.tracee.ancestors.file.rights",
		"ptrace.tracee.ancestors.file.uid",
		"ptrace.tracee.ancestors.file.user",
		"ptrace.tracee.ancestors.fsgid",
		"ptrace.tracee.ancestors.fsgroup",
		"ptrace.tracee.ancestors.fsuid",
		"ptrace.tracee.ancestors.fsuser",
		"ptrace.tracee.ancestors.gid",
		"ptrace.tracee.ancestors.group",
		"ptrace.tracee.ancestors.interpreter.file.change_time",
		"ptrace.tracee.ancestors.interpreter.file.filesystem",
		"ptrace.tracee.ancestors.interpreter.file.gid",
		"ptrace.tracee.ancestors.interpreter.file.group",
		"ptrace.tracee.ancestors.interpreter.file.in_upper_layer",
		"ptrace.tracee.ancestors.interpreter.file.inode",
		"ptrace.tracee.ancestors.interpreter.file.mode",
		"ptrace.tracee.ancestors.interpreter.file.modification_time",
		"ptrace.tracee.ancestors.interpreter.file.mount_id",
		"ptrace.tracee.ancestors.interpreter.file.name",
		"ptrace.tracee.ancestors.interpreter.file.name.length",
		"ptrace.tracee.ancestors.interpreter.file.path",
		"ptrace.tracee.ancestors.interpreter.file.path.length",
		"ptrace.tracee.ancestors.interpreter.file.rights",
		"ptrace.tracee.ancestors.interpreter.file.uid",
		"ptrace.tracee.ancestors.interpreter.file.user",
		"ptrace.tracee.ancestors.is_kworker",
		"ptrace.tracee.ancestors.is_thread",
		"ptrace.tracee.ancestors.pid",
		"ptrace.tracee.ancestors.ppid",
		"ptrace.tracee.ancestors.tid",
		"ptrace.tracee.ancestors.tty_name",
		"ptrace.tracee.ancestors.uid",
		"ptrace.tracee.ancestors.user",
		"ptrace.tracee.args",
		"ptrace.tracee.args_flags",
		"ptrace.tracee.args_options",
		"ptrace.tracee.args_truncated",
		"ptrace.tracee.argv",
		"ptrace.tracee.argv0",
		"ptrace.tracee.cap_effective",
		"ptrace.tracee.cap_permitted",
		"ptrace.tracee.comm",
		"ptrace.tracee.container.id",
		"ptrace.tracee.cookie",
		"ptrace.tracee.created_at",
		"ptrace.tracee.egid",
		"ptrace.tracee.egroup",
		"ptrace.tracee.envp",
		"ptrace.tracee.envs",
		"ptrace.tracee.envs_truncated",
		"ptrace.tracee.euid",
		"ptrace.tracee.euser",
		"ptrace.tracee.file.change_time",
		"ptrace.tracee.file.filesystem",
		"ptrace.tracee.file.gid",
		"ptrace.tracee.file.group",
		"ptrace.tracee.file.in_upper_layer",
		"ptrace.tracee.file.inode",
		"ptrace.tracee.file.mode",
		"ptrace.tracee.file.modification_time",
		"ptrace.tracee.file.mount_id",
		"ptrace.tracee.file.name",
		"ptrace.tracee.file.name.length",
		"ptrace.tracee.file.path",
		"ptrace.tracee.file.path.length",
		"ptrace.tracee.file.rights",
		"ptrace.tracee.file.uid",
		"ptrace.tracee.file.user",
		"ptrace.tracee.fsgid",
		"ptrace.tracee.fsgroup",
		"ptrace.tracee.fsuid",
		"ptrace.tracee.fsuser",
		"ptrace.tracee.gid",
		"ptrace.tracee.group",
		"ptrace.tracee.interpreter.file.change_time",
		"ptrace.tracee.interpreter.file.filesystem",
		"ptrace.tracee.interpreter.file.gid",
		"ptrace.tracee.interpreter.file.group",
		"ptrace.tracee.interpreter.file.in_upper_layer",
		"ptrace.tracee.interpreter.file.inode",
		"ptrace.tracee.interpreter.file.mode",
		"ptrace.tracee.interpreter.file.modification_time",
		"ptrace.tracee.interpreter.file.mount_id",
		"ptrace.tracee.interpreter.file.name",
		"ptrace.tracee.interpreter.file.name.length",
		"ptrace.tracee.interpreter.file.path",
		"ptrace.tracee.interpreter.file.path.length",
		"ptrace.tracee.interpreter.file.rights",
		"ptrace.tracee.interpreter.file.uid",
		"ptrace.tracee.interpreter.file.user",
		"ptrace.tracee.is_kworker",
		"ptrace.tracee.is_thread",
		"ptrace.tracee.pid",
		"ptrace.tracee.ppid",
		"ptrace.tracee.tid",
		"ptrace.tracee.tty_name",
		"ptrace.tracee.uid",
		"ptrace.tracee.user",
		"removexattr.file.change_time",
		"removexattr.file.destination.name",
		"removexattr.file.destination.namespace",
		"removexattr.file.filesystem",
		"removexattr.file.gid",
		"removexattr.file.group",
		"removexattr.file.in_upper_layer",
		"removexattr.file.inode",
		"removexattr.file.mode",
		"removexattr.file.modification_time",
		"removexattr.file.mount_id",
		"removexattr.file.name",
		"removexattr.file.name.length",
		"removexattr.file.path",
		"removexattr.file.path.length",
		"removexattr.file.rights",
		"removexattr.file.uid",
		"removexattr.file.user",
		"removexattr.retval",
		"rename.file.change_time",
		"rename.file.destination.change_time",
		"rename.file.destination.filesystem",
		"rename.file.destination.gid",
		"rename.file.destination.group",
		"rename.file.destination.in_upper_layer",
		"rename.file.destination.inode",
		"rename.file.destination.mode",
		"rename.file.destination.modification_time",
		"rename.file.destination.mount_id",
		"rename.file.destination.name",
		"rename.file.destination.name.length",
		"rename.file.destination.path",
		"rename.file.destination.path.length",
		"rename.file.destination.rights",
		"rename.file.destination.uid",
		"rename.file.destination.user",
		"rename.file.filesystem",
		"rename.file.gid",
		"rename.file.group",
		"rename.file.in_upper_layer",
		"rename.file.inode",
		"rename.file.mode",
		"rename.file.modification_time",
		"rename.file.mount_id",
		"rename.file.name",
		"rename.file.name.length",
		"rename.file.path",
		"rename.file.path.length",
		"rename.file.rights",
		"rename.file.uid",
		"rename.file.user",
		"rename.retval",
		"rmdir.file.change_time",
		"rmdir.file.filesystem",
		"rmdir.file.gid",
		"rmdir.file.group",
		"rmdir.file.in_upper_layer",
		"rmdir.file.inode",
		"rmdir.file.mode",
		"rmdir.file.modification_time",
		"rmdir.file.mount_id",
		"rmdir.file.name",
		"rmdir.file.name.length",
		"rmdir.file.path",
		"rmdir.file.path.length",
		"rmdir.file.rights",
		"rmdir.file.uid",
		"rmdir.file.user",
		"rmdir.retval",
		"selinux.bool.name",
		"selinux.bool.state",
		"selinux.bool_commit.state",
		"selinux.enforce.status",
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
		"setxattr.file.change_time",
		"setxattr.file.destination.name",
		"setxattr.file.destination.namespace",
		"setxattr.file.filesystem",
		"setxattr.file.gid",
		"setxattr.file.group",
		"setxattr.file.in_upper_layer",
		"setxattr.file.inode",
		"setxattr.file.mode",
		"setxattr.file.modification_time",
		"setxattr.file.mount_id",
		"setxattr.file.name",
		"setxattr.file.name.length",
		"setxattr.file.path",
		"setxattr.file.path.length",
		"setxattr.file.rights",
		"setxattr.file.uid",
		"setxattr.file.user",
		"setxattr.retval",
		"signal.pid",
		"signal.retval",
		"signal.target.ancestors.args",
		"signal.target.ancestors.args_flags",
		"signal.target.ancestors.args_options",
		"signal.target.ancestors.args_truncated",
		"signal.target.ancestors.argv",
		"signal.target.ancestors.argv0",
		"signal.target.ancestors.cap_effective",
		"signal.target.ancestors.cap_permitted",
		"signal.target.ancestors.comm",
		"signal.target.ancestors.container.id",
		"signal.target.ancestors.cookie",
		"signal.target.ancestors.created_at",
		"signal.target.ancestors.egid",
		"signal.target.ancestors.egroup",
		"signal.target.ancestors.envp",
		"signal.target.ancestors.envs",
		"signal.target.ancestors.envs_truncated",
		"signal.target.ancestors.euid",
		"signal.target.ancestors.euser",
		"signal.target.ancestors.file.change_time",
		"signal.target.ancestors.file.filesystem",
		"signal.target.ancestors.file.gid",
		"signal.target.ancestors.file.group",
		"signal.target.ancestors.file.in_upper_layer",
		"signal.target.ancestors.file.inode",
		"signal.target.ancestors.file.mode",
		"signal.target.ancestors.file.modification_time",
		"signal.target.ancestors.file.mount_id",
		"signal.target.ancestors.file.name",
		"signal.target.ancestors.file.name.length",
		"signal.target.ancestors.file.path",
		"signal.target.ancestors.file.path.length",
		"signal.target.ancestors.file.rights",
		"signal.target.ancestors.file.uid",
		"signal.target.ancestors.file.user",
		"signal.target.ancestors.fsgid",
		"signal.target.ancestors.fsgroup",
		"signal.target.ancestors.fsuid",
		"signal.target.ancestors.fsuser",
		"signal.target.ancestors.gid",
		"signal.target.ancestors.group",
		"signal.target.ancestors.interpreter.file.change_time",
		"signal.target.ancestors.interpreter.file.filesystem",
		"signal.target.ancestors.interpreter.file.gid",
		"signal.target.ancestors.interpreter.file.group",
		"signal.target.ancestors.interpreter.file.in_upper_layer",
		"signal.target.ancestors.interpreter.file.inode",
		"signal.target.ancestors.interpreter.file.mode",
		"signal.target.ancestors.interpreter.file.modification_time",
		"signal.target.ancestors.interpreter.file.mount_id",
		"signal.target.ancestors.interpreter.file.name",
		"signal.target.ancestors.interpreter.file.name.length",
		"signal.target.ancestors.interpreter.file.path",
		"signal.target.ancestors.interpreter.file.path.length",
		"signal.target.ancestors.interpreter.file.rights",
		"signal.target.ancestors.interpreter.file.uid",
		"signal.target.ancestors.interpreter.file.user",
		"signal.target.ancestors.is_kworker",
		"signal.target.ancestors.is_thread",
		"signal.target.ancestors.pid",
		"signal.target.ancestors.ppid",
		"signal.target.ancestors.tid",
		"signal.target.ancestors.tty_name",
		"signal.target.ancestors.uid",
		"signal.target.ancestors.user",
		"signal.target.args",
		"signal.target.args_flags",
		"signal.target.args_options",
		"signal.target.args_truncated",
		"signal.target.argv",
		"signal.target.argv0",
		"signal.target.cap_effective",
		"signal.target.cap_permitted",
		"signal.target.comm",
		"signal.target.container.id",
		"signal.target.cookie",
		"signal.target.created_at",
		"signal.target.egid",
		"signal.target.egroup",
		"signal.target.envp",
		"signal.target.envs",
		"signal.target.envs_truncated",
		"signal.target.euid",
		"signal.target.euser",
		"signal.target.file.change_time",
		"signal.target.file.filesystem",
		"signal.target.file.gid",
		"signal.target.file.group",
		"signal.target.file.in_upper_layer",
		"signal.target.file.inode",
		"signal.target.file.mode",
		"signal.target.file.modification_time",
		"signal.target.file.mount_id",
		"signal.target.file.name",
		"signal.target.file.name.length",
		"signal.target.file.path",
		"signal.target.file.path.length",
		"signal.target.file.rights",
		"signal.target.file.uid",
		"signal.target.file.user",
		"signal.target.fsgid",
		"signal.target.fsgroup",
		"signal.target.fsuid",
		"signal.target.fsuser",
		"signal.target.gid",
		"signal.target.group",
		"signal.target.interpreter.file.change_time",
		"signal.target.interpreter.file.filesystem",
		"signal.target.interpreter.file.gid",
		"signal.target.interpreter.file.group",
		"signal.target.interpreter.file.in_upper_layer",
		"signal.target.interpreter.file.inode",
		"signal.target.interpreter.file.mode",
		"signal.target.interpreter.file.modification_time",
		"signal.target.interpreter.file.mount_id",
		"signal.target.interpreter.file.name",
		"signal.target.interpreter.file.name.length",
		"signal.target.interpreter.file.path",
		"signal.target.interpreter.file.path.length",
		"signal.target.interpreter.file.rights",
		"signal.target.interpreter.file.uid",
		"signal.target.interpreter.file.user",
		"signal.target.is_kworker",
		"signal.target.is_thread",
		"signal.target.pid",
		"signal.target.ppid",
		"signal.target.tid",
		"signal.target.tty_name",
		"signal.target.uid",
		"signal.target.user",
		"signal.type",
		"splice.file.change_time",
		"splice.file.filesystem",
		"splice.file.gid",
		"splice.file.group",
		"splice.file.in_upper_layer",
		"splice.file.inode",
		"splice.file.mode",
		"splice.file.modification_time",
		"splice.file.mount_id",
		"splice.file.name",
		"splice.file.name.length",
		"splice.file.path",
		"splice.file.path.length",
		"splice.file.rights",
		"splice.file.uid",
		"splice.file.user",
		"splice.pipe_entry_flag",
		"splice.pipe_exit_flag",
		"splice.retval",
		"unlink.file.change_time",
		"unlink.file.filesystem",
		"unlink.file.gid",
		"unlink.file.group",
		"unlink.file.in_upper_layer",
		"unlink.file.inode",
		"unlink.file.mode",
		"unlink.file.modification_time",
		"unlink.file.mount_id",
		"unlink.file.name",
		"unlink.file.name.length",
		"unlink.file.path",
		"unlink.file.path.length",
		"unlink.file.rights",
		"unlink.file.uid",
		"unlink.file.user",
		"unlink.flags",
		"unlink.retval",
		"unload_module.name",
		"unload_module.retval",
		"utimes.file.change_time",
		"utimes.file.filesystem",
		"utimes.file.gid",
		"utimes.file.group",
		"utimes.file.in_upper_layer",
		"utimes.file.inode",
		"utimes.file.mode",
		"utimes.file.modification_time",
		"utimes.file.mount_id",
		"utimes.file.name",
		"utimes.file.name.length",
		"utimes.file.path",
		"utimes.file.path.length",
		"utimes.file.rights",
		"utimes.file.uid",
		"utimes.file.user",
		"utimes.retval",
	}
}
func (e *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
	case "async":
		return e.Async, nil
	case "bind.addr.family":
		return int(e.Bind.AddrFamily), nil
	case "bind.addr.ip":
		return e.Bind.Addr.IPNet, nil
	case "bind.addr.port":
		return int(e.Bind.Addr.Port), nil
	case "bind.retval":
		return int(e.Bind.SyscallEvent.Retval), nil
	case "bpf.cmd":
		return int(e.BPF.Cmd), nil
	case "bpf.map.name":
		return e.BPF.Map.Name, nil
	case "bpf.map.type":
		return int(e.BPF.Map.Type), nil
	case "bpf.prog.attach_type":
		return int(e.BPF.Program.AttachType), nil
	case "bpf.prog.helpers":
		result := make([]int, len(e.BPF.Program.Helpers))
		for i, v := range e.BPF.Program.Helpers {
			result[i] = int(v)
		}
		return result, nil
	case "bpf.prog.name":
		return e.BPF.Program.Name, nil
	case "bpf.prog.tag":
		return e.BPF.Program.Tag, nil
	case "bpf.prog.type":
		return int(e.BPF.Program.Type), nil
	case "bpf.retval":
		return int(e.BPF.SyscallEvent.Retval), nil
	case "capset.cap_effective":
		return int(e.Capset.CapEffective), nil
	case "capset.cap_permitted":
		return int(e.Capset.CapPermitted), nil
	case "chmod.file.change_time":
		return int(e.Chmod.File.FileFields.CTime), nil
	case "chmod.file.destination.mode":
		return int(e.Chmod.Mode), nil
	case "chmod.file.destination.rights":
		return int(e.Chmod.Mode), nil
	case "chmod.file.filesystem":
		return e.Chmod.File.Filesystem, nil
	case "chmod.file.gid":
		return int(e.Chmod.File.FileFields.GID), nil
	case "chmod.file.group":
		return e.Chmod.File.FileFields.Group, nil
	case "chmod.file.in_upper_layer":
		return e.Chmod.File.FileFields.InUpperLayer, nil
	case "chmod.file.inode":
		return int(e.Chmod.File.FileFields.Inode), nil
	case "chmod.file.mode":
		return int(e.Chmod.File.FileFields.Mode), nil
	case "chmod.file.modification_time":
		return int(e.Chmod.File.FileFields.MTime), nil
	case "chmod.file.mount_id":
		return int(e.Chmod.File.FileFields.MountID), nil
	case "chmod.file.name":
		return e.Chmod.File.BasenameStr, nil
	case "chmod.file.name.length":
		return len(e.Chmod.File.BasenameStr), nil
	case "chmod.file.path":
		return e.Chmod.File.PathnameStr, nil
	case "chmod.file.path.length":
		return len(e.Chmod.File.PathnameStr), nil
	case "chmod.file.rights":
		return int(e.Chmod.File.FileFields.Mode), nil
	case "chmod.file.uid":
		return int(e.Chmod.File.FileFields.UID), nil
	case "chmod.file.user":
		return e.Chmod.File.FileFields.User, nil
	case "chmod.retval":
		return int(e.Chmod.SyscallEvent.Retval), nil
	case "chown.file.change_time":
		return int(e.Chown.File.FileFields.CTime), nil
	case "chown.file.destination.gid":
		return int(e.Chown.GID), nil
	case "chown.file.destination.group":
		return e.Chown.Group, nil
	case "chown.file.destination.uid":
		return int(e.Chown.UID), nil
	case "chown.file.destination.user":
		return e.Chown.User, nil
	case "chown.file.filesystem":
		return e.Chown.File.Filesystem, nil
	case "chown.file.gid":
		return int(e.Chown.File.FileFields.GID), nil
	case "chown.file.group":
		return e.Chown.File.FileFields.Group, nil
	case "chown.file.in_upper_layer":
		return e.Chown.File.FileFields.InUpperLayer, nil
	case "chown.file.inode":
		return int(e.Chown.File.FileFields.Inode), nil
	case "chown.file.mode":
		return int(e.Chown.File.FileFields.Mode), nil
	case "chown.file.modification_time":
		return int(e.Chown.File.FileFields.MTime), nil
	case "chown.file.mount_id":
		return int(e.Chown.File.FileFields.MountID), nil
	case "chown.file.name":
		return e.Chown.File.BasenameStr, nil
	case "chown.file.name.length":
		return len(e.Chown.File.BasenameStr), nil
	case "chown.file.path":
		return e.Chown.File.PathnameStr, nil
	case "chown.file.path.length":
		return len(e.Chown.File.PathnameStr), nil
	case "chown.file.rights":
		return int(e.Chown.File.FileFields.Mode), nil
	case "chown.file.uid":
		return int(e.Chown.File.FileFields.UID), nil
	case "chown.file.user":
		return e.Chown.File.FileFields.User, nil
	case "chown.retval":
		return int(e.Chown.SyscallEvent.Retval), nil
	case "container.id":
		return e.ContainerContext.ID, nil
	case "container.tags":
		return e.ContainerContext.Tags, nil
	case "dns.id":
		return int(e.DNS.ID), nil
	case "dns.question.class":
		return int(e.DNS.Class), nil
	case "dns.question.count":
		return int(e.DNS.Count), nil
	case "dns.question.length":
		return int(e.DNS.Size), nil
	case "dns.question.name":
		return e.DNS.Name, nil
	case "dns.question.name.length":
		return len(e.DNS.Name), nil
	case "dns.question.type":
		return int(e.DNS.Type), nil
	case "exec.args":
		return e.Exec.Process.Args, nil
	case "exec.args_flags":
		return e.Exec.Process.Argv, nil
	case "exec.args_options":
		return e.Exec.Process.Argv, nil
	case "exec.args_truncated":
		return e.Exec.Process.ArgsTruncated, nil
	case "exec.argv":
		return e.Exec.Process.Argv, nil
	case "exec.argv0":
		return e.Exec.Process.Argv0, nil
	case "exec.cap_effective":
		return int(e.Exec.Process.Credentials.CapEffective), nil
	case "exec.cap_permitted":
		return int(e.Exec.Process.Credentials.CapPermitted), nil
	case "exec.comm":
		return e.Exec.Process.Comm, nil
	case "exec.container.id":
		return e.Exec.Process.ContainerID, nil
	case "exec.cookie":
		return int(e.Exec.Process.Cookie), nil
	case "exec.created_at":
		return int(e.Exec.Process.CreatedAt), nil
	case "exec.egid":
		return int(e.Exec.Process.Credentials.EGID), nil
	case "exec.egroup":
		return e.Exec.Process.Credentials.EGroup, nil
	case "exec.envp":
		return e.Exec.Process.Envp, nil
	case "exec.envs":
		return e.Exec.Process.Envs, nil
	case "exec.envs_truncated":
		return e.Exec.Process.EnvsTruncated, nil
	case "exec.euid":
		return int(e.Exec.Process.Credentials.EUID), nil
	case "exec.euser":
		return e.Exec.Process.Credentials.EUser, nil
	case "exec.file.change_time":
		return int(e.Exec.Process.FileEvent.FileFields.CTime), nil
	case "exec.file.filesystem":
		return e.Exec.Process.FileEvent.Filesystem, nil
	case "exec.file.gid":
		return int(e.Exec.Process.FileEvent.FileFields.GID), nil
	case "exec.file.group":
		return e.Exec.Process.FileEvent.FileFields.Group, nil
	case "exec.file.in_upper_layer":
		return e.Exec.Process.FileEvent.FileFields.InUpperLayer, nil
	case "exec.file.inode":
		return int(e.Exec.Process.FileEvent.FileFields.Inode), nil
	case "exec.file.mode":
		return int(e.Exec.Process.FileEvent.FileFields.Mode), nil
	case "exec.file.modification_time":
		return int(e.Exec.Process.FileEvent.FileFields.MTime), nil
	case "exec.file.mount_id":
		return int(e.Exec.Process.FileEvent.FileFields.MountID), nil
	case "exec.file.name":
		return e.Exec.Process.FileEvent.BasenameStr, nil
	case "exec.file.name.length":
		return len(e.Exec.Process.FileEvent.BasenameStr), nil
	case "exec.file.path":
		return e.Exec.Process.FileEvent.PathnameStr, nil
	case "exec.file.path.length":
		return len(e.Exec.Process.FileEvent.PathnameStr), nil
	case "exec.file.rights":
		return int(e.Exec.Process.FileEvent.FileFields.Mode), nil
	case "exec.file.uid":
		return int(e.Exec.Process.FileEvent.FileFields.UID), nil
	case "exec.file.user":
		return e.Exec.Process.FileEvent.FileFields.User, nil
	case "exec.fsgid":
		return int(e.Exec.Process.Credentials.FSGID), nil
	case "exec.fsgroup":
		return e.Exec.Process.Credentials.FSGroup, nil
	case "exec.fsuid":
		return int(e.Exec.Process.Credentials.FSUID), nil
	case "exec.fsuser":
		return e.Exec.Process.Credentials.FSUser, nil
	case "exec.gid":
		return int(e.Exec.Process.Credentials.GID), nil
	case "exec.group":
		return e.Exec.Process.Credentials.Group, nil
	case "exec.interpreter.file.change_time":
		return int(e.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "exec.interpreter.file.filesystem":
		return e.Exec.Process.LinuxBinprm.FileEvent.Filesystem, nil
	case "exec.interpreter.file.gid":
		return int(e.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "exec.interpreter.file.group":
		return e.Exec.Process.LinuxBinprm.FileEvent.FileFields.Group, nil
	case "exec.interpreter.file.in_upper_layer":
		return e.Exec.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, nil
	case "exec.interpreter.file.inode":
		return int(e.Exec.Process.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "exec.interpreter.file.mode":
		return int(e.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "exec.interpreter.file.modification_time":
		return int(e.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "exec.interpreter.file.mount_id":
		return int(e.Exec.Process.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "exec.interpreter.file.name":
		return e.Exec.Process.LinuxBinprm.FileEvent.BasenameStr, nil
	case "exec.interpreter.file.name.length":
		return len(e.Exec.Process.LinuxBinprm.FileEvent.BasenameStr), nil
	case "exec.interpreter.file.path":
		return e.Exec.Process.LinuxBinprm.FileEvent.PathnameStr, nil
	case "exec.interpreter.file.path.length":
		return len(e.Exec.Process.LinuxBinprm.FileEvent.PathnameStr), nil
	case "exec.interpreter.file.rights":
		return int(e.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "exec.interpreter.file.uid":
		return int(e.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "exec.interpreter.file.user":
		return e.Exec.Process.LinuxBinprm.FileEvent.FileFields.User, nil
	case "exec.is_kworker":
		return e.Exec.Process.PIDContext.IsKworker, nil
	case "exec.is_thread":
		return e.Exec.Process.IsThread, nil
	case "exec.pid":
		return int(e.Exec.Process.PIDContext.Pid), nil
	case "exec.ppid":
		return int(e.Exec.Process.PPid), nil
	case "exec.tid":
		return int(e.Exec.Process.PIDContext.Tid), nil
	case "exec.tty_name":
		return e.Exec.Process.TTYName, nil
	case "exec.uid":
		return int(e.Exec.Process.Credentials.UID), nil
	case "exec.user":
		return e.Exec.Process.Credentials.User, nil
	case "exit.args":
		return e.Exit.Process.Args, nil
	case "exit.args_flags":
		return e.Exit.Process.Argv, nil
	case "exit.args_options":
		return e.Exit.Process.Argv, nil
	case "exit.args_truncated":
		return e.Exit.Process.ArgsTruncated, nil
	case "exit.argv":
		return e.Exit.Process.Argv, nil
	case "exit.argv0":
		return e.Exit.Process.Argv0, nil
	case "exit.cap_effective":
		return int(e.Exit.Process.Credentials.CapEffective), nil
	case "exit.cap_permitted":
		return int(e.Exit.Process.Credentials.CapPermitted), nil
	case "exit.cause":
		return int(e.Exit.Cause), nil
	case "exit.code":
		return int(e.Exit.Code), nil
	case "exit.comm":
		return e.Exit.Process.Comm, nil
	case "exit.container.id":
		return e.Exit.Process.ContainerID, nil
	case "exit.cookie":
		return int(e.Exit.Process.Cookie), nil
	case "exit.created_at":
		return int(e.Exit.Process.CreatedAt), nil
	case "exit.egid":
		return int(e.Exit.Process.Credentials.EGID), nil
	case "exit.egroup":
		return e.Exit.Process.Credentials.EGroup, nil
	case "exit.envp":
		return e.Exit.Process.Envp, nil
	case "exit.envs":
		return e.Exit.Process.Envs, nil
	case "exit.envs_truncated":
		return e.Exit.Process.EnvsTruncated, nil
	case "exit.euid":
		return int(e.Exit.Process.Credentials.EUID), nil
	case "exit.euser":
		return e.Exit.Process.Credentials.EUser, nil
	case "exit.file.change_time":
		return int(e.Exit.Process.FileEvent.FileFields.CTime), nil
	case "exit.file.filesystem":
		return e.Exit.Process.FileEvent.Filesystem, nil
	case "exit.file.gid":
		return int(e.Exit.Process.FileEvent.FileFields.GID), nil
	case "exit.file.group":
		return e.Exit.Process.FileEvent.FileFields.Group, nil
	case "exit.file.in_upper_layer":
		return e.Exit.Process.FileEvent.FileFields.InUpperLayer, nil
	case "exit.file.inode":
		return int(e.Exit.Process.FileEvent.FileFields.Inode), nil
	case "exit.file.mode":
		return int(e.Exit.Process.FileEvent.FileFields.Mode), nil
	case "exit.file.modification_time":
		return int(e.Exit.Process.FileEvent.FileFields.MTime), nil
	case "exit.file.mount_id":
		return int(e.Exit.Process.FileEvent.FileFields.MountID), nil
	case "exit.file.name":
		return e.Exit.Process.FileEvent.BasenameStr, nil
	case "exit.file.name.length":
		return len(e.Exit.Process.FileEvent.BasenameStr), nil
	case "exit.file.path":
		return e.Exit.Process.FileEvent.PathnameStr, nil
	case "exit.file.path.length":
		return len(e.Exit.Process.FileEvent.PathnameStr), nil
	case "exit.file.rights":
		return int(e.Exit.Process.FileEvent.FileFields.Mode), nil
	case "exit.file.uid":
		return int(e.Exit.Process.FileEvent.FileFields.UID), nil
	case "exit.file.user":
		return e.Exit.Process.FileEvent.FileFields.User, nil
	case "exit.fsgid":
		return int(e.Exit.Process.Credentials.FSGID), nil
	case "exit.fsgroup":
		return e.Exit.Process.Credentials.FSGroup, nil
	case "exit.fsuid":
		return int(e.Exit.Process.Credentials.FSUID), nil
	case "exit.fsuser":
		return e.Exit.Process.Credentials.FSUser, nil
	case "exit.gid":
		return int(e.Exit.Process.Credentials.GID), nil
	case "exit.group":
		return e.Exit.Process.Credentials.Group, nil
	case "exit.interpreter.file.change_time":
		return int(e.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "exit.interpreter.file.filesystem":
		return e.Exit.Process.LinuxBinprm.FileEvent.Filesystem, nil
	case "exit.interpreter.file.gid":
		return int(e.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "exit.interpreter.file.group":
		return e.Exit.Process.LinuxBinprm.FileEvent.FileFields.Group, nil
	case "exit.interpreter.file.in_upper_layer":
		return e.Exit.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, nil
	case "exit.interpreter.file.inode":
		return int(e.Exit.Process.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "exit.interpreter.file.mode":
		return int(e.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "exit.interpreter.file.modification_time":
		return int(e.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "exit.interpreter.file.mount_id":
		return int(e.Exit.Process.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "exit.interpreter.file.name":
		return e.Exit.Process.LinuxBinprm.FileEvent.BasenameStr, nil
	case "exit.interpreter.file.name.length":
		return len(e.Exit.Process.LinuxBinprm.FileEvent.BasenameStr), nil
	case "exit.interpreter.file.path":
		return e.Exit.Process.LinuxBinprm.FileEvent.PathnameStr, nil
	case "exit.interpreter.file.path.length":
		return len(e.Exit.Process.LinuxBinprm.FileEvent.PathnameStr), nil
	case "exit.interpreter.file.rights":
		return int(e.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "exit.interpreter.file.uid":
		return int(e.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "exit.interpreter.file.user":
		return e.Exit.Process.LinuxBinprm.FileEvent.FileFields.User, nil
	case "exit.is_kworker":
		return e.Exit.Process.PIDContext.IsKworker, nil
	case "exit.is_thread":
		return e.Exit.Process.IsThread, nil
	case "exit.pid":
		return int(e.Exit.Process.PIDContext.Pid), nil
	case "exit.ppid":
		return int(e.Exit.Process.PPid), nil
	case "exit.tid":
		return int(e.Exit.Process.PIDContext.Tid), nil
	case "exit.tty_name":
		return e.Exit.Process.TTYName, nil
	case "exit.uid":
		return int(e.Exit.Process.Credentials.UID), nil
	case "exit.user":
		return e.Exit.Process.Credentials.User, nil
	case "link.file.change_time":
		return int(e.Link.Source.FileFields.CTime), nil
	case "link.file.destination.change_time":
		return int(e.Link.Target.FileFields.CTime), nil
	case "link.file.destination.filesystem":
		return e.Link.Target.Filesystem, nil
	case "link.file.destination.gid":
		return int(e.Link.Target.FileFields.GID), nil
	case "link.file.destination.group":
		return e.Link.Target.FileFields.Group, nil
	case "link.file.destination.in_upper_layer":
		return e.Link.Target.FileFields.InUpperLayer, nil
	case "link.file.destination.inode":
		return int(e.Link.Target.FileFields.Inode), nil
	case "link.file.destination.mode":
		return int(e.Link.Target.FileFields.Mode), nil
	case "link.file.destination.modification_time":
		return int(e.Link.Target.FileFields.MTime), nil
	case "link.file.destination.mount_id":
		return int(e.Link.Target.FileFields.MountID), nil
	case "link.file.destination.name":
		return e.Link.Target.BasenameStr, nil
	case "link.file.destination.name.length":
		return len(e.Link.Target.BasenameStr), nil
	case "link.file.destination.path":
		return e.Link.Target.PathnameStr, nil
	case "link.file.destination.path.length":
		return len(e.Link.Target.PathnameStr), nil
	case "link.file.destination.rights":
		return int(e.Link.Target.FileFields.Mode), nil
	case "link.file.destination.uid":
		return int(e.Link.Target.FileFields.UID), nil
	case "link.file.destination.user":
		return e.Link.Target.FileFields.User, nil
	case "link.file.filesystem":
		return e.Link.Source.Filesystem, nil
	case "link.file.gid":
		return int(e.Link.Source.FileFields.GID), nil
	case "link.file.group":
		return e.Link.Source.FileFields.Group, nil
	case "link.file.in_upper_layer":
		return e.Link.Source.FileFields.InUpperLayer, nil
	case "link.file.inode":
		return int(e.Link.Source.FileFields.Inode), nil
	case "link.file.mode":
		return int(e.Link.Source.FileFields.Mode), nil
	case "link.file.modification_time":
		return int(e.Link.Source.FileFields.MTime), nil
	case "link.file.mount_id":
		return int(e.Link.Source.FileFields.MountID), nil
	case "link.file.name":
		return e.Link.Source.BasenameStr, nil
	case "link.file.name.length":
		return len(e.Link.Source.BasenameStr), nil
	case "link.file.path":
		return e.Link.Source.PathnameStr, nil
	case "link.file.path.length":
		return len(e.Link.Source.PathnameStr), nil
	case "link.file.rights":
		return int(e.Link.Source.FileFields.Mode), nil
	case "link.file.uid":
		return int(e.Link.Source.FileFields.UID), nil
	case "link.file.user":
		return e.Link.Source.FileFields.User, nil
	case "link.retval":
		return int(e.Link.SyscallEvent.Retval), nil
	case "load_module.file.change_time":
		return int(e.LoadModule.File.FileFields.CTime), nil
	case "load_module.file.filesystem":
		return e.LoadModule.File.Filesystem, nil
	case "load_module.file.gid":
		return int(e.LoadModule.File.FileFields.GID), nil
	case "load_module.file.group":
		return e.LoadModule.File.FileFields.Group, nil
	case "load_module.file.in_upper_layer":
		return e.LoadModule.File.FileFields.InUpperLayer, nil
	case "load_module.file.inode":
		return int(e.LoadModule.File.FileFields.Inode), nil
	case "load_module.file.mode":
		return int(e.LoadModule.File.FileFields.Mode), nil
	case "load_module.file.modification_time":
		return int(e.LoadModule.File.FileFields.MTime), nil
	case "load_module.file.mount_id":
		return int(e.LoadModule.File.FileFields.MountID), nil
	case "load_module.file.name":
		return e.LoadModule.File.BasenameStr, nil
	case "load_module.file.name.length":
		return len(e.LoadModule.File.BasenameStr), nil
	case "load_module.file.path":
		return e.LoadModule.File.PathnameStr, nil
	case "load_module.file.path.length":
		return len(e.LoadModule.File.PathnameStr), nil
	case "load_module.file.rights":
		return int(e.LoadModule.File.FileFields.Mode), nil
	case "load_module.file.uid":
		return int(e.LoadModule.File.FileFields.UID), nil
	case "load_module.file.user":
		return e.LoadModule.File.FileFields.User, nil
	case "load_module.loaded_from_memory":
		return e.LoadModule.LoadedFromMemory, nil
	case "load_module.name":
		return e.LoadModule.Name, nil
	case "load_module.retval":
		return int(e.LoadModule.SyscallEvent.Retval), nil
	case "mkdir.file.change_time":
		return int(e.Mkdir.File.FileFields.CTime), nil
	case "mkdir.file.destination.mode":
		return int(e.Mkdir.Mode), nil
	case "mkdir.file.destination.rights":
		return int(e.Mkdir.Mode), nil
	case "mkdir.file.filesystem":
		return e.Mkdir.File.Filesystem, nil
	case "mkdir.file.gid":
		return int(e.Mkdir.File.FileFields.GID), nil
	case "mkdir.file.group":
		return e.Mkdir.File.FileFields.Group, nil
	case "mkdir.file.in_upper_layer":
		return e.Mkdir.File.FileFields.InUpperLayer, nil
	case "mkdir.file.inode":
		return int(e.Mkdir.File.FileFields.Inode), nil
	case "mkdir.file.mode":
		return int(e.Mkdir.File.FileFields.Mode), nil
	case "mkdir.file.modification_time":
		return int(e.Mkdir.File.FileFields.MTime), nil
	case "mkdir.file.mount_id":
		return int(e.Mkdir.File.FileFields.MountID), nil
	case "mkdir.file.name":
		return e.Mkdir.File.BasenameStr, nil
	case "mkdir.file.name.length":
		return len(e.Mkdir.File.BasenameStr), nil
	case "mkdir.file.path":
		return e.Mkdir.File.PathnameStr, nil
	case "mkdir.file.path.length":
		return len(e.Mkdir.File.PathnameStr), nil
	case "mkdir.file.rights":
		return int(e.Mkdir.File.FileFields.Mode), nil
	case "mkdir.file.uid":
		return int(e.Mkdir.File.FileFields.UID), nil
	case "mkdir.file.user":
		return e.Mkdir.File.FileFields.User, nil
	case "mkdir.retval":
		return int(e.Mkdir.SyscallEvent.Retval), nil
	case "mmap.file.change_time":
		return int(e.MMap.File.FileFields.CTime), nil
	case "mmap.file.filesystem":
		return e.MMap.File.Filesystem, nil
	case "mmap.file.gid":
		return int(e.MMap.File.FileFields.GID), nil
	case "mmap.file.group":
		return e.MMap.File.FileFields.Group, nil
	case "mmap.file.in_upper_layer":
		return e.MMap.File.FileFields.InUpperLayer, nil
	case "mmap.file.inode":
		return int(e.MMap.File.FileFields.Inode), nil
	case "mmap.file.mode":
		return int(e.MMap.File.FileFields.Mode), nil
	case "mmap.file.modification_time":
		return int(e.MMap.File.FileFields.MTime), nil
	case "mmap.file.mount_id":
		return int(e.MMap.File.FileFields.MountID), nil
	case "mmap.file.name":
		return e.MMap.File.BasenameStr, nil
	case "mmap.file.name.length":
		return len(e.MMap.File.BasenameStr), nil
	case "mmap.file.path":
		return e.MMap.File.PathnameStr, nil
	case "mmap.file.path.length":
		return len(e.MMap.File.PathnameStr), nil
	case "mmap.file.rights":
		return int(e.MMap.File.FileFields.Mode), nil
	case "mmap.file.uid":
		return int(e.MMap.File.FileFields.UID), nil
	case "mmap.file.user":
		return e.MMap.File.FileFields.User, nil
	case "mmap.flags":
		return e.MMap.Flags, nil
	case "mmap.protection":
		return e.MMap.Protection, nil
	case "mmap.retval":
		return int(e.MMap.SyscallEvent.Retval), nil
	case "mprotect.req_protection":
		return e.MProtect.ReqProtection, nil
	case "mprotect.retval":
		return int(e.MProtect.SyscallEvent.Retval), nil
	case "mprotect.vm_protection":
		return e.MProtect.VMProtection, nil
	case "network.destination.ip":
		return e.NetworkContext.Destination.IPNet, nil
	case "network.destination.port":
		return int(e.NetworkContext.Destination.Port), nil
	case "network.device.ifindex":
		return int(e.NetworkContext.Device.IfIndex), nil
	case "network.device.ifname":
		return e.NetworkContext.Device.IfName, nil
	case "network.l3_protocol":
		return int(e.NetworkContext.L3Protocol), nil
	case "network.l4_protocol":
		return int(e.NetworkContext.L4Protocol), nil
	case "network.size":
		return int(e.NetworkContext.Size), nil
	case "network.source.ip":
		return e.NetworkContext.Source.IPNet, nil
	case "network.source.port":
		return int(e.NetworkContext.Source.Port), nil
	case "open.file.change_time":
		return int(e.Open.File.FileFields.CTime), nil
	case "open.file.destination.mode":
		return int(e.Open.Mode), nil
	case "open.file.filesystem":
		return e.Open.File.Filesystem, nil
	case "open.file.gid":
		return int(e.Open.File.FileFields.GID), nil
	case "open.file.group":
		return e.Open.File.FileFields.Group, nil
	case "open.file.in_upper_layer":
		return e.Open.File.FileFields.InUpperLayer, nil
	case "open.file.inode":
		return int(e.Open.File.FileFields.Inode), nil
	case "open.file.mode":
		return int(e.Open.File.FileFields.Mode), nil
	case "open.file.modification_time":
		return int(e.Open.File.FileFields.MTime), nil
	case "open.file.mount_id":
		return int(e.Open.File.FileFields.MountID), nil
	case "open.file.name":
		return e.Open.File.BasenameStr, nil
	case "open.file.name.length":
		return len(e.Open.File.BasenameStr), nil
	case "open.file.path":
		return e.Open.File.PathnameStr, nil
	case "open.file.path.length":
		return len(e.Open.File.PathnameStr), nil
	case "open.file.rights":
		return int(e.Open.File.FileFields.Mode), nil
	case "open.file.uid":
		return int(e.Open.File.FileFields.UID), nil
	case "open.file.user":
		return e.Open.File.FileFields.User, nil
	case "open.flags":
		return int(e.Open.Flags), nil
	case "open.retval":
		return int(e.Open.SyscallEvent.Retval), nil
	case "process.ancestors.args":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Args
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.args_flags":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.args_options":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.args_truncated":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.ArgsTruncated
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.argv":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.argv0":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv0
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.cap_effective":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.CreatedAt)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.egid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Envp
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.envs":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Envs
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.envs_truncated":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.EnvsTruncated
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.euid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.Filesystem
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.gid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.in_upper_layer":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.InUpperLayer
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.inode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.BasenameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.name.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.FileEvent.BasenameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.path":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.PathnameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.path.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.FileEvent.PathnameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.rights":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.uid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.fsgid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.gid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.in_upper_layer":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.inode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.name.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.path":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.path.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.rights":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.interpreter.file.uid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.is_kworker":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		ctx := eval.NewContext(unsafe.Pointer(e))
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
		return e.ProcessContext.Process.Args, nil
	case "process.args_flags":
		return e.ProcessContext.Process.Argv, nil
	case "process.args_options":
		return e.ProcessContext.Process.Argv, nil
	case "process.args_truncated":
		return e.ProcessContext.Process.ArgsTruncated, nil
	case "process.argv":
		return e.ProcessContext.Process.Argv, nil
	case "process.argv0":
		return e.ProcessContext.Process.Argv0, nil
	case "process.cap_effective":
		return int(e.ProcessContext.Process.Credentials.CapEffective), nil
	case "process.cap_permitted":
		return int(e.ProcessContext.Process.Credentials.CapPermitted), nil
	case "process.comm":
		return e.ProcessContext.Process.Comm, nil
	case "process.container.id":
		return e.ProcessContext.Process.ContainerID, nil
	case "process.cookie":
		return int(e.ProcessContext.Process.Cookie), nil
	case "process.created_at":
		return int(e.ProcessContext.Process.CreatedAt), nil
	case "process.egid":
		return int(e.ProcessContext.Process.Credentials.EGID), nil
	case "process.egroup":
		return e.ProcessContext.Process.Credentials.EGroup, nil
	case "process.envp":
		return e.ProcessContext.Process.Envp, nil
	case "process.envs":
		return e.ProcessContext.Process.Envs, nil
	case "process.envs_truncated":
		return e.ProcessContext.Process.EnvsTruncated, nil
	case "process.euid":
		return int(e.ProcessContext.Process.Credentials.EUID), nil
	case "process.euser":
		return e.ProcessContext.Process.Credentials.EUser, nil
	case "process.file.change_time":
		return int(e.ProcessContext.Process.FileEvent.FileFields.CTime), nil
	case "process.file.filesystem":
		return e.ProcessContext.Process.FileEvent.Filesystem, nil
	case "process.file.gid":
		return int(e.ProcessContext.Process.FileEvent.FileFields.GID), nil
	case "process.file.group":
		return e.ProcessContext.Process.FileEvent.FileFields.Group, nil
	case "process.file.in_upper_layer":
		return e.ProcessContext.Process.FileEvent.FileFields.InUpperLayer, nil
	case "process.file.inode":
		return int(e.ProcessContext.Process.FileEvent.FileFields.Inode), nil
	case "process.file.mode":
		return int(e.ProcessContext.Process.FileEvent.FileFields.Mode), nil
	case "process.file.modification_time":
		return int(e.ProcessContext.Process.FileEvent.FileFields.MTime), nil
	case "process.file.mount_id":
		return int(e.ProcessContext.Process.FileEvent.FileFields.MountID), nil
	case "process.file.name":
		return e.ProcessContext.Process.FileEvent.BasenameStr, nil
	case "process.file.name.length":
		return len(e.ProcessContext.Process.FileEvent.BasenameStr), nil
	case "process.file.path":
		return e.ProcessContext.Process.FileEvent.PathnameStr, nil
	case "process.file.path.length":
		return len(e.ProcessContext.Process.FileEvent.PathnameStr), nil
	case "process.file.rights":
		return int(e.ProcessContext.Process.FileEvent.FileFields.Mode), nil
	case "process.file.uid":
		return int(e.ProcessContext.Process.FileEvent.FileFields.UID), nil
	case "process.file.user":
		return e.ProcessContext.Process.FileEvent.FileFields.User, nil
	case "process.fsgid":
		return int(e.ProcessContext.Process.Credentials.FSGID), nil
	case "process.fsgroup":
		return e.ProcessContext.Process.Credentials.FSGroup, nil
	case "process.fsuid":
		return int(e.ProcessContext.Process.Credentials.FSUID), nil
	case "process.fsuser":
		return e.ProcessContext.Process.Credentials.FSUser, nil
	case "process.gid":
		return int(e.ProcessContext.Process.Credentials.GID), nil
	case "process.group":
		return e.ProcessContext.Process.Credentials.Group, nil
	case "process.interpreter.file.change_time":
		return int(e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "process.interpreter.file.filesystem":
		return e.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem, nil
	case "process.interpreter.file.gid":
		return int(e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "process.interpreter.file.group":
		return e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group, nil
	case "process.interpreter.file.in_upper_layer":
		return e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, nil
	case "process.interpreter.file.inode":
		return int(e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "process.interpreter.file.mode":
		return int(e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "process.interpreter.file.modification_time":
		return int(e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "process.interpreter.file.mount_id":
		return int(e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "process.interpreter.file.name":
		return e.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr, nil
	case "process.interpreter.file.name.length":
		return len(e.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr), nil
	case "process.interpreter.file.path":
		return e.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr, nil
	case "process.interpreter.file.path.length":
		return len(e.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr), nil
	case "process.interpreter.file.rights":
		return int(e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "process.interpreter.file.uid":
		return int(e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "process.interpreter.file.user":
		return e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User, nil
	case "process.is_kworker":
		return e.ProcessContext.Process.PIDContext.IsKworker, nil
	case "process.is_thread":
		return e.ProcessContext.Process.IsThread, nil
	case "process.pid":
		return int(e.ProcessContext.Process.PIDContext.Pid), nil
	case "process.ppid":
		return int(e.ProcessContext.Process.PPid), nil
	case "process.tid":
		return int(e.ProcessContext.Process.PIDContext.Tid), nil
	case "process.tty_name":
		return e.ProcessContext.Process.TTYName, nil
	case "process.uid":
		return int(e.ProcessContext.Process.Credentials.UID), nil
	case "process.user":
		return e.ProcessContext.Process.Credentials.User, nil
	case "ptrace.request":
		return int(e.PTrace.Request), nil
	case "ptrace.retval":
		return int(e.PTrace.SyscallEvent.Retval), nil
	case "ptrace.tracee.ancestors.args":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Args
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.args_flags":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.args_options":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.args_truncated":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.ArgsTruncated
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.argv":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.argv0":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv0
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.cap_effective":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.CapEffective)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.cap_permitted":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.CapPermitted)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.comm":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Comm
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.container.id":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.ContainerID
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.cookie":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Cookie)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.created_at":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.CreatedAt)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.egid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.EGID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.egroup":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.EGroup
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.envp":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Envp
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.envs":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Envs
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.envs_truncated":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.EnvsTruncated
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.euid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.EUID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.euser":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.EUser
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.change_time":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.filesystem":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.Filesystem
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.gid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.group":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.in_upper_layer":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.InUpperLayer
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.inode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Inode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.mode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.modification_time":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.mount_id":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.MountID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.name":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.BasenameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.name.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.FileEvent.BasenameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.path":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.PathnameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.path.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.FileEvent.PathnameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.rights":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.uid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.file.user":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.fsgid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.FSGID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.fsgroup":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.FSGroup
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.fsuid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.FSUID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.fsuser":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.FSUser
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.gid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.group":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.change_time":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.filesystem":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.gid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.group":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.in_upper_layer":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.inode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.mode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.modification_time":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.mount_id":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.name":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.name.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.path":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.path.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.rights":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.uid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.interpreter.file.user":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.is_kworker":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.PIDContext.IsKworker
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.is_thread":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.IsThread
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.pid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PIDContext.Pid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.ppid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PPid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.tid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PIDContext.Tid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.tty_name":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.TTYName
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.uid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.ancestors.user":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "ptrace.tracee.args":
		return e.PTrace.Tracee.Process.Args, nil
	case "ptrace.tracee.args_flags":
		return e.PTrace.Tracee.Process.Argv, nil
	case "ptrace.tracee.args_options":
		return e.PTrace.Tracee.Process.Argv, nil
	case "ptrace.tracee.args_truncated":
		return e.PTrace.Tracee.Process.ArgsTruncated, nil
	case "ptrace.tracee.argv":
		return e.PTrace.Tracee.Process.Argv, nil
	case "ptrace.tracee.argv0":
		return e.PTrace.Tracee.Process.Argv0, nil
	case "ptrace.tracee.cap_effective":
		return int(e.PTrace.Tracee.Process.Credentials.CapEffective), nil
	case "ptrace.tracee.cap_permitted":
		return int(e.PTrace.Tracee.Process.Credentials.CapPermitted), nil
	case "ptrace.tracee.comm":
		return e.PTrace.Tracee.Process.Comm, nil
	case "ptrace.tracee.container.id":
		return e.PTrace.Tracee.Process.ContainerID, nil
	case "ptrace.tracee.cookie":
		return int(e.PTrace.Tracee.Process.Cookie), nil
	case "ptrace.tracee.created_at":
		return int(e.PTrace.Tracee.Process.CreatedAt), nil
	case "ptrace.tracee.egid":
		return int(e.PTrace.Tracee.Process.Credentials.EGID), nil
	case "ptrace.tracee.egroup":
		return e.PTrace.Tracee.Process.Credentials.EGroup, nil
	case "ptrace.tracee.envp":
		return e.PTrace.Tracee.Process.Envp, nil
	case "ptrace.tracee.envs":
		return e.PTrace.Tracee.Process.Envs, nil
	case "ptrace.tracee.envs_truncated":
		return e.PTrace.Tracee.Process.EnvsTruncated, nil
	case "ptrace.tracee.euid":
		return int(e.PTrace.Tracee.Process.Credentials.EUID), nil
	case "ptrace.tracee.euser":
		return e.PTrace.Tracee.Process.Credentials.EUser, nil
	case "ptrace.tracee.file.change_time":
		return int(e.PTrace.Tracee.Process.FileEvent.FileFields.CTime), nil
	case "ptrace.tracee.file.filesystem":
		return e.PTrace.Tracee.Process.FileEvent.Filesystem, nil
	case "ptrace.tracee.file.gid":
		return int(e.PTrace.Tracee.Process.FileEvent.FileFields.GID), nil
	case "ptrace.tracee.file.group":
		return e.PTrace.Tracee.Process.FileEvent.FileFields.Group, nil
	case "ptrace.tracee.file.in_upper_layer":
		return e.PTrace.Tracee.Process.FileEvent.FileFields.InUpperLayer, nil
	case "ptrace.tracee.file.inode":
		return int(e.PTrace.Tracee.Process.FileEvent.FileFields.Inode), nil
	case "ptrace.tracee.file.mode":
		return int(e.PTrace.Tracee.Process.FileEvent.FileFields.Mode), nil
	case "ptrace.tracee.file.modification_time":
		return int(e.PTrace.Tracee.Process.FileEvent.FileFields.MTime), nil
	case "ptrace.tracee.file.mount_id":
		return int(e.PTrace.Tracee.Process.FileEvent.FileFields.MountID), nil
	case "ptrace.tracee.file.name":
		return e.PTrace.Tracee.Process.FileEvent.BasenameStr, nil
	case "ptrace.tracee.file.name.length":
		return len(e.PTrace.Tracee.Process.FileEvent.BasenameStr), nil
	case "ptrace.tracee.file.path":
		return e.PTrace.Tracee.Process.FileEvent.PathnameStr, nil
	case "ptrace.tracee.file.path.length":
		return len(e.PTrace.Tracee.Process.FileEvent.PathnameStr), nil
	case "ptrace.tracee.file.rights":
		return int(e.PTrace.Tracee.Process.FileEvent.FileFields.Mode), nil
	case "ptrace.tracee.file.uid":
		return int(e.PTrace.Tracee.Process.FileEvent.FileFields.UID), nil
	case "ptrace.tracee.file.user":
		return e.PTrace.Tracee.Process.FileEvent.FileFields.User, nil
	case "ptrace.tracee.fsgid":
		return int(e.PTrace.Tracee.Process.Credentials.FSGID), nil
	case "ptrace.tracee.fsgroup":
		return e.PTrace.Tracee.Process.Credentials.FSGroup, nil
	case "ptrace.tracee.fsuid":
		return int(e.PTrace.Tracee.Process.Credentials.FSUID), nil
	case "ptrace.tracee.fsuser":
		return e.PTrace.Tracee.Process.Credentials.FSUser, nil
	case "ptrace.tracee.gid":
		return int(e.PTrace.Tracee.Process.Credentials.GID), nil
	case "ptrace.tracee.group":
		return e.PTrace.Tracee.Process.Credentials.Group, nil
	case "ptrace.tracee.interpreter.file.change_time":
		return int(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "ptrace.tracee.interpreter.file.filesystem":
		return e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.Filesystem, nil
	case "ptrace.tracee.interpreter.file.gid":
		return int(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "ptrace.tracee.interpreter.file.group":
		return e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Group, nil
	case "ptrace.tracee.interpreter.file.in_upper_layer":
		return e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, nil
	case "ptrace.tracee.interpreter.file.inode":
		return int(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "ptrace.tracee.interpreter.file.mode":
		return int(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "ptrace.tracee.interpreter.file.modification_time":
		return int(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "ptrace.tracee.interpreter.file.mount_id":
		return int(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "ptrace.tracee.interpreter.file.name":
		return e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.BasenameStr, nil
	case "ptrace.tracee.interpreter.file.name.length":
		return len(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.BasenameStr), nil
	case "ptrace.tracee.interpreter.file.path":
		return e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.PathnameStr, nil
	case "ptrace.tracee.interpreter.file.path.length":
		return len(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.PathnameStr), nil
	case "ptrace.tracee.interpreter.file.rights":
		return int(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "ptrace.tracee.interpreter.file.uid":
		return int(e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "ptrace.tracee.interpreter.file.user":
		return e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.User, nil
	case "ptrace.tracee.is_kworker":
		return e.PTrace.Tracee.Process.PIDContext.IsKworker, nil
	case "ptrace.tracee.is_thread":
		return e.PTrace.Tracee.Process.IsThread, nil
	case "ptrace.tracee.pid":
		return int(e.PTrace.Tracee.Process.PIDContext.Pid), nil
	case "ptrace.tracee.ppid":
		return int(e.PTrace.Tracee.Process.PPid), nil
	case "ptrace.tracee.tid":
		return int(e.PTrace.Tracee.Process.PIDContext.Tid), nil
	case "ptrace.tracee.tty_name":
		return e.PTrace.Tracee.Process.TTYName, nil
	case "ptrace.tracee.uid":
		return int(e.PTrace.Tracee.Process.Credentials.UID), nil
	case "ptrace.tracee.user":
		return e.PTrace.Tracee.Process.Credentials.User, nil
	case "removexattr.file.change_time":
		return int(e.RemoveXAttr.File.FileFields.CTime), nil
	case "removexattr.file.destination.name":
		return e.RemoveXAttr.Name, nil
	case "removexattr.file.destination.namespace":
		return e.RemoveXAttr.Namespace, nil
	case "removexattr.file.filesystem":
		return e.RemoveXAttr.File.Filesystem, nil
	case "removexattr.file.gid":
		return int(e.RemoveXAttr.File.FileFields.GID), nil
	case "removexattr.file.group":
		return e.RemoveXAttr.File.FileFields.Group, nil
	case "removexattr.file.in_upper_layer":
		return e.RemoveXAttr.File.FileFields.InUpperLayer, nil
	case "removexattr.file.inode":
		return int(e.RemoveXAttr.File.FileFields.Inode), nil
	case "removexattr.file.mode":
		return int(e.RemoveXAttr.File.FileFields.Mode), nil
	case "removexattr.file.modification_time":
		return int(e.RemoveXAttr.File.FileFields.MTime), nil
	case "removexattr.file.mount_id":
		return int(e.RemoveXAttr.File.FileFields.MountID), nil
	case "removexattr.file.name":
		return e.RemoveXAttr.File.BasenameStr, nil
	case "removexattr.file.name.length":
		return len(e.RemoveXAttr.File.BasenameStr), nil
	case "removexattr.file.path":
		return e.RemoveXAttr.File.PathnameStr, nil
	case "removexattr.file.path.length":
		return len(e.RemoveXAttr.File.PathnameStr), nil
	case "removexattr.file.rights":
		return int(e.RemoveXAttr.File.FileFields.Mode), nil
	case "removexattr.file.uid":
		return int(e.RemoveXAttr.File.FileFields.UID), nil
	case "removexattr.file.user":
		return e.RemoveXAttr.File.FileFields.User, nil
	case "removexattr.retval":
		return int(e.RemoveXAttr.SyscallEvent.Retval), nil
	case "rename.file.change_time":
		return int(e.Rename.Old.FileFields.CTime), nil
	case "rename.file.destination.change_time":
		return int(e.Rename.New.FileFields.CTime), nil
	case "rename.file.destination.filesystem":
		return e.Rename.New.Filesystem, nil
	case "rename.file.destination.gid":
		return int(e.Rename.New.FileFields.GID), nil
	case "rename.file.destination.group":
		return e.Rename.New.FileFields.Group, nil
	case "rename.file.destination.in_upper_layer":
		return e.Rename.New.FileFields.InUpperLayer, nil
	case "rename.file.destination.inode":
		return int(e.Rename.New.FileFields.Inode), nil
	case "rename.file.destination.mode":
		return int(e.Rename.New.FileFields.Mode), nil
	case "rename.file.destination.modification_time":
		return int(e.Rename.New.FileFields.MTime), nil
	case "rename.file.destination.mount_id":
		return int(e.Rename.New.FileFields.MountID), nil
	case "rename.file.destination.name":
		return e.Rename.New.BasenameStr, nil
	case "rename.file.destination.name.length":
		return len(e.Rename.New.BasenameStr), nil
	case "rename.file.destination.path":
		return e.Rename.New.PathnameStr, nil
	case "rename.file.destination.path.length":
		return len(e.Rename.New.PathnameStr), nil
	case "rename.file.destination.rights":
		return int(e.Rename.New.FileFields.Mode), nil
	case "rename.file.destination.uid":
		return int(e.Rename.New.FileFields.UID), nil
	case "rename.file.destination.user":
		return e.Rename.New.FileFields.User, nil
	case "rename.file.filesystem":
		return e.Rename.Old.Filesystem, nil
	case "rename.file.gid":
		return int(e.Rename.Old.FileFields.GID), nil
	case "rename.file.group":
		return e.Rename.Old.FileFields.Group, nil
	case "rename.file.in_upper_layer":
		return e.Rename.Old.FileFields.InUpperLayer, nil
	case "rename.file.inode":
		return int(e.Rename.Old.FileFields.Inode), nil
	case "rename.file.mode":
		return int(e.Rename.Old.FileFields.Mode), nil
	case "rename.file.modification_time":
		return int(e.Rename.Old.FileFields.MTime), nil
	case "rename.file.mount_id":
		return int(e.Rename.Old.FileFields.MountID), nil
	case "rename.file.name":
		return e.Rename.Old.BasenameStr, nil
	case "rename.file.name.length":
		return len(e.Rename.Old.BasenameStr), nil
	case "rename.file.path":
		return e.Rename.Old.PathnameStr, nil
	case "rename.file.path.length":
		return len(e.Rename.Old.PathnameStr), nil
	case "rename.file.rights":
		return int(e.Rename.Old.FileFields.Mode), nil
	case "rename.file.uid":
		return int(e.Rename.Old.FileFields.UID), nil
	case "rename.file.user":
		return e.Rename.Old.FileFields.User, nil
	case "rename.retval":
		return int(e.Rename.SyscallEvent.Retval), nil
	case "rmdir.file.change_time":
		return int(e.Rmdir.File.FileFields.CTime), nil
	case "rmdir.file.filesystem":
		return e.Rmdir.File.Filesystem, nil
	case "rmdir.file.gid":
		return int(e.Rmdir.File.FileFields.GID), nil
	case "rmdir.file.group":
		return e.Rmdir.File.FileFields.Group, nil
	case "rmdir.file.in_upper_layer":
		return e.Rmdir.File.FileFields.InUpperLayer, nil
	case "rmdir.file.inode":
		return int(e.Rmdir.File.FileFields.Inode), nil
	case "rmdir.file.mode":
		return int(e.Rmdir.File.FileFields.Mode), nil
	case "rmdir.file.modification_time":
		return int(e.Rmdir.File.FileFields.MTime), nil
	case "rmdir.file.mount_id":
		return int(e.Rmdir.File.FileFields.MountID), nil
	case "rmdir.file.name":
		return e.Rmdir.File.BasenameStr, nil
	case "rmdir.file.name.length":
		return len(e.Rmdir.File.BasenameStr), nil
	case "rmdir.file.path":
		return e.Rmdir.File.PathnameStr, nil
	case "rmdir.file.path.length":
		return len(e.Rmdir.File.PathnameStr), nil
	case "rmdir.file.rights":
		return int(e.Rmdir.File.FileFields.Mode), nil
	case "rmdir.file.uid":
		return int(e.Rmdir.File.FileFields.UID), nil
	case "rmdir.file.user":
		return e.Rmdir.File.FileFields.User, nil
	case "rmdir.retval":
		return int(e.Rmdir.SyscallEvent.Retval), nil
	case "selinux.bool.name":
		return e.SELinux.BoolName, nil
	case "selinux.bool.state":
		return e.SELinux.BoolChangeValue, nil
	case "selinux.bool_commit.state":
		return e.SELinux.BoolCommitValue, nil
	case "selinux.enforce.status":
		return e.SELinux.EnforceStatus, nil
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
	case "setxattr.file.change_time":
		return int(e.SetXAttr.File.FileFields.CTime), nil
	case "setxattr.file.destination.name":
		return e.SetXAttr.Name, nil
	case "setxattr.file.destination.namespace":
		return e.SetXAttr.Namespace, nil
	case "setxattr.file.filesystem":
		return e.SetXAttr.File.Filesystem, nil
	case "setxattr.file.gid":
		return int(e.SetXAttr.File.FileFields.GID), nil
	case "setxattr.file.group":
		return e.SetXAttr.File.FileFields.Group, nil
	case "setxattr.file.in_upper_layer":
		return e.SetXAttr.File.FileFields.InUpperLayer, nil
	case "setxattr.file.inode":
		return int(e.SetXAttr.File.FileFields.Inode), nil
	case "setxattr.file.mode":
		return int(e.SetXAttr.File.FileFields.Mode), nil
	case "setxattr.file.modification_time":
		return int(e.SetXAttr.File.FileFields.MTime), nil
	case "setxattr.file.mount_id":
		return int(e.SetXAttr.File.FileFields.MountID), nil
	case "setxattr.file.name":
		return e.SetXAttr.File.BasenameStr, nil
	case "setxattr.file.name.length":
		return len(e.SetXAttr.File.BasenameStr), nil
	case "setxattr.file.path":
		return e.SetXAttr.File.PathnameStr, nil
	case "setxattr.file.path.length":
		return len(e.SetXAttr.File.PathnameStr), nil
	case "setxattr.file.rights":
		return int(e.SetXAttr.File.FileFields.Mode), nil
	case "setxattr.file.uid":
		return int(e.SetXAttr.File.FileFields.UID), nil
	case "setxattr.file.user":
		return e.SetXAttr.File.FileFields.User, nil
	case "setxattr.retval":
		return int(e.SetXAttr.SyscallEvent.Retval), nil
	case "signal.pid":
		return int(e.Signal.PID), nil
	case "signal.retval":
		return int(e.Signal.SyscallEvent.Retval), nil
	case "signal.target.ancestors.args":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Args
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.args_flags":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.args_options":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.args_truncated":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.ArgsTruncated
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.argv":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.argv0":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Argv0
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.cap_effective":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.CapEffective)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.cap_permitted":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.CapPermitted)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.comm":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Comm
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.container.id":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.ContainerID
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.cookie":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Cookie)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.created_at":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.CreatedAt)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.egid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.EGID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.egroup":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.EGroup
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.envp":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Envp
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.envs":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Envs
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.envs_truncated":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.EnvsTruncated
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.euid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.EUID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.euser":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.EUser
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.change_time":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.filesystem":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.Filesystem
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.gid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.group":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.in_upper_layer":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.InUpperLayer
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.inode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Inode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.mode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.modification_time":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.mount_id":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.MountID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.name":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.BasenameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.name.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.FileEvent.BasenameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.path":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.PathnameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.path.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.FileEvent.PathnameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.rights":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.uid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.file.user":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.FileEvent.FileFields.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.fsgid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.FSGID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.fsgroup":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.FSGroup
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.fsuid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.FSUID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.fsuser":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.FSUser
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.gid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.group":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.change_time":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.filesystem":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.gid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.group":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.in_upper_layer":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.inode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.mode":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.modification_time":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.mount_id":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.name":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.name.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.path":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.path.length":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(element.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.rights":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.uid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.interpreter.file.user":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.is_kworker":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.PIDContext.IsKworker
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.is_thread":
		var values []bool
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.IsThread
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.pid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PIDContext.Pid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.ppid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PPid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.tid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.PIDContext.Tid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.tty_name":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.TTYName
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.uid":
		var values []int
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := int(element.ProcessContext.Process.Credentials.UID)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.ancestors.user":
		var values []string
		ctx := eval.NewContext(unsafe.Pointer(e))
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := element.ProcessContext.Process.Credentials.User
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "signal.target.args":
		return e.Signal.Target.Process.Args, nil
	case "signal.target.args_flags":
		return e.Signal.Target.Process.Argv, nil
	case "signal.target.args_options":
		return e.Signal.Target.Process.Argv, nil
	case "signal.target.args_truncated":
		return e.Signal.Target.Process.ArgsTruncated, nil
	case "signal.target.argv":
		return e.Signal.Target.Process.Argv, nil
	case "signal.target.argv0":
		return e.Signal.Target.Process.Argv0, nil
	case "signal.target.cap_effective":
		return int(e.Signal.Target.Process.Credentials.CapEffective), nil
	case "signal.target.cap_permitted":
		return int(e.Signal.Target.Process.Credentials.CapPermitted), nil
	case "signal.target.comm":
		return e.Signal.Target.Process.Comm, nil
	case "signal.target.container.id":
		return e.Signal.Target.Process.ContainerID, nil
	case "signal.target.cookie":
		return int(e.Signal.Target.Process.Cookie), nil
	case "signal.target.created_at":
		return int(e.Signal.Target.Process.CreatedAt), nil
	case "signal.target.egid":
		return int(e.Signal.Target.Process.Credentials.EGID), nil
	case "signal.target.egroup":
		return e.Signal.Target.Process.Credentials.EGroup, nil
	case "signal.target.envp":
		return e.Signal.Target.Process.Envp, nil
	case "signal.target.envs":
		return e.Signal.Target.Process.Envs, nil
	case "signal.target.envs_truncated":
		return e.Signal.Target.Process.EnvsTruncated, nil
	case "signal.target.euid":
		return int(e.Signal.Target.Process.Credentials.EUID), nil
	case "signal.target.euser":
		return e.Signal.Target.Process.Credentials.EUser, nil
	case "signal.target.file.change_time":
		return int(e.Signal.Target.Process.FileEvent.FileFields.CTime), nil
	case "signal.target.file.filesystem":
		return e.Signal.Target.Process.FileEvent.Filesystem, nil
	case "signal.target.file.gid":
		return int(e.Signal.Target.Process.FileEvent.FileFields.GID), nil
	case "signal.target.file.group":
		return e.Signal.Target.Process.FileEvent.FileFields.Group, nil
	case "signal.target.file.in_upper_layer":
		return e.Signal.Target.Process.FileEvent.FileFields.InUpperLayer, nil
	case "signal.target.file.inode":
		return int(e.Signal.Target.Process.FileEvent.FileFields.Inode), nil
	case "signal.target.file.mode":
		return int(e.Signal.Target.Process.FileEvent.FileFields.Mode), nil
	case "signal.target.file.modification_time":
		return int(e.Signal.Target.Process.FileEvent.FileFields.MTime), nil
	case "signal.target.file.mount_id":
		return int(e.Signal.Target.Process.FileEvent.FileFields.MountID), nil
	case "signal.target.file.name":
		return e.Signal.Target.Process.FileEvent.BasenameStr, nil
	case "signal.target.file.name.length":
		return len(e.Signal.Target.Process.FileEvent.BasenameStr), nil
	case "signal.target.file.path":
		return e.Signal.Target.Process.FileEvent.PathnameStr, nil
	case "signal.target.file.path.length":
		return len(e.Signal.Target.Process.FileEvent.PathnameStr), nil
	case "signal.target.file.rights":
		return int(e.Signal.Target.Process.FileEvent.FileFields.Mode), nil
	case "signal.target.file.uid":
		return int(e.Signal.Target.Process.FileEvent.FileFields.UID), nil
	case "signal.target.file.user":
		return e.Signal.Target.Process.FileEvent.FileFields.User, nil
	case "signal.target.fsgid":
		return int(e.Signal.Target.Process.Credentials.FSGID), nil
	case "signal.target.fsgroup":
		return e.Signal.Target.Process.Credentials.FSGroup, nil
	case "signal.target.fsuid":
		return int(e.Signal.Target.Process.Credentials.FSUID), nil
	case "signal.target.fsuser":
		return e.Signal.Target.Process.Credentials.FSUser, nil
	case "signal.target.gid":
		return int(e.Signal.Target.Process.Credentials.GID), nil
	case "signal.target.group":
		return e.Signal.Target.Process.Credentials.Group, nil
	case "signal.target.interpreter.file.change_time":
		return int(e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime), nil
	case "signal.target.interpreter.file.filesystem":
		return e.Signal.Target.Process.LinuxBinprm.FileEvent.Filesystem, nil
	case "signal.target.interpreter.file.gid":
		return int(e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID), nil
	case "signal.target.interpreter.file.group":
		return e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Group, nil
	case "signal.target.interpreter.file.in_upper_layer":
		return e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, nil
	case "signal.target.interpreter.file.inode":
		return int(e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Inode), nil
	case "signal.target.interpreter.file.mode":
		return int(e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "signal.target.interpreter.file.modification_time":
		return int(e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime), nil
	case "signal.target.interpreter.file.mount_id":
		return int(e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MountID), nil
	case "signal.target.interpreter.file.name":
		return e.Signal.Target.Process.LinuxBinprm.FileEvent.BasenameStr, nil
	case "signal.target.interpreter.file.name.length":
		return len(e.Signal.Target.Process.LinuxBinprm.FileEvent.BasenameStr), nil
	case "signal.target.interpreter.file.path":
		return e.Signal.Target.Process.LinuxBinprm.FileEvent.PathnameStr, nil
	case "signal.target.interpreter.file.path.length":
		return len(e.Signal.Target.Process.LinuxBinprm.FileEvent.PathnameStr), nil
	case "signal.target.interpreter.file.rights":
		return int(e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode), nil
	case "signal.target.interpreter.file.uid":
		return int(e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID), nil
	case "signal.target.interpreter.file.user":
		return e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.User, nil
	case "signal.target.is_kworker":
		return e.Signal.Target.Process.PIDContext.IsKworker, nil
	case "signal.target.is_thread":
		return e.Signal.Target.Process.IsThread, nil
	case "signal.target.pid":
		return int(e.Signal.Target.Process.PIDContext.Pid), nil
	case "signal.target.ppid":
		return int(e.Signal.Target.Process.PPid), nil
	case "signal.target.tid":
		return int(e.Signal.Target.Process.PIDContext.Tid), nil
	case "signal.target.tty_name":
		return e.Signal.Target.Process.TTYName, nil
	case "signal.target.uid":
		return int(e.Signal.Target.Process.Credentials.UID), nil
	case "signal.target.user":
		return e.Signal.Target.Process.Credentials.User, nil
	case "signal.type":
		return int(e.Signal.Type), nil
	case "splice.file.change_time":
		return int(e.Splice.File.FileFields.CTime), nil
	case "splice.file.filesystem":
		return e.Splice.File.Filesystem, nil
	case "splice.file.gid":
		return int(e.Splice.File.FileFields.GID), nil
	case "splice.file.group":
		return e.Splice.File.FileFields.Group, nil
	case "splice.file.in_upper_layer":
		return e.Splice.File.FileFields.InUpperLayer, nil
	case "splice.file.inode":
		return int(e.Splice.File.FileFields.Inode), nil
	case "splice.file.mode":
		return int(e.Splice.File.FileFields.Mode), nil
	case "splice.file.modification_time":
		return int(e.Splice.File.FileFields.MTime), nil
	case "splice.file.mount_id":
		return int(e.Splice.File.FileFields.MountID), nil
	case "splice.file.name":
		return e.Splice.File.BasenameStr, nil
	case "splice.file.name.length":
		return len(e.Splice.File.BasenameStr), nil
	case "splice.file.path":
		return e.Splice.File.PathnameStr, nil
	case "splice.file.path.length":
		return len(e.Splice.File.PathnameStr), nil
	case "splice.file.rights":
		return int(e.Splice.File.FileFields.Mode), nil
	case "splice.file.uid":
		return int(e.Splice.File.FileFields.UID), nil
	case "splice.file.user":
		return e.Splice.File.FileFields.User, nil
	case "splice.pipe_entry_flag":
		return int(e.Splice.PipeEntryFlag), nil
	case "splice.pipe_exit_flag":
		return int(e.Splice.PipeExitFlag), nil
	case "splice.retval":
		return int(e.Splice.SyscallEvent.Retval), nil
	case "unlink.file.change_time":
		return int(e.Unlink.File.FileFields.CTime), nil
	case "unlink.file.filesystem":
		return e.Unlink.File.Filesystem, nil
	case "unlink.file.gid":
		return int(e.Unlink.File.FileFields.GID), nil
	case "unlink.file.group":
		return e.Unlink.File.FileFields.Group, nil
	case "unlink.file.in_upper_layer":
		return e.Unlink.File.FileFields.InUpperLayer, nil
	case "unlink.file.inode":
		return int(e.Unlink.File.FileFields.Inode), nil
	case "unlink.file.mode":
		return int(e.Unlink.File.FileFields.Mode), nil
	case "unlink.file.modification_time":
		return int(e.Unlink.File.FileFields.MTime), nil
	case "unlink.file.mount_id":
		return int(e.Unlink.File.FileFields.MountID), nil
	case "unlink.file.name":
		return e.Unlink.File.BasenameStr, nil
	case "unlink.file.name.length":
		return len(e.Unlink.File.BasenameStr), nil
	case "unlink.file.path":
		return e.Unlink.File.PathnameStr, nil
	case "unlink.file.path.length":
		return len(e.Unlink.File.PathnameStr), nil
	case "unlink.file.rights":
		return int(e.Unlink.File.FileFields.Mode), nil
	case "unlink.file.uid":
		return int(e.Unlink.File.FileFields.UID), nil
	case "unlink.file.user":
		return e.Unlink.File.FileFields.User, nil
	case "unlink.flags":
		return int(e.Unlink.Flags), nil
	case "unlink.retval":
		return int(e.Unlink.SyscallEvent.Retval), nil
	case "unload_module.name":
		return e.UnloadModule.Name, nil
	case "unload_module.retval":
		return int(e.UnloadModule.SyscallEvent.Retval), nil
	case "utimes.file.change_time":
		return int(e.Utimes.File.FileFields.CTime), nil
	case "utimes.file.filesystem":
		return e.Utimes.File.Filesystem, nil
	case "utimes.file.gid":
		return int(e.Utimes.File.FileFields.GID), nil
	case "utimes.file.group":
		return e.Utimes.File.FileFields.Group, nil
	case "utimes.file.in_upper_layer":
		return e.Utimes.File.FileFields.InUpperLayer, nil
	case "utimes.file.inode":
		return int(e.Utimes.File.FileFields.Inode), nil
	case "utimes.file.mode":
		return int(e.Utimes.File.FileFields.Mode), nil
	case "utimes.file.modification_time":
		return int(e.Utimes.File.FileFields.MTime), nil
	case "utimes.file.mount_id":
		return int(e.Utimes.File.FileFields.MountID), nil
	case "utimes.file.name":
		return e.Utimes.File.BasenameStr, nil
	case "utimes.file.name.length":
		return len(e.Utimes.File.BasenameStr), nil
	case "utimes.file.path":
		return e.Utimes.File.PathnameStr, nil
	case "utimes.file.path.length":
		return len(e.Utimes.File.PathnameStr), nil
	case "utimes.file.rights":
		return int(e.Utimes.File.FileFields.Mode), nil
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
	case "async":
		return "*", nil
	case "bind.addr.family":
		return "bind", nil
	case "bind.addr.ip":
		return "bind", nil
	case "bind.addr.port":
		return "bind", nil
	case "bind.retval":
		return "bind", nil
	case "bpf.cmd":
		return "bpf", nil
	case "bpf.map.name":
		return "bpf", nil
	case "bpf.map.type":
		return "bpf", nil
	case "bpf.prog.attach_type":
		return "bpf", nil
	case "bpf.prog.helpers":
		return "bpf", nil
	case "bpf.prog.name":
		return "bpf", nil
	case "bpf.prog.tag":
		return "bpf", nil
	case "bpf.prog.type":
		return "bpf", nil
	case "bpf.retval":
		return "bpf", nil
	case "capset.cap_effective":
		return "capset", nil
	case "capset.cap_permitted":
		return "capset", nil
	case "chmod.file.change_time":
		return "chmod", nil
	case "chmod.file.destination.mode":
		return "chmod", nil
	case "chmod.file.destination.rights":
		return "chmod", nil
	case "chmod.file.filesystem":
		return "chmod", nil
	case "chmod.file.gid":
		return "chmod", nil
	case "chmod.file.group":
		return "chmod", nil
	case "chmod.file.in_upper_layer":
		return "chmod", nil
	case "chmod.file.inode":
		return "chmod", nil
	case "chmod.file.mode":
		return "chmod", nil
	case "chmod.file.modification_time":
		return "chmod", nil
	case "chmod.file.mount_id":
		return "chmod", nil
	case "chmod.file.name":
		return "chmod", nil
	case "chmod.file.name.length":
		return "chmod", nil
	case "chmod.file.path":
		return "chmod", nil
	case "chmod.file.path.length":
		return "chmod", nil
	case "chmod.file.rights":
		return "chmod", nil
	case "chmod.file.uid":
		return "chmod", nil
	case "chmod.file.user":
		return "chmod", nil
	case "chmod.retval":
		return "chmod", nil
	case "chown.file.change_time":
		return "chown", nil
	case "chown.file.destination.gid":
		return "chown", nil
	case "chown.file.destination.group":
		return "chown", nil
	case "chown.file.destination.uid":
		return "chown", nil
	case "chown.file.destination.user":
		return "chown", nil
	case "chown.file.filesystem":
		return "chown", nil
	case "chown.file.gid":
		return "chown", nil
	case "chown.file.group":
		return "chown", nil
	case "chown.file.in_upper_layer":
		return "chown", nil
	case "chown.file.inode":
		return "chown", nil
	case "chown.file.mode":
		return "chown", nil
	case "chown.file.modification_time":
		return "chown", nil
	case "chown.file.mount_id":
		return "chown", nil
	case "chown.file.name":
		return "chown", nil
	case "chown.file.name.length":
		return "chown", nil
	case "chown.file.path":
		return "chown", nil
	case "chown.file.path.length":
		return "chown", nil
	case "chown.file.rights":
		return "chown", nil
	case "chown.file.uid":
		return "chown", nil
	case "chown.file.user":
		return "chown", nil
	case "chown.retval":
		return "chown", nil
	case "container.id":
		return "*", nil
	case "container.tags":
		return "*", nil
	case "dns.id":
		return "dns", nil
	case "dns.question.class":
		return "dns", nil
	case "dns.question.count":
		return "dns", nil
	case "dns.question.length":
		return "dns", nil
	case "dns.question.name":
		return "dns", nil
	case "dns.question.name.length":
		return "dns", nil
	case "dns.question.type":
		return "dns", nil
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
	case "link.file.change_time":
		return "link", nil
	case "link.file.destination.change_time":
		return "link", nil
	case "link.file.destination.filesystem":
		return "link", nil
	case "link.file.destination.gid":
		return "link", nil
	case "link.file.destination.group":
		return "link", nil
	case "link.file.destination.in_upper_layer":
		return "link", nil
	case "link.file.destination.inode":
		return "link", nil
	case "link.file.destination.mode":
		return "link", nil
	case "link.file.destination.modification_time":
		return "link", nil
	case "link.file.destination.mount_id":
		return "link", nil
	case "link.file.destination.name":
		return "link", nil
	case "link.file.destination.name.length":
		return "link", nil
	case "link.file.destination.path":
		return "link", nil
	case "link.file.destination.path.length":
		return "link", nil
	case "link.file.destination.rights":
		return "link", nil
	case "link.file.destination.uid":
		return "link", nil
	case "link.file.destination.user":
		return "link", nil
	case "link.file.filesystem":
		return "link", nil
	case "link.file.gid":
		return "link", nil
	case "link.file.group":
		return "link", nil
	case "link.file.in_upper_layer":
		return "link", nil
	case "link.file.inode":
		return "link", nil
	case "link.file.mode":
		return "link", nil
	case "link.file.modification_time":
		return "link", nil
	case "link.file.mount_id":
		return "link", nil
	case "link.file.name":
		return "link", nil
	case "link.file.name.length":
		return "link", nil
	case "link.file.path":
		return "link", nil
	case "link.file.path.length":
		return "link", nil
	case "link.file.rights":
		return "link", nil
	case "link.file.uid":
		return "link", nil
	case "link.file.user":
		return "link", nil
	case "link.retval":
		return "link", nil
	case "load_module.file.change_time":
		return "load_module", nil
	case "load_module.file.filesystem":
		return "load_module", nil
	case "load_module.file.gid":
		return "load_module", nil
	case "load_module.file.group":
		return "load_module", nil
	case "load_module.file.in_upper_layer":
		return "load_module", nil
	case "load_module.file.inode":
		return "load_module", nil
	case "load_module.file.mode":
		return "load_module", nil
	case "load_module.file.modification_time":
		return "load_module", nil
	case "load_module.file.mount_id":
		return "load_module", nil
	case "load_module.file.name":
		return "load_module", nil
	case "load_module.file.name.length":
		return "load_module", nil
	case "load_module.file.path":
		return "load_module", nil
	case "load_module.file.path.length":
		return "load_module", nil
	case "load_module.file.rights":
		return "load_module", nil
	case "load_module.file.uid":
		return "load_module", nil
	case "load_module.file.user":
		return "load_module", nil
	case "load_module.loaded_from_memory":
		return "load_module", nil
	case "load_module.name":
		return "load_module", nil
	case "load_module.retval":
		return "load_module", nil
	case "mkdir.file.change_time":
		return "mkdir", nil
	case "mkdir.file.destination.mode":
		return "mkdir", nil
	case "mkdir.file.destination.rights":
		return "mkdir", nil
	case "mkdir.file.filesystem":
		return "mkdir", nil
	case "mkdir.file.gid":
		return "mkdir", nil
	case "mkdir.file.group":
		return "mkdir", nil
	case "mkdir.file.in_upper_layer":
		return "mkdir", nil
	case "mkdir.file.inode":
		return "mkdir", nil
	case "mkdir.file.mode":
		return "mkdir", nil
	case "mkdir.file.modification_time":
		return "mkdir", nil
	case "mkdir.file.mount_id":
		return "mkdir", nil
	case "mkdir.file.name":
		return "mkdir", nil
	case "mkdir.file.name.length":
		return "mkdir", nil
	case "mkdir.file.path":
		return "mkdir", nil
	case "mkdir.file.path.length":
		return "mkdir", nil
	case "mkdir.file.rights":
		return "mkdir", nil
	case "mkdir.file.uid":
		return "mkdir", nil
	case "mkdir.file.user":
		return "mkdir", nil
	case "mkdir.retval":
		return "mkdir", nil
	case "mmap.file.change_time":
		return "mmap", nil
	case "mmap.file.filesystem":
		return "mmap", nil
	case "mmap.file.gid":
		return "mmap", nil
	case "mmap.file.group":
		return "mmap", nil
	case "mmap.file.in_upper_layer":
		return "mmap", nil
	case "mmap.file.inode":
		return "mmap", nil
	case "mmap.file.mode":
		return "mmap", nil
	case "mmap.file.modification_time":
		return "mmap", nil
	case "mmap.file.mount_id":
		return "mmap", nil
	case "mmap.file.name":
		return "mmap", nil
	case "mmap.file.name.length":
		return "mmap", nil
	case "mmap.file.path":
		return "mmap", nil
	case "mmap.file.path.length":
		return "mmap", nil
	case "mmap.file.rights":
		return "mmap", nil
	case "mmap.file.uid":
		return "mmap", nil
	case "mmap.file.user":
		return "mmap", nil
	case "mmap.flags":
		return "mmap", nil
	case "mmap.protection":
		return "mmap", nil
	case "mmap.retval":
		return "mmap", nil
	case "mprotect.req_protection":
		return "mprotect", nil
	case "mprotect.retval":
		return "mprotect", nil
	case "mprotect.vm_protection":
		return "mprotect", nil
	case "network.destination.ip":
		return "*", nil
	case "network.destination.port":
		return "*", nil
	case "network.device.ifindex":
		return "*", nil
	case "network.device.ifname":
		return "*", nil
	case "network.l3_protocol":
		return "*", nil
	case "network.l4_protocol":
		return "*", nil
	case "network.size":
		return "*", nil
	case "network.source.ip":
		return "*", nil
	case "network.source.port":
		return "*", nil
	case "open.file.change_time":
		return "open", nil
	case "open.file.destination.mode":
		return "open", nil
	case "open.file.filesystem":
		return "open", nil
	case "open.file.gid":
		return "open", nil
	case "open.file.group":
		return "open", nil
	case "open.file.in_upper_layer":
		return "open", nil
	case "open.file.inode":
		return "open", nil
	case "open.file.mode":
		return "open", nil
	case "open.file.modification_time":
		return "open", nil
	case "open.file.mount_id":
		return "open", nil
	case "open.file.name":
		return "open", nil
	case "open.file.name.length":
		return "open", nil
	case "open.file.path":
		return "open", nil
	case "open.file.path.length":
		return "open", nil
	case "open.file.rights":
		return "open", nil
	case "open.file.uid":
		return "open", nil
	case "open.file.user":
		return "open", nil
	case "open.flags":
		return "open", nil
	case "open.retval":
		return "open", nil
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
	case "ptrace.request":
		return "ptrace", nil
	case "ptrace.retval":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.args":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.args_flags":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.args_options":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.args_truncated":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.argv":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.argv0":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.cap_effective":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.cap_permitted":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.comm":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.container.id":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.cookie":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.created_at":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.egid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.egroup":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.envp":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.envs":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.envs_truncated":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.euid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.euser":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.change_time":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.filesystem":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.gid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.group":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.in_upper_layer":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.inode":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.mode":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.modification_time":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.mount_id":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.name":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.name.length":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.path":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.path.length":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.rights":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.uid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.file.user":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.fsgid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.fsgroup":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.fsuid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.fsuser":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.gid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.group":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.change_time":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.filesystem":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.gid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.group":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.in_upper_layer":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.inode":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.mode":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.modification_time":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.mount_id":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.name":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.name.length":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.path":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.path.length":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.rights":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.uid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.interpreter.file.user":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.is_kworker":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.is_thread":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.pid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.ppid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.tid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.tty_name":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.uid":
		return "ptrace", nil
	case "ptrace.tracee.ancestors.user":
		return "ptrace", nil
	case "ptrace.tracee.args":
		return "ptrace", nil
	case "ptrace.tracee.args_flags":
		return "ptrace", nil
	case "ptrace.tracee.args_options":
		return "ptrace", nil
	case "ptrace.tracee.args_truncated":
		return "ptrace", nil
	case "ptrace.tracee.argv":
		return "ptrace", nil
	case "ptrace.tracee.argv0":
		return "ptrace", nil
	case "ptrace.tracee.cap_effective":
		return "ptrace", nil
	case "ptrace.tracee.cap_permitted":
		return "ptrace", nil
	case "ptrace.tracee.comm":
		return "ptrace", nil
	case "ptrace.tracee.container.id":
		return "ptrace", nil
	case "ptrace.tracee.cookie":
		return "ptrace", nil
	case "ptrace.tracee.created_at":
		return "ptrace", nil
	case "ptrace.tracee.egid":
		return "ptrace", nil
	case "ptrace.tracee.egroup":
		return "ptrace", nil
	case "ptrace.tracee.envp":
		return "ptrace", nil
	case "ptrace.tracee.envs":
		return "ptrace", nil
	case "ptrace.tracee.envs_truncated":
		return "ptrace", nil
	case "ptrace.tracee.euid":
		return "ptrace", nil
	case "ptrace.tracee.euser":
		return "ptrace", nil
	case "ptrace.tracee.file.change_time":
		return "ptrace", nil
	case "ptrace.tracee.file.filesystem":
		return "ptrace", nil
	case "ptrace.tracee.file.gid":
		return "ptrace", nil
	case "ptrace.tracee.file.group":
		return "ptrace", nil
	case "ptrace.tracee.file.in_upper_layer":
		return "ptrace", nil
	case "ptrace.tracee.file.inode":
		return "ptrace", nil
	case "ptrace.tracee.file.mode":
		return "ptrace", nil
	case "ptrace.tracee.file.modification_time":
		return "ptrace", nil
	case "ptrace.tracee.file.mount_id":
		return "ptrace", nil
	case "ptrace.tracee.file.name":
		return "ptrace", nil
	case "ptrace.tracee.file.name.length":
		return "ptrace", nil
	case "ptrace.tracee.file.path":
		return "ptrace", nil
	case "ptrace.tracee.file.path.length":
		return "ptrace", nil
	case "ptrace.tracee.file.rights":
		return "ptrace", nil
	case "ptrace.tracee.file.uid":
		return "ptrace", nil
	case "ptrace.tracee.file.user":
		return "ptrace", nil
	case "ptrace.tracee.fsgid":
		return "ptrace", nil
	case "ptrace.tracee.fsgroup":
		return "ptrace", nil
	case "ptrace.tracee.fsuid":
		return "ptrace", nil
	case "ptrace.tracee.fsuser":
		return "ptrace", nil
	case "ptrace.tracee.gid":
		return "ptrace", nil
	case "ptrace.tracee.group":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.change_time":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.filesystem":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.gid":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.group":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.in_upper_layer":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.inode":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.mode":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.modification_time":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.mount_id":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.name":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.name.length":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.path":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.path.length":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.rights":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.uid":
		return "ptrace", nil
	case "ptrace.tracee.interpreter.file.user":
		return "ptrace", nil
	case "ptrace.tracee.is_kworker":
		return "ptrace", nil
	case "ptrace.tracee.is_thread":
		return "ptrace", nil
	case "ptrace.tracee.pid":
		return "ptrace", nil
	case "ptrace.tracee.ppid":
		return "ptrace", nil
	case "ptrace.tracee.tid":
		return "ptrace", nil
	case "ptrace.tracee.tty_name":
		return "ptrace", nil
	case "ptrace.tracee.uid":
		return "ptrace", nil
	case "ptrace.tracee.user":
		return "ptrace", nil
	case "removexattr.file.change_time":
		return "removexattr", nil
	case "removexattr.file.destination.name":
		return "removexattr", nil
	case "removexattr.file.destination.namespace":
		return "removexattr", nil
	case "removexattr.file.filesystem":
		return "removexattr", nil
	case "removexattr.file.gid":
		return "removexattr", nil
	case "removexattr.file.group":
		return "removexattr", nil
	case "removexattr.file.in_upper_layer":
		return "removexattr", nil
	case "removexattr.file.inode":
		return "removexattr", nil
	case "removexattr.file.mode":
		return "removexattr", nil
	case "removexattr.file.modification_time":
		return "removexattr", nil
	case "removexattr.file.mount_id":
		return "removexattr", nil
	case "removexattr.file.name":
		return "removexattr", nil
	case "removexattr.file.name.length":
		return "removexattr", nil
	case "removexattr.file.path":
		return "removexattr", nil
	case "removexattr.file.path.length":
		return "removexattr", nil
	case "removexattr.file.rights":
		return "removexattr", nil
	case "removexattr.file.uid":
		return "removexattr", nil
	case "removexattr.file.user":
		return "removexattr", nil
	case "removexattr.retval":
		return "removexattr", nil
	case "rename.file.change_time":
		return "rename", nil
	case "rename.file.destination.change_time":
		return "rename", nil
	case "rename.file.destination.filesystem":
		return "rename", nil
	case "rename.file.destination.gid":
		return "rename", nil
	case "rename.file.destination.group":
		return "rename", nil
	case "rename.file.destination.in_upper_layer":
		return "rename", nil
	case "rename.file.destination.inode":
		return "rename", nil
	case "rename.file.destination.mode":
		return "rename", nil
	case "rename.file.destination.modification_time":
		return "rename", nil
	case "rename.file.destination.mount_id":
		return "rename", nil
	case "rename.file.destination.name":
		return "rename", nil
	case "rename.file.destination.name.length":
		return "rename", nil
	case "rename.file.destination.path":
		return "rename", nil
	case "rename.file.destination.path.length":
		return "rename", nil
	case "rename.file.destination.rights":
		return "rename", nil
	case "rename.file.destination.uid":
		return "rename", nil
	case "rename.file.destination.user":
		return "rename", nil
	case "rename.file.filesystem":
		return "rename", nil
	case "rename.file.gid":
		return "rename", nil
	case "rename.file.group":
		return "rename", nil
	case "rename.file.in_upper_layer":
		return "rename", nil
	case "rename.file.inode":
		return "rename", nil
	case "rename.file.mode":
		return "rename", nil
	case "rename.file.modification_time":
		return "rename", nil
	case "rename.file.mount_id":
		return "rename", nil
	case "rename.file.name":
		return "rename", nil
	case "rename.file.name.length":
		return "rename", nil
	case "rename.file.path":
		return "rename", nil
	case "rename.file.path.length":
		return "rename", nil
	case "rename.file.rights":
		return "rename", nil
	case "rename.file.uid":
		return "rename", nil
	case "rename.file.user":
		return "rename", nil
	case "rename.retval":
		return "rename", nil
	case "rmdir.file.change_time":
		return "rmdir", nil
	case "rmdir.file.filesystem":
		return "rmdir", nil
	case "rmdir.file.gid":
		return "rmdir", nil
	case "rmdir.file.group":
		return "rmdir", nil
	case "rmdir.file.in_upper_layer":
		return "rmdir", nil
	case "rmdir.file.inode":
		return "rmdir", nil
	case "rmdir.file.mode":
		return "rmdir", nil
	case "rmdir.file.modification_time":
		return "rmdir", nil
	case "rmdir.file.mount_id":
		return "rmdir", nil
	case "rmdir.file.name":
		return "rmdir", nil
	case "rmdir.file.name.length":
		return "rmdir", nil
	case "rmdir.file.path":
		return "rmdir", nil
	case "rmdir.file.path.length":
		return "rmdir", nil
	case "rmdir.file.rights":
		return "rmdir", nil
	case "rmdir.file.uid":
		return "rmdir", nil
	case "rmdir.file.user":
		return "rmdir", nil
	case "rmdir.retval":
		return "rmdir", nil
	case "selinux.bool.name":
		return "selinux", nil
	case "selinux.bool.state":
		return "selinux", nil
	case "selinux.bool_commit.state":
		return "selinux", nil
	case "selinux.enforce.status":
		return "selinux", nil
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
	case "setxattr.file.change_time":
		return "setxattr", nil
	case "setxattr.file.destination.name":
		return "setxattr", nil
	case "setxattr.file.destination.namespace":
		return "setxattr", nil
	case "setxattr.file.filesystem":
		return "setxattr", nil
	case "setxattr.file.gid":
		return "setxattr", nil
	case "setxattr.file.group":
		return "setxattr", nil
	case "setxattr.file.in_upper_layer":
		return "setxattr", nil
	case "setxattr.file.inode":
		return "setxattr", nil
	case "setxattr.file.mode":
		return "setxattr", nil
	case "setxattr.file.modification_time":
		return "setxattr", nil
	case "setxattr.file.mount_id":
		return "setxattr", nil
	case "setxattr.file.name":
		return "setxattr", nil
	case "setxattr.file.name.length":
		return "setxattr", nil
	case "setxattr.file.path":
		return "setxattr", nil
	case "setxattr.file.path.length":
		return "setxattr", nil
	case "setxattr.file.rights":
		return "setxattr", nil
	case "setxattr.file.uid":
		return "setxattr", nil
	case "setxattr.file.user":
		return "setxattr", nil
	case "setxattr.retval":
		return "setxattr", nil
	case "signal.pid":
		return "signal", nil
	case "signal.retval":
		return "signal", nil
	case "signal.target.ancestors.args":
		return "signal", nil
	case "signal.target.ancestors.args_flags":
		return "signal", nil
	case "signal.target.ancestors.args_options":
		return "signal", nil
	case "signal.target.ancestors.args_truncated":
		return "signal", nil
	case "signal.target.ancestors.argv":
		return "signal", nil
	case "signal.target.ancestors.argv0":
		return "signal", nil
	case "signal.target.ancestors.cap_effective":
		return "signal", nil
	case "signal.target.ancestors.cap_permitted":
		return "signal", nil
	case "signal.target.ancestors.comm":
		return "signal", nil
	case "signal.target.ancestors.container.id":
		return "signal", nil
	case "signal.target.ancestors.cookie":
		return "signal", nil
	case "signal.target.ancestors.created_at":
		return "signal", nil
	case "signal.target.ancestors.egid":
		return "signal", nil
	case "signal.target.ancestors.egroup":
		return "signal", nil
	case "signal.target.ancestors.envp":
		return "signal", nil
	case "signal.target.ancestors.envs":
		return "signal", nil
	case "signal.target.ancestors.envs_truncated":
		return "signal", nil
	case "signal.target.ancestors.euid":
		return "signal", nil
	case "signal.target.ancestors.euser":
		return "signal", nil
	case "signal.target.ancestors.file.change_time":
		return "signal", nil
	case "signal.target.ancestors.file.filesystem":
		return "signal", nil
	case "signal.target.ancestors.file.gid":
		return "signal", nil
	case "signal.target.ancestors.file.group":
		return "signal", nil
	case "signal.target.ancestors.file.in_upper_layer":
		return "signal", nil
	case "signal.target.ancestors.file.inode":
		return "signal", nil
	case "signal.target.ancestors.file.mode":
		return "signal", nil
	case "signal.target.ancestors.file.modification_time":
		return "signal", nil
	case "signal.target.ancestors.file.mount_id":
		return "signal", nil
	case "signal.target.ancestors.file.name":
		return "signal", nil
	case "signal.target.ancestors.file.name.length":
		return "signal", nil
	case "signal.target.ancestors.file.path":
		return "signal", nil
	case "signal.target.ancestors.file.path.length":
		return "signal", nil
	case "signal.target.ancestors.file.rights":
		return "signal", nil
	case "signal.target.ancestors.file.uid":
		return "signal", nil
	case "signal.target.ancestors.file.user":
		return "signal", nil
	case "signal.target.ancestors.fsgid":
		return "signal", nil
	case "signal.target.ancestors.fsgroup":
		return "signal", nil
	case "signal.target.ancestors.fsuid":
		return "signal", nil
	case "signal.target.ancestors.fsuser":
		return "signal", nil
	case "signal.target.ancestors.gid":
		return "signal", nil
	case "signal.target.ancestors.group":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.change_time":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.filesystem":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.gid":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.group":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.in_upper_layer":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.inode":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.mode":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.modification_time":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.mount_id":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.name":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.name.length":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.path":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.path.length":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.rights":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.uid":
		return "signal", nil
	case "signal.target.ancestors.interpreter.file.user":
		return "signal", nil
	case "signal.target.ancestors.is_kworker":
		return "signal", nil
	case "signal.target.ancestors.is_thread":
		return "signal", nil
	case "signal.target.ancestors.pid":
		return "signal", nil
	case "signal.target.ancestors.ppid":
		return "signal", nil
	case "signal.target.ancestors.tid":
		return "signal", nil
	case "signal.target.ancestors.tty_name":
		return "signal", nil
	case "signal.target.ancestors.uid":
		return "signal", nil
	case "signal.target.ancestors.user":
		return "signal", nil
	case "signal.target.args":
		return "signal", nil
	case "signal.target.args_flags":
		return "signal", nil
	case "signal.target.args_options":
		return "signal", nil
	case "signal.target.args_truncated":
		return "signal", nil
	case "signal.target.argv":
		return "signal", nil
	case "signal.target.argv0":
		return "signal", nil
	case "signal.target.cap_effective":
		return "signal", nil
	case "signal.target.cap_permitted":
		return "signal", nil
	case "signal.target.comm":
		return "signal", nil
	case "signal.target.container.id":
		return "signal", nil
	case "signal.target.cookie":
		return "signal", nil
	case "signal.target.created_at":
		return "signal", nil
	case "signal.target.egid":
		return "signal", nil
	case "signal.target.egroup":
		return "signal", nil
	case "signal.target.envp":
		return "signal", nil
	case "signal.target.envs":
		return "signal", nil
	case "signal.target.envs_truncated":
		return "signal", nil
	case "signal.target.euid":
		return "signal", nil
	case "signal.target.euser":
		return "signal", nil
	case "signal.target.file.change_time":
		return "signal", nil
	case "signal.target.file.filesystem":
		return "signal", nil
	case "signal.target.file.gid":
		return "signal", nil
	case "signal.target.file.group":
		return "signal", nil
	case "signal.target.file.in_upper_layer":
		return "signal", nil
	case "signal.target.file.inode":
		return "signal", nil
	case "signal.target.file.mode":
		return "signal", nil
	case "signal.target.file.modification_time":
		return "signal", nil
	case "signal.target.file.mount_id":
		return "signal", nil
	case "signal.target.file.name":
		return "signal", nil
	case "signal.target.file.name.length":
		return "signal", nil
	case "signal.target.file.path":
		return "signal", nil
	case "signal.target.file.path.length":
		return "signal", nil
	case "signal.target.file.rights":
		return "signal", nil
	case "signal.target.file.uid":
		return "signal", nil
	case "signal.target.file.user":
		return "signal", nil
	case "signal.target.fsgid":
		return "signal", nil
	case "signal.target.fsgroup":
		return "signal", nil
	case "signal.target.fsuid":
		return "signal", nil
	case "signal.target.fsuser":
		return "signal", nil
	case "signal.target.gid":
		return "signal", nil
	case "signal.target.group":
		return "signal", nil
	case "signal.target.interpreter.file.change_time":
		return "signal", nil
	case "signal.target.interpreter.file.filesystem":
		return "signal", nil
	case "signal.target.interpreter.file.gid":
		return "signal", nil
	case "signal.target.interpreter.file.group":
		return "signal", nil
	case "signal.target.interpreter.file.in_upper_layer":
		return "signal", nil
	case "signal.target.interpreter.file.inode":
		return "signal", nil
	case "signal.target.interpreter.file.mode":
		return "signal", nil
	case "signal.target.interpreter.file.modification_time":
		return "signal", nil
	case "signal.target.interpreter.file.mount_id":
		return "signal", nil
	case "signal.target.interpreter.file.name":
		return "signal", nil
	case "signal.target.interpreter.file.name.length":
		return "signal", nil
	case "signal.target.interpreter.file.path":
		return "signal", nil
	case "signal.target.interpreter.file.path.length":
		return "signal", nil
	case "signal.target.interpreter.file.rights":
		return "signal", nil
	case "signal.target.interpreter.file.uid":
		return "signal", nil
	case "signal.target.interpreter.file.user":
		return "signal", nil
	case "signal.target.is_kworker":
		return "signal", nil
	case "signal.target.is_thread":
		return "signal", nil
	case "signal.target.pid":
		return "signal", nil
	case "signal.target.ppid":
		return "signal", nil
	case "signal.target.tid":
		return "signal", nil
	case "signal.target.tty_name":
		return "signal", nil
	case "signal.target.uid":
		return "signal", nil
	case "signal.target.user":
		return "signal", nil
	case "signal.type":
		return "signal", nil
	case "splice.file.change_time":
		return "splice", nil
	case "splice.file.filesystem":
		return "splice", nil
	case "splice.file.gid":
		return "splice", nil
	case "splice.file.group":
		return "splice", nil
	case "splice.file.in_upper_layer":
		return "splice", nil
	case "splice.file.inode":
		return "splice", nil
	case "splice.file.mode":
		return "splice", nil
	case "splice.file.modification_time":
		return "splice", nil
	case "splice.file.mount_id":
		return "splice", nil
	case "splice.file.name":
		return "splice", nil
	case "splice.file.name.length":
		return "splice", nil
	case "splice.file.path":
		return "splice", nil
	case "splice.file.path.length":
		return "splice", nil
	case "splice.file.rights":
		return "splice", nil
	case "splice.file.uid":
		return "splice", nil
	case "splice.file.user":
		return "splice", nil
	case "splice.pipe_entry_flag":
		return "splice", nil
	case "splice.pipe_exit_flag":
		return "splice", nil
	case "splice.retval":
		return "splice", nil
	case "unlink.file.change_time":
		return "unlink", nil
	case "unlink.file.filesystem":
		return "unlink", nil
	case "unlink.file.gid":
		return "unlink", nil
	case "unlink.file.group":
		return "unlink", nil
	case "unlink.file.in_upper_layer":
		return "unlink", nil
	case "unlink.file.inode":
		return "unlink", nil
	case "unlink.file.mode":
		return "unlink", nil
	case "unlink.file.modification_time":
		return "unlink", nil
	case "unlink.file.mount_id":
		return "unlink", nil
	case "unlink.file.name":
		return "unlink", nil
	case "unlink.file.name.length":
		return "unlink", nil
	case "unlink.file.path":
		return "unlink", nil
	case "unlink.file.path.length":
		return "unlink", nil
	case "unlink.file.rights":
		return "unlink", nil
	case "unlink.file.uid":
		return "unlink", nil
	case "unlink.file.user":
		return "unlink", nil
	case "unlink.flags":
		return "unlink", nil
	case "unlink.retval":
		return "unlink", nil
	case "unload_module.name":
		return "unload_module", nil
	case "unload_module.retval":
		return "unload_module", nil
	case "utimes.file.change_time":
		return "utimes", nil
	case "utimes.file.filesystem":
		return "utimes", nil
	case "utimes.file.gid":
		return "utimes", nil
	case "utimes.file.group":
		return "utimes", nil
	case "utimes.file.in_upper_layer":
		return "utimes", nil
	case "utimes.file.inode":
		return "utimes", nil
	case "utimes.file.mode":
		return "utimes", nil
	case "utimes.file.modification_time":
		return "utimes", nil
	case "utimes.file.mount_id":
		return "utimes", nil
	case "utimes.file.name":
		return "utimes", nil
	case "utimes.file.name.length":
		return "utimes", nil
	case "utimes.file.path":
		return "utimes", nil
	case "utimes.file.path.length":
		return "utimes", nil
	case "utimes.file.rights":
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
	case "async":
		return reflect.Bool, nil
	case "bind.addr.family":
		return reflect.Int, nil
	case "bind.addr.ip":
		return reflect.Struct, nil
	case "bind.addr.port":
		return reflect.Int, nil
	case "bind.retval":
		return reflect.Int, nil
	case "bpf.cmd":
		return reflect.Int, nil
	case "bpf.map.name":
		return reflect.String, nil
	case "bpf.map.type":
		return reflect.Int, nil
	case "bpf.prog.attach_type":
		return reflect.Int, nil
	case "bpf.prog.helpers":
		return reflect.Int, nil
	case "bpf.prog.name":
		return reflect.String, nil
	case "bpf.prog.tag":
		return reflect.String, nil
	case "bpf.prog.type":
		return reflect.Int, nil
	case "bpf.retval":
		return reflect.Int, nil
	case "capset.cap_effective":
		return reflect.Int, nil
	case "capset.cap_permitted":
		return reflect.Int, nil
	case "chmod.file.change_time":
		return reflect.Int, nil
	case "chmod.file.destination.mode":
		return reflect.Int, nil
	case "chmod.file.destination.rights":
		return reflect.Int, nil
	case "chmod.file.filesystem":
		return reflect.String, nil
	case "chmod.file.gid":
		return reflect.Int, nil
	case "chmod.file.group":
		return reflect.String, nil
	case "chmod.file.in_upper_layer":
		return reflect.Bool, nil
	case "chmod.file.inode":
		return reflect.Int, nil
	case "chmod.file.mode":
		return reflect.Int, nil
	case "chmod.file.modification_time":
		return reflect.Int, nil
	case "chmod.file.mount_id":
		return reflect.Int, nil
	case "chmod.file.name":
		return reflect.String, nil
	case "chmod.file.name.length":
		return reflect.Int, nil
	case "chmod.file.path":
		return reflect.String, nil
	case "chmod.file.path.length":
		return reflect.Int, nil
	case "chmod.file.rights":
		return reflect.Int, nil
	case "chmod.file.uid":
		return reflect.Int, nil
	case "chmod.file.user":
		return reflect.String, nil
	case "chmod.retval":
		return reflect.Int, nil
	case "chown.file.change_time":
		return reflect.Int, nil
	case "chown.file.destination.gid":
		return reflect.Int, nil
	case "chown.file.destination.group":
		return reflect.String, nil
	case "chown.file.destination.uid":
		return reflect.Int, nil
	case "chown.file.destination.user":
		return reflect.String, nil
	case "chown.file.filesystem":
		return reflect.String, nil
	case "chown.file.gid":
		return reflect.Int, nil
	case "chown.file.group":
		return reflect.String, nil
	case "chown.file.in_upper_layer":
		return reflect.Bool, nil
	case "chown.file.inode":
		return reflect.Int, nil
	case "chown.file.mode":
		return reflect.Int, nil
	case "chown.file.modification_time":
		return reflect.Int, nil
	case "chown.file.mount_id":
		return reflect.Int, nil
	case "chown.file.name":
		return reflect.String, nil
	case "chown.file.name.length":
		return reflect.Int, nil
	case "chown.file.path":
		return reflect.String, nil
	case "chown.file.path.length":
		return reflect.Int, nil
	case "chown.file.rights":
		return reflect.Int, nil
	case "chown.file.uid":
		return reflect.Int, nil
	case "chown.file.user":
		return reflect.String, nil
	case "chown.retval":
		return reflect.Int, nil
	case "container.id":
		return reflect.String, nil
	case "container.tags":
		return reflect.String, nil
	case "dns.id":
		return reflect.Int, nil
	case "dns.question.class":
		return reflect.Int, nil
	case "dns.question.count":
		return reflect.Int, nil
	case "dns.question.length":
		return reflect.Int, nil
	case "dns.question.name":
		return reflect.String, nil
	case "dns.question.name.length":
		return reflect.Int, nil
	case "dns.question.type":
		return reflect.Int, nil
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
	case "link.file.change_time":
		return reflect.Int, nil
	case "link.file.destination.change_time":
		return reflect.Int, nil
	case "link.file.destination.filesystem":
		return reflect.String, nil
	case "link.file.destination.gid":
		return reflect.Int, nil
	case "link.file.destination.group":
		return reflect.String, nil
	case "link.file.destination.in_upper_layer":
		return reflect.Bool, nil
	case "link.file.destination.inode":
		return reflect.Int, nil
	case "link.file.destination.mode":
		return reflect.Int, nil
	case "link.file.destination.modification_time":
		return reflect.Int, nil
	case "link.file.destination.mount_id":
		return reflect.Int, nil
	case "link.file.destination.name":
		return reflect.String, nil
	case "link.file.destination.name.length":
		return reflect.Int, nil
	case "link.file.destination.path":
		return reflect.String, nil
	case "link.file.destination.path.length":
		return reflect.Int, nil
	case "link.file.destination.rights":
		return reflect.Int, nil
	case "link.file.destination.uid":
		return reflect.Int, nil
	case "link.file.destination.user":
		return reflect.String, nil
	case "link.file.filesystem":
		return reflect.String, nil
	case "link.file.gid":
		return reflect.Int, nil
	case "link.file.group":
		return reflect.String, nil
	case "link.file.in_upper_layer":
		return reflect.Bool, nil
	case "link.file.inode":
		return reflect.Int, nil
	case "link.file.mode":
		return reflect.Int, nil
	case "link.file.modification_time":
		return reflect.Int, nil
	case "link.file.mount_id":
		return reflect.Int, nil
	case "link.file.name":
		return reflect.String, nil
	case "link.file.name.length":
		return reflect.Int, nil
	case "link.file.path":
		return reflect.String, nil
	case "link.file.path.length":
		return reflect.Int, nil
	case "link.file.rights":
		return reflect.Int, nil
	case "link.file.uid":
		return reflect.Int, nil
	case "link.file.user":
		return reflect.String, nil
	case "link.retval":
		return reflect.Int, nil
	case "load_module.file.change_time":
		return reflect.Int, nil
	case "load_module.file.filesystem":
		return reflect.String, nil
	case "load_module.file.gid":
		return reflect.Int, nil
	case "load_module.file.group":
		return reflect.String, nil
	case "load_module.file.in_upper_layer":
		return reflect.Bool, nil
	case "load_module.file.inode":
		return reflect.Int, nil
	case "load_module.file.mode":
		return reflect.Int, nil
	case "load_module.file.modification_time":
		return reflect.Int, nil
	case "load_module.file.mount_id":
		return reflect.Int, nil
	case "load_module.file.name":
		return reflect.String, nil
	case "load_module.file.name.length":
		return reflect.Int, nil
	case "load_module.file.path":
		return reflect.String, nil
	case "load_module.file.path.length":
		return reflect.Int, nil
	case "load_module.file.rights":
		return reflect.Int, nil
	case "load_module.file.uid":
		return reflect.Int, nil
	case "load_module.file.user":
		return reflect.String, nil
	case "load_module.loaded_from_memory":
		return reflect.Bool, nil
	case "load_module.name":
		return reflect.String, nil
	case "load_module.retval":
		return reflect.Int, nil
	case "mkdir.file.change_time":
		return reflect.Int, nil
	case "mkdir.file.destination.mode":
		return reflect.Int, nil
	case "mkdir.file.destination.rights":
		return reflect.Int, nil
	case "mkdir.file.filesystem":
		return reflect.String, nil
	case "mkdir.file.gid":
		return reflect.Int, nil
	case "mkdir.file.group":
		return reflect.String, nil
	case "mkdir.file.in_upper_layer":
		return reflect.Bool, nil
	case "mkdir.file.inode":
		return reflect.Int, nil
	case "mkdir.file.mode":
		return reflect.Int, nil
	case "mkdir.file.modification_time":
		return reflect.Int, nil
	case "mkdir.file.mount_id":
		return reflect.Int, nil
	case "mkdir.file.name":
		return reflect.String, nil
	case "mkdir.file.name.length":
		return reflect.Int, nil
	case "mkdir.file.path":
		return reflect.String, nil
	case "mkdir.file.path.length":
		return reflect.Int, nil
	case "mkdir.file.rights":
		return reflect.Int, nil
	case "mkdir.file.uid":
		return reflect.Int, nil
	case "mkdir.file.user":
		return reflect.String, nil
	case "mkdir.retval":
		return reflect.Int, nil
	case "mmap.file.change_time":
		return reflect.Int, nil
	case "mmap.file.filesystem":
		return reflect.String, nil
	case "mmap.file.gid":
		return reflect.Int, nil
	case "mmap.file.group":
		return reflect.String, nil
	case "mmap.file.in_upper_layer":
		return reflect.Bool, nil
	case "mmap.file.inode":
		return reflect.Int, nil
	case "mmap.file.mode":
		return reflect.Int, nil
	case "mmap.file.modification_time":
		return reflect.Int, nil
	case "mmap.file.mount_id":
		return reflect.Int, nil
	case "mmap.file.name":
		return reflect.String, nil
	case "mmap.file.name.length":
		return reflect.Int, nil
	case "mmap.file.path":
		return reflect.String, nil
	case "mmap.file.path.length":
		return reflect.Int, nil
	case "mmap.file.rights":
		return reflect.Int, nil
	case "mmap.file.uid":
		return reflect.Int, nil
	case "mmap.file.user":
		return reflect.String, nil
	case "mmap.flags":
		return reflect.Int, nil
	case "mmap.protection":
		return reflect.Int, nil
	case "mmap.retval":
		return reflect.Int, nil
	case "mprotect.req_protection":
		return reflect.Int, nil
	case "mprotect.retval":
		return reflect.Int, nil
	case "mprotect.vm_protection":
		return reflect.Int, nil
	case "network.destination.ip":
		return reflect.Struct, nil
	case "network.destination.port":
		return reflect.Int, nil
	case "network.device.ifindex":
		return reflect.Int, nil
	case "network.device.ifname":
		return reflect.String, nil
	case "network.l3_protocol":
		return reflect.Int, nil
	case "network.l4_protocol":
		return reflect.Int, nil
	case "network.size":
		return reflect.Int, nil
	case "network.source.ip":
		return reflect.Struct, nil
	case "network.source.port":
		return reflect.Int, nil
	case "open.file.change_time":
		return reflect.Int, nil
	case "open.file.destination.mode":
		return reflect.Int, nil
	case "open.file.filesystem":
		return reflect.String, nil
	case "open.file.gid":
		return reflect.Int, nil
	case "open.file.group":
		return reflect.String, nil
	case "open.file.in_upper_layer":
		return reflect.Bool, nil
	case "open.file.inode":
		return reflect.Int, nil
	case "open.file.mode":
		return reflect.Int, nil
	case "open.file.modification_time":
		return reflect.Int, nil
	case "open.file.mount_id":
		return reflect.Int, nil
	case "open.file.name":
		return reflect.String, nil
	case "open.file.name.length":
		return reflect.Int, nil
	case "open.file.path":
		return reflect.String, nil
	case "open.file.path.length":
		return reflect.Int, nil
	case "open.file.rights":
		return reflect.Int, nil
	case "open.file.uid":
		return reflect.Int, nil
	case "open.file.user":
		return reflect.String, nil
	case "open.flags":
		return reflect.Int, nil
	case "open.retval":
		return reflect.Int, nil
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
	case "ptrace.request":
		return reflect.Int, nil
	case "ptrace.retval":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.args":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.args_flags":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.args_options":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.args_truncated":
		return reflect.Bool, nil
	case "ptrace.tracee.ancestors.argv":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.argv0":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.cap_effective":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.cap_permitted":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.comm":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.container.id":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.cookie":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.created_at":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.egid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.egroup":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.envp":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.envs":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.envs_truncated":
		return reflect.Bool, nil
	case "ptrace.tracee.ancestors.euid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.euser":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.file.change_time":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.filesystem":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.file.gid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.group":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.file.in_upper_layer":
		return reflect.Bool, nil
	case "ptrace.tracee.ancestors.file.inode":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.mode":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.modification_time":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.mount_id":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.name":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.file.name.length":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.path":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.file.path.length":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.rights":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.uid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.file.user":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.fsgid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.fsgroup":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.fsuid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.fsuser":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.gid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.group":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.interpreter.file.change_time":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.filesystem":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.interpreter.file.gid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.group":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "ptrace.tracee.ancestors.interpreter.file.inode":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.mode":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.modification_time":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.mount_id":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.name":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.interpreter.file.name.length":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.path":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.interpreter.file.path.length":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.rights":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.uid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.interpreter.file.user":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.is_kworker":
		return reflect.Bool, nil
	case "ptrace.tracee.ancestors.is_thread":
		return reflect.Bool, nil
	case "ptrace.tracee.ancestors.pid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.ppid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.tid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.tty_name":
		return reflect.String, nil
	case "ptrace.tracee.ancestors.uid":
		return reflect.Int, nil
	case "ptrace.tracee.ancestors.user":
		return reflect.String, nil
	case "ptrace.tracee.args":
		return reflect.String, nil
	case "ptrace.tracee.args_flags":
		return reflect.String, nil
	case "ptrace.tracee.args_options":
		return reflect.String, nil
	case "ptrace.tracee.args_truncated":
		return reflect.Bool, nil
	case "ptrace.tracee.argv":
		return reflect.String, nil
	case "ptrace.tracee.argv0":
		return reflect.String, nil
	case "ptrace.tracee.cap_effective":
		return reflect.Int, nil
	case "ptrace.tracee.cap_permitted":
		return reflect.Int, nil
	case "ptrace.tracee.comm":
		return reflect.String, nil
	case "ptrace.tracee.container.id":
		return reflect.String, nil
	case "ptrace.tracee.cookie":
		return reflect.Int, nil
	case "ptrace.tracee.created_at":
		return reflect.Int, nil
	case "ptrace.tracee.egid":
		return reflect.Int, nil
	case "ptrace.tracee.egroup":
		return reflect.String, nil
	case "ptrace.tracee.envp":
		return reflect.String, nil
	case "ptrace.tracee.envs":
		return reflect.String, nil
	case "ptrace.tracee.envs_truncated":
		return reflect.Bool, nil
	case "ptrace.tracee.euid":
		return reflect.Int, nil
	case "ptrace.tracee.euser":
		return reflect.String, nil
	case "ptrace.tracee.file.change_time":
		return reflect.Int, nil
	case "ptrace.tracee.file.filesystem":
		return reflect.String, nil
	case "ptrace.tracee.file.gid":
		return reflect.Int, nil
	case "ptrace.tracee.file.group":
		return reflect.String, nil
	case "ptrace.tracee.file.in_upper_layer":
		return reflect.Bool, nil
	case "ptrace.tracee.file.inode":
		return reflect.Int, nil
	case "ptrace.tracee.file.mode":
		return reflect.Int, nil
	case "ptrace.tracee.file.modification_time":
		return reflect.Int, nil
	case "ptrace.tracee.file.mount_id":
		return reflect.Int, nil
	case "ptrace.tracee.file.name":
		return reflect.String, nil
	case "ptrace.tracee.file.name.length":
		return reflect.Int, nil
	case "ptrace.tracee.file.path":
		return reflect.String, nil
	case "ptrace.tracee.file.path.length":
		return reflect.Int, nil
	case "ptrace.tracee.file.rights":
		return reflect.Int, nil
	case "ptrace.tracee.file.uid":
		return reflect.Int, nil
	case "ptrace.tracee.file.user":
		return reflect.String, nil
	case "ptrace.tracee.fsgid":
		return reflect.Int, nil
	case "ptrace.tracee.fsgroup":
		return reflect.String, nil
	case "ptrace.tracee.fsuid":
		return reflect.Int, nil
	case "ptrace.tracee.fsuser":
		return reflect.String, nil
	case "ptrace.tracee.gid":
		return reflect.Int, nil
	case "ptrace.tracee.group":
		return reflect.String, nil
	case "ptrace.tracee.interpreter.file.change_time":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.filesystem":
		return reflect.String, nil
	case "ptrace.tracee.interpreter.file.gid":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.group":
		return reflect.String, nil
	case "ptrace.tracee.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "ptrace.tracee.interpreter.file.inode":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.mode":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.modification_time":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.mount_id":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.name":
		return reflect.String, nil
	case "ptrace.tracee.interpreter.file.name.length":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.path":
		return reflect.String, nil
	case "ptrace.tracee.interpreter.file.path.length":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.rights":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.uid":
		return reflect.Int, nil
	case "ptrace.tracee.interpreter.file.user":
		return reflect.String, nil
	case "ptrace.tracee.is_kworker":
		return reflect.Bool, nil
	case "ptrace.tracee.is_thread":
		return reflect.Bool, nil
	case "ptrace.tracee.pid":
		return reflect.Int, nil
	case "ptrace.tracee.ppid":
		return reflect.Int, nil
	case "ptrace.tracee.tid":
		return reflect.Int, nil
	case "ptrace.tracee.tty_name":
		return reflect.String, nil
	case "ptrace.tracee.uid":
		return reflect.Int, nil
	case "ptrace.tracee.user":
		return reflect.String, nil
	case "removexattr.file.change_time":
		return reflect.Int, nil
	case "removexattr.file.destination.name":
		return reflect.String, nil
	case "removexattr.file.destination.namespace":
		return reflect.String, nil
	case "removexattr.file.filesystem":
		return reflect.String, nil
	case "removexattr.file.gid":
		return reflect.Int, nil
	case "removexattr.file.group":
		return reflect.String, nil
	case "removexattr.file.in_upper_layer":
		return reflect.Bool, nil
	case "removexattr.file.inode":
		return reflect.Int, nil
	case "removexattr.file.mode":
		return reflect.Int, nil
	case "removexattr.file.modification_time":
		return reflect.Int, nil
	case "removexattr.file.mount_id":
		return reflect.Int, nil
	case "removexattr.file.name":
		return reflect.String, nil
	case "removexattr.file.name.length":
		return reflect.Int, nil
	case "removexattr.file.path":
		return reflect.String, nil
	case "removexattr.file.path.length":
		return reflect.Int, nil
	case "removexattr.file.rights":
		return reflect.Int, nil
	case "removexattr.file.uid":
		return reflect.Int, nil
	case "removexattr.file.user":
		return reflect.String, nil
	case "removexattr.retval":
		return reflect.Int, nil
	case "rename.file.change_time":
		return reflect.Int, nil
	case "rename.file.destination.change_time":
		return reflect.Int, nil
	case "rename.file.destination.filesystem":
		return reflect.String, nil
	case "rename.file.destination.gid":
		return reflect.Int, nil
	case "rename.file.destination.group":
		return reflect.String, nil
	case "rename.file.destination.in_upper_layer":
		return reflect.Bool, nil
	case "rename.file.destination.inode":
		return reflect.Int, nil
	case "rename.file.destination.mode":
		return reflect.Int, nil
	case "rename.file.destination.modification_time":
		return reflect.Int, nil
	case "rename.file.destination.mount_id":
		return reflect.Int, nil
	case "rename.file.destination.name":
		return reflect.String, nil
	case "rename.file.destination.name.length":
		return reflect.Int, nil
	case "rename.file.destination.path":
		return reflect.String, nil
	case "rename.file.destination.path.length":
		return reflect.Int, nil
	case "rename.file.destination.rights":
		return reflect.Int, nil
	case "rename.file.destination.uid":
		return reflect.Int, nil
	case "rename.file.destination.user":
		return reflect.String, nil
	case "rename.file.filesystem":
		return reflect.String, nil
	case "rename.file.gid":
		return reflect.Int, nil
	case "rename.file.group":
		return reflect.String, nil
	case "rename.file.in_upper_layer":
		return reflect.Bool, nil
	case "rename.file.inode":
		return reflect.Int, nil
	case "rename.file.mode":
		return reflect.Int, nil
	case "rename.file.modification_time":
		return reflect.Int, nil
	case "rename.file.mount_id":
		return reflect.Int, nil
	case "rename.file.name":
		return reflect.String, nil
	case "rename.file.name.length":
		return reflect.Int, nil
	case "rename.file.path":
		return reflect.String, nil
	case "rename.file.path.length":
		return reflect.Int, nil
	case "rename.file.rights":
		return reflect.Int, nil
	case "rename.file.uid":
		return reflect.Int, nil
	case "rename.file.user":
		return reflect.String, nil
	case "rename.retval":
		return reflect.Int, nil
	case "rmdir.file.change_time":
		return reflect.Int, nil
	case "rmdir.file.filesystem":
		return reflect.String, nil
	case "rmdir.file.gid":
		return reflect.Int, nil
	case "rmdir.file.group":
		return reflect.String, nil
	case "rmdir.file.in_upper_layer":
		return reflect.Bool, nil
	case "rmdir.file.inode":
		return reflect.Int, nil
	case "rmdir.file.mode":
		return reflect.Int, nil
	case "rmdir.file.modification_time":
		return reflect.Int, nil
	case "rmdir.file.mount_id":
		return reflect.Int, nil
	case "rmdir.file.name":
		return reflect.String, nil
	case "rmdir.file.name.length":
		return reflect.Int, nil
	case "rmdir.file.path":
		return reflect.String, nil
	case "rmdir.file.path.length":
		return reflect.Int, nil
	case "rmdir.file.rights":
		return reflect.Int, nil
	case "rmdir.file.uid":
		return reflect.Int, nil
	case "rmdir.file.user":
		return reflect.String, nil
	case "rmdir.retval":
		return reflect.Int, nil
	case "selinux.bool.name":
		return reflect.String, nil
	case "selinux.bool.state":
		return reflect.String, nil
	case "selinux.bool_commit.state":
		return reflect.Bool, nil
	case "selinux.enforce.status":
		return reflect.String, nil
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
	case "setxattr.file.change_time":
		return reflect.Int, nil
	case "setxattr.file.destination.name":
		return reflect.String, nil
	case "setxattr.file.destination.namespace":
		return reflect.String, nil
	case "setxattr.file.filesystem":
		return reflect.String, nil
	case "setxattr.file.gid":
		return reflect.Int, nil
	case "setxattr.file.group":
		return reflect.String, nil
	case "setxattr.file.in_upper_layer":
		return reflect.Bool, nil
	case "setxattr.file.inode":
		return reflect.Int, nil
	case "setxattr.file.mode":
		return reflect.Int, nil
	case "setxattr.file.modification_time":
		return reflect.Int, nil
	case "setxattr.file.mount_id":
		return reflect.Int, nil
	case "setxattr.file.name":
		return reflect.String, nil
	case "setxattr.file.name.length":
		return reflect.Int, nil
	case "setxattr.file.path":
		return reflect.String, nil
	case "setxattr.file.path.length":
		return reflect.Int, nil
	case "setxattr.file.rights":
		return reflect.Int, nil
	case "setxattr.file.uid":
		return reflect.Int, nil
	case "setxattr.file.user":
		return reflect.String, nil
	case "setxattr.retval":
		return reflect.Int, nil
	case "signal.pid":
		return reflect.Int, nil
	case "signal.retval":
		return reflect.Int, nil
	case "signal.target.ancestors.args":
		return reflect.String, nil
	case "signal.target.ancestors.args_flags":
		return reflect.String, nil
	case "signal.target.ancestors.args_options":
		return reflect.String, nil
	case "signal.target.ancestors.args_truncated":
		return reflect.Bool, nil
	case "signal.target.ancestors.argv":
		return reflect.String, nil
	case "signal.target.ancestors.argv0":
		return reflect.String, nil
	case "signal.target.ancestors.cap_effective":
		return reflect.Int, nil
	case "signal.target.ancestors.cap_permitted":
		return reflect.Int, nil
	case "signal.target.ancestors.comm":
		return reflect.String, nil
	case "signal.target.ancestors.container.id":
		return reflect.String, nil
	case "signal.target.ancestors.cookie":
		return reflect.Int, nil
	case "signal.target.ancestors.created_at":
		return reflect.Int, nil
	case "signal.target.ancestors.egid":
		return reflect.Int, nil
	case "signal.target.ancestors.egroup":
		return reflect.String, nil
	case "signal.target.ancestors.envp":
		return reflect.String, nil
	case "signal.target.ancestors.envs":
		return reflect.String, nil
	case "signal.target.ancestors.envs_truncated":
		return reflect.Bool, nil
	case "signal.target.ancestors.euid":
		return reflect.Int, nil
	case "signal.target.ancestors.euser":
		return reflect.String, nil
	case "signal.target.ancestors.file.change_time":
		return reflect.Int, nil
	case "signal.target.ancestors.file.filesystem":
		return reflect.String, nil
	case "signal.target.ancestors.file.gid":
		return reflect.Int, nil
	case "signal.target.ancestors.file.group":
		return reflect.String, nil
	case "signal.target.ancestors.file.in_upper_layer":
		return reflect.Bool, nil
	case "signal.target.ancestors.file.inode":
		return reflect.Int, nil
	case "signal.target.ancestors.file.mode":
		return reflect.Int, nil
	case "signal.target.ancestors.file.modification_time":
		return reflect.Int, nil
	case "signal.target.ancestors.file.mount_id":
		return reflect.Int, nil
	case "signal.target.ancestors.file.name":
		return reflect.String, nil
	case "signal.target.ancestors.file.name.length":
		return reflect.Int, nil
	case "signal.target.ancestors.file.path":
		return reflect.String, nil
	case "signal.target.ancestors.file.path.length":
		return reflect.Int, nil
	case "signal.target.ancestors.file.rights":
		return reflect.Int, nil
	case "signal.target.ancestors.file.uid":
		return reflect.Int, nil
	case "signal.target.ancestors.file.user":
		return reflect.String, nil
	case "signal.target.ancestors.fsgid":
		return reflect.Int, nil
	case "signal.target.ancestors.fsgroup":
		return reflect.String, nil
	case "signal.target.ancestors.fsuid":
		return reflect.Int, nil
	case "signal.target.ancestors.fsuser":
		return reflect.String, nil
	case "signal.target.ancestors.gid":
		return reflect.Int, nil
	case "signal.target.ancestors.group":
		return reflect.String, nil
	case "signal.target.ancestors.interpreter.file.change_time":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.filesystem":
		return reflect.String, nil
	case "signal.target.ancestors.interpreter.file.gid":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.group":
		return reflect.String, nil
	case "signal.target.ancestors.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "signal.target.ancestors.interpreter.file.inode":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.mode":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.modification_time":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.mount_id":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.name":
		return reflect.String, nil
	case "signal.target.ancestors.interpreter.file.name.length":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.path":
		return reflect.String, nil
	case "signal.target.ancestors.interpreter.file.path.length":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.rights":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.uid":
		return reflect.Int, nil
	case "signal.target.ancestors.interpreter.file.user":
		return reflect.String, nil
	case "signal.target.ancestors.is_kworker":
		return reflect.Bool, nil
	case "signal.target.ancestors.is_thread":
		return reflect.Bool, nil
	case "signal.target.ancestors.pid":
		return reflect.Int, nil
	case "signal.target.ancestors.ppid":
		return reflect.Int, nil
	case "signal.target.ancestors.tid":
		return reflect.Int, nil
	case "signal.target.ancestors.tty_name":
		return reflect.String, nil
	case "signal.target.ancestors.uid":
		return reflect.Int, nil
	case "signal.target.ancestors.user":
		return reflect.String, nil
	case "signal.target.args":
		return reflect.String, nil
	case "signal.target.args_flags":
		return reflect.String, nil
	case "signal.target.args_options":
		return reflect.String, nil
	case "signal.target.args_truncated":
		return reflect.Bool, nil
	case "signal.target.argv":
		return reflect.String, nil
	case "signal.target.argv0":
		return reflect.String, nil
	case "signal.target.cap_effective":
		return reflect.Int, nil
	case "signal.target.cap_permitted":
		return reflect.Int, nil
	case "signal.target.comm":
		return reflect.String, nil
	case "signal.target.container.id":
		return reflect.String, nil
	case "signal.target.cookie":
		return reflect.Int, nil
	case "signal.target.created_at":
		return reflect.Int, nil
	case "signal.target.egid":
		return reflect.Int, nil
	case "signal.target.egroup":
		return reflect.String, nil
	case "signal.target.envp":
		return reflect.String, nil
	case "signal.target.envs":
		return reflect.String, nil
	case "signal.target.envs_truncated":
		return reflect.Bool, nil
	case "signal.target.euid":
		return reflect.Int, nil
	case "signal.target.euser":
		return reflect.String, nil
	case "signal.target.file.change_time":
		return reflect.Int, nil
	case "signal.target.file.filesystem":
		return reflect.String, nil
	case "signal.target.file.gid":
		return reflect.Int, nil
	case "signal.target.file.group":
		return reflect.String, nil
	case "signal.target.file.in_upper_layer":
		return reflect.Bool, nil
	case "signal.target.file.inode":
		return reflect.Int, nil
	case "signal.target.file.mode":
		return reflect.Int, nil
	case "signal.target.file.modification_time":
		return reflect.Int, nil
	case "signal.target.file.mount_id":
		return reflect.Int, nil
	case "signal.target.file.name":
		return reflect.String, nil
	case "signal.target.file.name.length":
		return reflect.Int, nil
	case "signal.target.file.path":
		return reflect.String, nil
	case "signal.target.file.path.length":
		return reflect.Int, nil
	case "signal.target.file.rights":
		return reflect.Int, nil
	case "signal.target.file.uid":
		return reflect.Int, nil
	case "signal.target.file.user":
		return reflect.String, nil
	case "signal.target.fsgid":
		return reflect.Int, nil
	case "signal.target.fsgroup":
		return reflect.String, nil
	case "signal.target.fsuid":
		return reflect.Int, nil
	case "signal.target.fsuser":
		return reflect.String, nil
	case "signal.target.gid":
		return reflect.Int, nil
	case "signal.target.group":
		return reflect.String, nil
	case "signal.target.interpreter.file.change_time":
		return reflect.Int, nil
	case "signal.target.interpreter.file.filesystem":
		return reflect.String, nil
	case "signal.target.interpreter.file.gid":
		return reflect.Int, nil
	case "signal.target.interpreter.file.group":
		return reflect.String, nil
	case "signal.target.interpreter.file.in_upper_layer":
		return reflect.Bool, nil
	case "signal.target.interpreter.file.inode":
		return reflect.Int, nil
	case "signal.target.interpreter.file.mode":
		return reflect.Int, nil
	case "signal.target.interpreter.file.modification_time":
		return reflect.Int, nil
	case "signal.target.interpreter.file.mount_id":
		return reflect.Int, nil
	case "signal.target.interpreter.file.name":
		return reflect.String, nil
	case "signal.target.interpreter.file.name.length":
		return reflect.Int, nil
	case "signal.target.interpreter.file.path":
		return reflect.String, nil
	case "signal.target.interpreter.file.path.length":
		return reflect.Int, nil
	case "signal.target.interpreter.file.rights":
		return reflect.Int, nil
	case "signal.target.interpreter.file.uid":
		return reflect.Int, nil
	case "signal.target.interpreter.file.user":
		return reflect.String, nil
	case "signal.target.is_kworker":
		return reflect.Bool, nil
	case "signal.target.is_thread":
		return reflect.Bool, nil
	case "signal.target.pid":
		return reflect.Int, nil
	case "signal.target.ppid":
		return reflect.Int, nil
	case "signal.target.tid":
		return reflect.Int, nil
	case "signal.target.tty_name":
		return reflect.String, nil
	case "signal.target.uid":
		return reflect.Int, nil
	case "signal.target.user":
		return reflect.String, nil
	case "signal.type":
		return reflect.Int, nil
	case "splice.file.change_time":
		return reflect.Int, nil
	case "splice.file.filesystem":
		return reflect.String, nil
	case "splice.file.gid":
		return reflect.Int, nil
	case "splice.file.group":
		return reflect.String, nil
	case "splice.file.in_upper_layer":
		return reflect.Bool, nil
	case "splice.file.inode":
		return reflect.Int, nil
	case "splice.file.mode":
		return reflect.Int, nil
	case "splice.file.modification_time":
		return reflect.Int, nil
	case "splice.file.mount_id":
		return reflect.Int, nil
	case "splice.file.name":
		return reflect.String, nil
	case "splice.file.name.length":
		return reflect.Int, nil
	case "splice.file.path":
		return reflect.String, nil
	case "splice.file.path.length":
		return reflect.Int, nil
	case "splice.file.rights":
		return reflect.Int, nil
	case "splice.file.uid":
		return reflect.Int, nil
	case "splice.file.user":
		return reflect.String, nil
	case "splice.pipe_entry_flag":
		return reflect.Int, nil
	case "splice.pipe_exit_flag":
		return reflect.Int, nil
	case "splice.retval":
		return reflect.Int, nil
	case "unlink.file.change_time":
		return reflect.Int, nil
	case "unlink.file.filesystem":
		return reflect.String, nil
	case "unlink.file.gid":
		return reflect.Int, nil
	case "unlink.file.group":
		return reflect.String, nil
	case "unlink.file.in_upper_layer":
		return reflect.Bool, nil
	case "unlink.file.inode":
		return reflect.Int, nil
	case "unlink.file.mode":
		return reflect.Int, nil
	case "unlink.file.modification_time":
		return reflect.Int, nil
	case "unlink.file.mount_id":
		return reflect.Int, nil
	case "unlink.file.name":
		return reflect.String, nil
	case "unlink.file.name.length":
		return reflect.Int, nil
	case "unlink.file.path":
		return reflect.String, nil
	case "unlink.file.path.length":
		return reflect.Int, nil
	case "unlink.file.rights":
		return reflect.Int, nil
	case "unlink.file.uid":
		return reflect.Int, nil
	case "unlink.file.user":
		return reflect.String, nil
	case "unlink.flags":
		return reflect.Int, nil
	case "unlink.retval":
		return reflect.Int, nil
	case "unload_module.name":
		return reflect.String, nil
	case "unload_module.retval":
		return reflect.Int, nil
	case "utimes.file.change_time":
		return reflect.Int, nil
	case "utimes.file.filesystem":
		return reflect.String, nil
	case "utimes.file.gid":
		return reflect.Int, nil
	case "utimes.file.group":
		return reflect.String, nil
	case "utimes.file.in_upper_layer":
		return reflect.Bool, nil
	case "utimes.file.inode":
		return reflect.Int, nil
	case "utimes.file.mode":
		return reflect.Int, nil
	case "utimes.file.modification_time":
		return reflect.Int, nil
	case "utimes.file.mount_id":
		return reflect.Int, nil
	case "utimes.file.name":
		return reflect.String, nil
	case "utimes.file.name.length":
		return reflect.Int, nil
	case "utimes.file.path":
		return reflect.String, nil
	case "utimes.file.path.length":
		return reflect.Int, nil
	case "utimes.file.rights":
		return reflect.Int, nil
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
	case "async":
		var ok bool
		if e.Async, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Async"}
		}
		return nil
	case "bind.addr.family":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Bind.AddrFamily"}
		}
		e.Bind.AddrFamily = uint16(v)
		return nil
	case "bind.addr.ip":
		v, ok := value.(net.IPNet)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Bind.Addr.IPNet"}
		}
		e.Bind.Addr.IPNet = v
		return nil
	case "bind.addr.port":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Bind.Addr.Port"}
		}
		e.Bind.Addr.Port = uint16(v)
		return nil
	case "bind.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Bind.SyscallEvent.Retval"}
		}
		e.Bind.SyscallEvent.Retval = int64(v)
		return nil
	case "bpf.cmd":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Cmd"}
		}
		e.BPF.Cmd = uint32(v)
		return nil
	case "bpf.map.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Map.Name"}
		}
		e.BPF.Map.Name = str
		return nil
	case "bpf.map.type":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Map.Type"}
		}
		e.BPF.Map.Type = uint32(v)
		return nil
	case "bpf.prog.attach_type":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.AttachType"}
		}
		e.BPF.Program.AttachType = uint32(v)
		return nil
	case "bpf.prog.helpers":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.Helpers"}
		}
		e.BPF.Program.Helpers = append(e.BPF.Program.Helpers, uint32(v))
		return nil
	case "bpf.prog.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.Name"}
		}
		e.BPF.Program.Name = str
		return nil
	case "bpf.prog.tag":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.Tag"}
		}
		e.BPF.Program.Tag = str
		return nil
	case "bpf.prog.type":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.Type"}
		}
		e.BPF.Program.Type = uint32(v)
		return nil
	case "bpf.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.SyscallEvent.Retval"}
		}
		e.BPF.SyscallEvent.Retval = int64(v)
		return nil
	case "capset.cap_effective":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Capset.CapEffective"}
		}
		e.Capset.CapEffective = uint64(v)
		return nil
	case "capset.cap_permitted":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Capset.CapPermitted"}
		}
		e.Capset.CapPermitted = uint64(v)
		return nil
	case "chmod.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.CTime"}
		}
		e.Chmod.File.FileFields.CTime = uint64(v)
		return nil
	case "chmod.file.destination.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Mode"}
		}
		e.Chmod.Mode = uint32(v)
		return nil
	case "chmod.file.destination.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Mode"}
		}
		e.Chmod.Mode = uint32(v)
		return nil
	case "chmod.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.Filesystem"}
		}
		e.Chmod.File.Filesystem = str
		return nil
	case "chmod.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.GID"}
		}
		e.Chmod.File.FileFields.GID = uint32(v)
		return nil
	case "chmod.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.Group"}
		}
		e.Chmod.File.FileFields.Group = str
		return nil
	case "chmod.file.in_upper_layer":
		var ok bool
		if e.Chmod.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.InUpperLayer"}
		}
		return nil
	case "chmod.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.Inode"}
		}
		e.Chmod.File.FileFields.Inode = uint64(v)
		return nil
	case "chmod.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.Mode"}
		}
		e.Chmod.File.FileFields.Mode = uint16(v)
		return nil
	case "chmod.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.MTime"}
		}
		e.Chmod.File.FileFields.MTime = uint64(v)
		return nil
	case "chmod.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.MountID"}
		}
		e.Chmod.File.FileFields.MountID = uint32(v)
		return nil
	case "chmod.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.BasenameStr"}
		}
		e.Chmod.File.BasenameStr = str
		return nil
	case "chmod.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "chmod.file.name.length"}
	case "chmod.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.PathnameStr"}
		}
		e.Chmod.File.PathnameStr = str
		return nil
	case "chmod.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "chmod.file.path.length"}
	case "chmod.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.Mode"}
		}
		e.Chmod.File.FileFields.Mode = uint16(v)
		return nil
	case "chmod.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.UID"}
		}
		e.Chmod.File.FileFields.UID = uint32(v)
		return nil
	case "chmod.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.User"}
		}
		e.Chmod.File.FileFields.User = str
		return nil
	case "chmod.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.SyscallEvent.Retval"}
		}
		e.Chmod.SyscallEvent.Retval = int64(v)
		return nil
	case "chown.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.CTime"}
		}
		e.Chown.File.FileFields.CTime = uint64(v)
		return nil
	case "chown.file.destination.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.GID"}
		}
		e.Chown.GID = int64(v)
		return nil
	case "chown.file.destination.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.Group"}
		}
		e.Chown.Group = str
		return nil
	case "chown.file.destination.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.UID"}
		}
		e.Chown.UID = int64(v)
		return nil
	case "chown.file.destination.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.User"}
		}
		e.Chown.User = str
		return nil
	case "chown.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.Filesystem"}
		}
		e.Chown.File.Filesystem = str
		return nil
	case "chown.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.GID"}
		}
		e.Chown.File.FileFields.GID = uint32(v)
		return nil
	case "chown.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.Group"}
		}
		e.Chown.File.FileFields.Group = str
		return nil
	case "chown.file.in_upper_layer":
		var ok bool
		if e.Chown.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.InUpperLayer"}
		}
		return nil
	case "chown.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.Inode"}
		}
		e.Chown.File.FileFields.Inode = uint64(v)
		return nil
	case "chown.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.Mode"}
		}
		e.Chown.File.FileFields.Mode = uint16(v)
		return nil
	case "chown.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.MTime"}
		}
		e.Chown.File.FileFields.MTime = uint64(v)
		return nil
	case "chown.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.MountID"}
		}
		e.Chown.File.FileFields.MountID = uint32(v)
		return nil
	case "chown.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.BasenameStr"}
		}
		e.Chown.File.BasenameStr = str
		return nil
	case "chown.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "chown.file.name.length"}
	case "chown.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.PathnameStr"}
		}
		e.Chown.File.PathnameStr = str
		return nil
	case "chown.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "chown.file.path.length"}
	case "chown.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.Mode"}
		}
		e.Chown.File.FileFields.Mode = uint16(v)
		return nil
	case "chown.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.UID"}
		}
		e.Chown.File.FileFields.UID = uint32(v)
		return nil
	case "chown.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.User"}
		}
		e.Chown.File.FileFields.User = str
		return nil
	case "chown.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.SyscallEvent.Retval"}
		}
		e.Chown.SyscallEvent.Retval = int64(v)
		return nil
	case "container.id":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ContainerContext.ID"}
		}
		e.ContainerContext.ID = str
		return nil
	case "container.tags":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ContainerContext.Tags"}
		}
		e.ContainerContext.Tags = append(e.ContainerContext.Tags, str)
		return nil
	case "dns.id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.ID"}
		}
		e.DNS.ID = uint16(v)
		return nil
	case "dns.question.class":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.Class"}
		}
		e.DNS.Class = uint16(v)
		return nil
	case "dns.question.count":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.Count"}
		}
		e.DNS.Count = uint16(v)
		return nil
	case "dns.question.length":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.Size"}
		}
		e.DNS.Size = uint16(v)
		return nil
	case "dns.question.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.Name"}
		}
		e.DNS.Name = str
		return nil
	case "dns.question.name.length":
		return &eval.ErrFieldReadOnly{Field: "dns.question.name.length"}
	case "dns.question.type":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.Type"}
		}
		e.DNS.Type = uint16(v)
		return nil
	case "exec.args":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Args"}
		}
		e.Exec.Process.Args = str
		return nil
	case "exec.args_flags":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		e.Exec.Process.Argv = append(e.Exec.Process.Argv, str)
		return nil
	case "exec.args_options":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		e.Exec.Process.Argv = append(e.Exec.Process.Argv, str)
		return nil
	case "exec.args_truncated":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		var ok bool
		if e.Exec.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.ArgsTruncated"}
		}
		return nil
	case "exec.argv":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		e.Exec.Process.Argv = append(e.Exec.Process.Argv, str)
		return nil
	case "exec.argv0":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv0"}
		}
		e.Exec.Process.Argv0 = str
		return nil
	case "exec.cap_effective":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.CapEffective"}
		}
		e.Exec.Process.Credentials.CapEffective = uint64(v)
		return nil
	case "exec.cap_permitted":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.CapPermitted"}
		}
		e.Exec.Process.Credentials.CapPermitted = uint64(v)
		return nil
	case "exec.comm":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Comm"}
		}
		e.Exec.Process.Comm = str
		return nil
	case "exec.container.id":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.ContainerID"}
		}
		e.Exec.Process.ContainerID = str
		return nil
	case "exec.cookie":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Cookie"}
		}
		e.Exec.Process.Cookie = uint32(v)
		return nil
	case "exec.created_at":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.CreatedAt"}
		}
		e.Exec.Process.CreatedAt = uint64(v)
		return nil
	case "exec.egid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EGID"}
		}
		e.Exec.Process.Credentials.EGID = uint32(v)
		return nil
	case "exec.egroup":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EGroup"}
		}
		e.Exec.Process.Credentials.EGroup = str
		return nil
	case "exec.envp":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Envp"}
		}
		e.Exec.Process.Envp = append(e.Exec.Process.Envp, str)
		return nil
	case "exec.envs":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Envs"}
		}
		e.Exec.Process.Envs = append(e.Exec.Process.Envs, str)
		return nil
	case "exec.envs_truncated":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		var ok bool
		if e.Exec.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.EnvsTruncated"}
		}
		return nil
	case "exec.euid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EUID"}
		}
		e.Exec.Process.Credentials.EUID = uint32(v)
		return nil
	case "exec.euser":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EUser"}
		}
		e.Exec.Process.Credentials.EUser = str
		return nil
	case "exec.file.change_time":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.CTime"}
		}
		e.Exec.Process.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "exec.file.filesystem":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.Filesystem"}
		}
		e.Exec.Process.FileEvent.Filesystem = str
		return nil
	case "exec.file.gid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.GID"}
		}
		e.Exec.Process.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "exec.file.group":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.Group"}
		}
		e.Exec.Process.FileEvent.FileFields.Group = str
		return nil
	case "exec.file.in_upper_layer":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		var ok bool
		if e.Exec.Process.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "exec.file.inode":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.Inode"}
		}
		e.Exec.Process.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "exec.file.mode":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.Mode"}
		}
		e.Exec.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "exec.file.modification_time":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.MTime"}
		}
		e.Exec.Process.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "exec.file.mount_id":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.MountID"}
		}
		e.Exec.Process.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "exec.file.name":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.BasenameStr"}
		}
		e.Exec.Process.FileEvent.BasenameStr = str
		return nil
	case "exec.file.name.length":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.file.name.length"}
	case "exec.file.path":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.PathnameStr"}
		}
		e.Exec.Process.FileEvent.PathnameStr = str
		return nil
	case "exec.file.path.length":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.file.path.length"}
	case "exec.file.rights":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.Mode"}
		}
		e.Exec.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "exec.file.uid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.UID"}
		}
		e.Exec.Process.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "exec.file.user":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.FileFields.User"}
		}
		e.Exec.Process.FileEvent.FileFields.User = str
		return nil
	case "exec.fsgid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSGID"}
		}
		e.Exec.Process.Credentials.FSGID = uint32(v)
		return nil
	case "exec.fsgroup":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSGroup"}
		}
		e.Exec.Process.Credentials.FSGroup = str
		return nil
	case "exec.fsuid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSUID"}
		}
		e.Exec.Process.Credentials.FSUID = uint32(v)
		return nil
	case "exec.fsuser":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSUser"}
		}
		e.Exec.Process.Credentials.FSUser = str
		return nil
	case "exec.gid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.GID"}
		}
		e.Exec.Process.Credentials.GID = uint32(v)
		return nil
	case "exec.group":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.Group"}
		}
		e.Exec.Process.Credentials.Group = str
		return nil
	case "exec.interpreter.file.change_time":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "exec.interpreter.file.filesystem":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.Filesystem = str
		return nil
	case "exec.interpreter.file.gid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "exec.interpreter.file.group":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.Group = str
		return nil
	case "exec.interpreter.file.in_upper_layer":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		var ok bool
		if e.Exec.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "exec.interpreter.file.inode":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "exec.interpreter.file.mode":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "exec.interpreter.file.modification_time":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "exec.interpreter.file.mount_id":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "exec.interpreter.file.name":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.BasenameStr = str
		return nil
	case "exec.interpreter.file.name.length":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.interpreter.file.name.length"}
	case "exec.interpreter.file.path":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.PathnameStr = str
		return nil
	case "exec.interpreter.file.path.length":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.interpreter.file.path.length"}
	case "exec.interpreter.file.rights":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "exec.interpreter.file.uid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "exec.interpreter.file.user":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		e.Exec.Process.LinuxBinprm.FileEvent.FileFields.User = str
		return nil
	case "exec.is_kworker":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		var ok bool
		if e.Exec.Process.PIDContext.IsKworker, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PIDContext.IsKworker"}
		}
		return nil
	case "exec.is_thread":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		var ok bool
		if e.Exec.Process.IsThread, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.IsThread"}
		}
		return nil
	case "exec.pid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PIDContext.Pid"}
		}
		e.Exec.Process.PIDContext.Pid = uint32(v)
		return nil
	case "exec.ppid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PPid"}
		}
		e.Exec.Process.PPid = uint32(v)
		return nil
	case "exec.tid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PIDContext.Tid"}
		}
		e.Exec.Process.PIDContext.Tid = uint32(v)
		return nil
	case "exec.tty_name":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.TTYName"}
		}
		e.Exec.Process.TTYName = str
		return nil
	case "exec.uid":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.UID"}
		}
		e.Exec.Process.Credentials.UID = uint32(v)
		return nil
	case "exec.user":
		if e.Exec.Process == nil {
			e.Exec.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.User"}
		}
		e.Exec.Process.Credentials.User = str
		return nil
	case "exit.args":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Args"}
		}
		e.Exit.Process.Args = str
		return nil
	case "exit.args_flags":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Argv"}
		}
		e.Exit.Process.Argv = append(e.Exit.Process.Argv, str)
		return nil
	case "exit.args_options":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Argv"}
		}
		e.Exit.Process.Argv = append(e.Exit.Process.Argv, str)
		return nil
	case "exit.args_truncated":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		var ok bool
		if e.Exit.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.ArgsTruncated"}
		}
		return nil
	case "exit.argv":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Argv"}
		}
		e.Exit.Process.Argv = append(e.Exit.Process.Argv, str)
		return nil
	case "exit.argv0":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Argv0"}
		}
		e.Exit.Process.Argv0 = str
		return nil
	case "exit.cap_effective":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.CapEffective"}
		}
		e.Exit.Process.Credentials.CapEffective = uint64(v)
		return nil
	case "exit.cap_permitted":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.CapPermitted"}
		}
		e.Exit.Process.Credentials.CapPermitted = uint64(v)
		return nil
	case "exit.cause":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Cause"}
		}
		e.Exit.Cause = uint32(v)
		return nil
	case "exit.code":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Code"}
		}
		e.Exit.Code = uint32(v)
		return nil
	case "exit.comm":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Comm"}
		}
		e.Exit.Process.Comm = str
		return nil
	case "exit.container.id":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.ContainerID"}
		}
		e.Exit.Process.ContainerID = str
		return nil
	case "exit.cookie":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Cookie"}
		}
		e.Exit.Process.Cookie = uint32(v)
		return nil
	case "exit.created_at":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.CreatedAt"}
		}
		e.Exit.Process.CreatedAt = uint64(v)
		return nil
	case "exit.egid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.EGID"}
		}
		e.Exit.Process.Credentials.EGID = uint32(v)
		return nil
	case "exit.egroup":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.EGroup"}
		}
		e.Exit.Process.Credentials.EGroup = str
		return nil
	case "exit.envp":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Envp"}
		}
		e.Exit.Process.Envp = append(e.Exit.Process.Envp, str)
		return nil
	case "exit.envs":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Envs"}
		}
		e.Exit.Process.Envs = append(e.Exit.Process.Envs, str)
		return nil
	case "exit.envs_truncated":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		var ok bool
		if e.Exit.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.EnvsTruncated"}
		}
		return nil
	case "exit.euid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.EUID"}
		}
		e.Exit.Process.Credentials.EUID = uint32(v)
		return nil
	case "exit.euser":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.EUser"}
		}
		e.Exit.Process.Credentials.EUser = str
		return nil
	case "exit.file.change_time":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.CTime"}
		}
		e.Exit.Process.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "exit.file.filesystem":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.Filesystem"}
		}
		e.Exit.Process.FileEvent.Filesystem = str
		return nil
	case "exit.file.gid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.GID"}
		}
		e.Exit.Process.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "exit.file.group":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.Group"}
		}
		e.Exit.Process.FileEvent.FileFields.Group = str
		return nil
	case "exit.file.in_upper_layer":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		var ok bool
		if e.Exit.Process.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "exit.file.inode":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.Inode"}
		}
		e.Exit.Process.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "exit.file.mode":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.Mode"}
		}
		e.Exit.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "exit.file.modification_time":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.MTime"}
		}
		e.Exit.Process.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "exit.file.mount_id":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.MountID"}
		}
		e.Exit.Process.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "exit.file.name":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.BasenameStr"}
		}
		e.Exit.Process.FileEvent.BasenameStr = str
		return nil
	case "exit.file.name.length":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.file.name.length"}
	case "exit.file.path":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.PathnameStr"}
		}
		e.Exit.Process.FileEvent.PathnameStr = str
		return nil
	case "exit.file.path.length":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.file.path.length"}
	case "exit.file.rights":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.Mode"}
		}
		e.Exit.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "exit.file.uid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.UID"}
		}
		e.Exit.Process.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "exit.file.user":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.FileFields.User"}
		}
		e.Exit.Process.FileEvent.FileFields.User = str
		return nil
	case "exit.fsgid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.FSGID"}
		}
		e.Exit.Process.Credentials.FSGID = uint32(v)
		return nil
	case "exit.fsgroup":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.FSGroup"}
		}
		e.Exit.Process.Credentials.FSGroup = str
		return nil
	case "exit.fsuid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.FSUID"}
		}
		e.Exit.Process.Credentials.FSUID = uint32(v)
		return nil
	case "exit.fsuser":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.FSUser"}
		}
		e.Exit.Process.Credentials.FSUser = str
		return nil
	case "exit.gid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.GID"}
		}
		e.Exit.Process.Credentials.GID = uint32(v)
		return nil
	case "exit.group":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.Group"}
		}
		e.Exit.Process.Credentials.Group = str
		return nil
	case "exit.interpreter.file.change_time":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "exit.interpreter.file.filesystem":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.Filesystem = str
		return nil
	case "exit.interpreter.file.gid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "exit.interpreter.file.group":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.Group = str
		return nil
	case "exit.interpreter.file.in_upper_layer":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		var ok bool
		if e.Exit.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "exit.interpreter.file.inode":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "exit.interpreter.file.mode":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "exit.interpreter.file.modification_time":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "exit.interpreter.file.mount_id":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "exit.interpreter.file.name":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.BasenameStr = str
		return nil
	case "exit.interpreter.file.name.length":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.interpreter.file.name.length"}
	case "exit.interpreter.file.path":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.PathnameStr = str
		return nil
	case "exit.interpreter.file.path.length":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.interpreter.file.path.length"}
	case "exit.interpreter.file.rights":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "exit.interpreter.file.uid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "exit.interpreter.file.user":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		e.Exit.Process.LinuxBinprm.FileEvent.FileFields.User = str
		return nil
	case "exit.is_kworker":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		var ok bool
		if e.Exit.Process.PIDContext.IsKworker, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PIDContext.IsKworker"}
		}
		return nil
	case "exit.is_thread":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		var ok bool
		if e.Exit.Process.IsThread, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.IsThread"}
		}
		return nil
	case "exit.pid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PIDContext.Pid"}
		}
		e.Exit.Process.PIDContext.Pid = uint32(v)
		return nil
	case "exit.ppid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PPid"}
		}
		e.Exit.Process.PPid = uint32(v)
		return nil
	case "exit.tid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PIDContext.Tid"}
		}
		e.Exit.Process.PIDContext.Tid = uint32(v)
		return nil
	case "exit.tty_name":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.TTYName"}
		}
		e.Exit.Process.TTYName = str
		return nil
	case "exit.uid":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.UID"}
		}
		e.Exit.Process.Credentials.UID = uint32(v)
		return nil
	case "exit.user":
		if e.Exit.Process == nil {
			e.Exit.Process = &Process{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Credentials.User"}
		}
		e.Exit.Process.Credentials.User = str
		return nil
	case "link.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.CTime"}
		}
		e.Link.Source.FileFields.CTime = uint64(v)
		return nil
	case "link.file.destination.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.CTime"}
		}
		e.Link.Target.FileFields.CTime = uint64(v)
		return nil
	case "link.file.destination.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.Filesystem"}
		}
		e.Link.Target.Filesystem = str
		return nil
	case "link.file.destination.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.GID"}
		}
		e.Link.Target.FileFields.GID = uint32(v)
		return nil
	case "link.file.destination.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Group"}
		}
		e.Link.Target.FileFields.Group = str
		return nil
	case "link.file.destination.in_upper_layer":
		var ok bool
		if e.Link.Target.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.InUpperLayer"}
		}
		return nil
	case "link.file.destination.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Inode"}
		}
		e.Link.Target.FileFields.Inode = uint64(v)
		return nil
	case "link.file.destination.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Mode"}
		}
		e.Link.Target.FileFields.Mode = uint16(v)
		return nil
	case "link.file.destination.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.MTime"}
		}
		e.Link.Target.FileFields.MTime = uint64(v)
		return nil
	case "link.file.destination.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.MountID"}
		}
		e.Link.Target.FileFields.MountID = uint32(v)
		return nil
	case "link.file.destination.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.BasenameStr"}
		}
		e.Link.Target.BasenameStr = str
		return nil
	case "link.file.destination.name.length":
		return &eval.ErrFieldReadOnly{Field: "link.file.destination.name.length"}
	case "link.file.destination.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.PathnameStr"}
		}
		e.Link.Target.PathnameStr = str
		return nil
	case "link.file.destination.path.length":
		return &eval.ErrFieldReadOnly{Field: "link.file.destination.path.length"}
	case "link.file.destination.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Mode"}
		}
		e.Link.Target.FileFields.Mode = uint16(v)
		return nil
	case "link.file.destination.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.UID"}
		}
		e.Link.Target.FileFields.UID = uint32(v)
		return nil
	case "link.file.destination.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.User"}
		}
		e.Link.Target.FileFields.User = str
		return nil
	case "link.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.Filesystem"}
		}
		e.Link.Source.Filesystem = str
		return nil
	case "link.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.GID"}
		}
		e.Link.Source.FileFields.GID = uint32(v)
		return nil
	case "link.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Group"}
		}
		e.Link.Source.FileFields.Group = str
		return nil
	case "link.file.in_upper_layer":
		var ok bool
		if e.Link.Source.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.InUpperLayer"}
		}
		return nil
	case "link.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Inode"}
		}
		e.Link.Source.FileFields.Inode = uint64(v)
		return nil
	case "link.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Mode"}
		}
		e.Link.Source.FileFields.Mode = uint16(v)
		return nil
	case "link.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.MTime"}
		}
		e.Link.Source.FileFields.MTime = uint64(v)
		return nil
	case "link.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.MountID"}
		}
		e.Link.Source.FileFields.MountID = uint32(v)
		return nil
	case "link.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.BasenameStr"}
		}
		e.Link.Source.BasenameStr = str
		return nil
	case "link.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "link.file.name.length"}
	case "link.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.PathnameStr"}
		}
		e.Link.Source.PathnameStr = str
		return nil
	case "link.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "link.file.path.length"}
	case "link.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Mode"}
		}
		e.Link.Source.FileFields.Mode = uint16(v)
		return nil
	case "link.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.UID"}
		}
		e.Link.Source.FileFields.UID = uint32(v)
		return nil
	case "link.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.User"}
		}
		e.Link.Source.FileFields.User = str
		return nil
	case "link.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.SyscallEvent.Retval"}
		}
		e.Link.SyscallEvent.Retval = int64(v)
		return nil
	case "load_module.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.CTime"}
		}
		e.LoadModule.File.FileFields.CTime = uint64(v)
		return nil
	case "load_module.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.Filesystem"}
		}
		e.LoadModule.File.Filesystem = str
		return nil
	case "load_module.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.GID"}
		}
		e.LoadModule.File.FileFields.GID = uint32(v)
		return nil
	case "load_module.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.Group"}
		}
		e.LoadModule.File.FileFields.Group = str
		return nil
	case "load_module.file.in_upper_layer":
		var ok bool
		if e.LoadModule.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.InUpperLayer"}
		}
		return nil
	case "load_module.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.Inode"}
		}
		e.LoadModule.File.FileFields.Inode = uint64(v)
		return nil
	case "load_module.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.Mode"}
		}
		e.LoadModule.File.FileFields.Mode = uint16(v)
		return nil
	case "load_module.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.MTime"}
		}
		e.LoadModule.File.FileFields.MTime = uint64(v)
		return nil
	case "load_module.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.MountID"}
		}
		e.LoadModule.File.FileFields.MountID = uint32(v)
		return nil
	case "load_module.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.BasenameStr"}
		}
		e.LoadModule.File.BasenameStr = str
		return nil
	case "load_module.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "load_module.file.name.length"}
	case "load_module.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.PathnameStr"}
		}
		e.LoadModule.File.PathnameStr = str
		return nil
	case "load_module.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "load_module.file.path.length"}
	case "load_module.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.Mode"}
		}
		e.LoadModule.File.FileFields.Mode = uint16(v)
		return nil
	case "load_module.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.UID"}
		}
		e.LoadModule.File.FileFields.UID = uint32(v)
		return nil
	case "load_module.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.User"}
		}
		e.LoadModule.File.FileFields.User = str
		return nil
	case "load_module.loaded_from_memory":
		var ok bool
		if e.LoadModule.LoadedFromMemory, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.LoadedFromMemory"}
		}
		return nil
	case "load_module.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.Name"}
		}
		e.LoadModule.Name = str
		return nil
	case "load_module.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.SyscallEvent.Retval"}
		}
		e.LoadModule.SyscallEvent.Retval = int64(v)
		return nil
	case "mkdir.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.CTime"}
		}
		e.Mkdir.File.FileFields.CTime = uint64(v)
		return nil
	case "mkdir.file.destination.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Mode"}
		}
		e.Mkdir.Mode = uint32(v)
		return nil
	case "mkdir.file.destination.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Mode"}
		}
		e.Mkdir.Mode = uint32(v)
		return nil
	case "mkdir.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.Filesystem"}
		}
		e.Mkdir.File.Filesystem = str
		return nil
	case "mkdir.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.GID"}
		}
		e.Mkdir.File.FileFields.GID = uint32(v)
		return nil
	case "mkdir.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.Group"}
		}
		e.Mkdir.File.FileFields.Group = str
		return nil
	case "mkdir.file.in_upper_layer":
		var ok bool
		if e.Mkdir.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.InUpperLayer"}
		}
		return nil
	case "mkdir.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.Inode"}
		}
		e.Mkdir.File.FileFields.Inode = uint64(v)
		return nil
	case "mkdir.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.Mode"}
		}
		e.Mkdir.File.FileFields.Mode = uint16(v)
		return nil
	case "mkdir.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.MTime"}
		}
		e.Mkdir.File.FileFields.MTime = uint64(v)
		return nil
	case "mkdir.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.MountID"}
		}
		e.Mkdir.File.FileFields.MountID = uint32(v)
		return nil
	case "mkdir.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.BasenameStr"}
		}
		e.Mkdir.File.BasenameStr = str
		return nil
	case "mkdir.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "mkdir.file.name.length"}
	case "mkdir.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.PathnameStr"}
		}
		e.Mkdir.File.PathnameStr = str
		return nil
	case "mkdir.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "mkdir.file.path.length"}
	case "mkdir.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.Mode"}
		}
		e.Mkdir.File.FileFields.Mode = uint16(v)
		return nil
	case "mkdir.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.UID"}
		}
		e.Mkdir.File.FileFields.UID = uint32(v)
		return nil
	case "mkdir.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.User"}
		}
		e.Mkdir.File.FileFields.User = str
		return nil
	case "mkdir.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.SyscallEvent.Retval"}
		}
		e.Mkdir.SyscallEvent.Retval = int64(v)
		return nil
	case "mmap.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.CTime"}
		}
		e.MMap.File.FileFields.CTime = uint64(v)
		return nil
	case "mmap.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.Filesystem"}
		}
		e.MMap.File.Filesystem = str
		return nil
	case "mmap.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.GID"}
		}
		e.MMap.File.FileFields.GID = uint32(v)
		return nil
	case "mmap.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.Group"}
		}
		e.MMap.File.FileFields.Group = str
		return nil
	case "mmap.file.in_upper_layer":
		var ok bool
		if e.MMap.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.InUpperLayer"}
		}
		return nil
	case "mmap.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.Inode"}
		}
		e.MMap.File.FileFields.Inode = uint64(v)
		return nil
	case "mmap.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.Mode"}
		}
		e.MMap.File.FileFields.Mode = uint16(v)
		return nil
	case "mmap.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.MTime"}
		}
		e.MMap.File.FileFields.MTime = uint64(v)
		return nil
	case "mmap.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.MountID"}
		}
		e.MMap.File.FileFields.MountID = uint32(v)
		return nil
	case "mmap.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.BasenameStr"}
		}
		e.MMap.File.BasenameStr = str
		return nil
	case "mmap.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "mmap.file.name.length"}
	case "mmap.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.PathnameStr"}
		}
		e.MMap.File.PathnameStr = str
		return nil
	case "mmap.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "mmap.file.path.length"}
	case "mmap.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.Mode"}
		}
		e.MMap.File.FileFields.Mode = uint16(v)
		return nil
	case "mmap.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.UID"}
		}
		e.MMap.File.FileFields.UID = uint32(v)
		return nil
	case "mmap.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.User"}
		}
		e.MMap.File.FileFields.User = str
		return nil
	case "mmap.flags":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.Flags"}
		}
		e.MMap.Flags = int(v)
		return nil
	case "mmap.protection":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.Protection"}
		}
		e.MMap.Protection = int(v)
		return nil
	case "mmap.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.SyscallEvent.Retval"}
		}
		e.MMap.SyscallEvent.Retval = int64(v)
		return nil
	case "mprotect.req_protection":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MProtect.ReqProtection"}
		}
		e.MProtect.ReqProtection = int(v)
		return nil
	case "mprotect.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MProtect.SyscallEvent.Retval"}
		}
		e.MProtect.SyscallEvent.Retval = int64(v)
		return nil
	case "mprotect.vm_protection":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MProtect.VMProtection"}
		}
		e.MProtect.VMProtection = int(v)
		return nil
	case "network.destination.ip":
		v, ok := value.(net.IPNet)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.Destination.IPNet"}
		}
		e.NetworkContext.Destination.IPNet = v
		return nil
	case "network.destination.port":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.Destination.Port"}
		}
		e.NetworkContext.Destination.Port = uint16(v)
		return nil
	case "network.device.ifindex":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.Device.IfIndex"}
		}
		e.NetworkContext.Device.IfIndex = uint32(v)
		return nil
	case "network.device.ifname":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.Device.IfName"}
		}
		e.NetworkContext.Device.IfName = str
		return nil
	case "network.l3_protocol":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.L3Protocol"}
		}
		e.NetworkContext.L3Protocol = uint16(v)
		return nil
	case "network.l4_protocol":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.L4Protocol"}
		}
		e.NetworkContext.L4Protocol = uint16(v)
		return nil
	case "network.size":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.Size"}
		}
		e.NetworkContext.Size = uint32(v)
		return nil
	case "network.source.ip":
		v, ok := value.(net.IPNet)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.Source.IPNet"}
		}
		e.NetworkContext.Source.IPNet = v
		return nil
	case "network.source.port":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "NetworkContext.Source.Port"}
		}
		e.NetworkContext.Source.Port = uint16(v)
		return nil
	case "open.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.CTime"}
		}
		e.Open.File.FileFields.CTime = uint64(v)
		return nil
	case "open.file.destination.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Mode"}
		}
		e.Open.Mode = uint32(v)
		return nil
	case "open.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.Filesystem"}
		}
		e.Open.File.Filesystem = str
		return nil
	case "open.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.GID"}
		}
		e.Open.File.FileFields.GID = uint32(v)
		return nil
	case "open.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.Group"}
		}
		e.Open.File.FileFields.Group = str
		return nil
	case "open.file.in_upper_layer":
		var ok bool
		if e.Open.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.InUpperLayer"}
		}
		return nil
	case "open.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.Inode"}
		}
		e.Open.File.FileFields.Inode = uint64(v)
		return nil
	case "open.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.Mode"}
		}
		e.Open.File.FileFields.Mode = uint16(v)
		return nil
	case "open.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.MTime"}
		}
		e.Open.File.FileFields.MTime = uint64(v)
		return nil
	case "open.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.MountID"}
		}
		e.Open.File.FileFields.MountID = uint32(v)
		return nil
	case "open.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.BasenameStr"}
		}
		e.Open.File.BasenameStr = str
		return nil
	case "open.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "open.file.name.length"}
	case "open.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.PathnameStr"}
		}
		e.Open.File.PathnameStr = str
		return nil
	case "open.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "open.file.path.length"}
	case "open.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.Mode"}
		}
		e.Open.File.FileFields.Mode = uint16(v)
		return nil
	case "open.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.UID"}
		}
		e.Open.File.FileFields.UID = uint32(v)
		return nil
	case "open.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.User"}
		}
		e.Open.File.FileFields.User = str
		return nil
	case "open.flags":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Flags"}
		}
		e.Open.Flags = uint32(v)
		return nil
	case "open.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.SyscallEvent.Retval"}
		}
		e.Open.SyscallEvent.Retval = int64(v)
		return nil
	case "process.ancestors.args":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Args"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Args = str
		return nil
	case "process.ancestors.args_flags":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(e.ProcessContext.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "process.ancestors.args_options":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(e.ProcessContext.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "process.ancestors.args_truncated":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.ArgsTruncated"}
		}
		return nil
	case "process.ancestors.argv":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(e.ProcessContext.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "process.ancestors.argv0":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv0"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Argv0 = str
		return nil
	case "process.ancestors.cap_effective":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapEffective"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapEffective = uint64(v)
		return nil
	case "process.ancestors.cap_permitted":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapPermitted"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapPermitted = uint64(v)
		return nil
	case "process.ancestors.comm":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Comm"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Comm = str
		return nil
	case "process.ancestors.container.id":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.ContainerID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.ContainerID = str
		return nil
	case "process.ancestors.cookie":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Cookie"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Cookie = uint32(v)
		return nil
	case "process.ancestors.created_at":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.CreatedAt"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.CreatedAt = uint64(v)
		return nil
	case "process.ancestors.egid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGID = uint32(v)
		return nil
	case "process.ancestors.egroup":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGroup"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGroup = str
		return nil
	case "process.ancestors.envp":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Envp"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Envp = append(e.ProcessContext.Ancestor.ProcessContext.Process.Envp, str)
		return nil
	case "process.ancestors.envs":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Envs"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Envs = append(e.ProcessContext.Ancestor.ProcessContext.Process.Envs, str)
		return nil
	case "process.ancestors.envs_truncated":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.EnvsTruncated"}
		}
		return nil
	case "process.ancestors.euid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUID = uint32(v)
		return nil
	case "process.ancestors.euser":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUser"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUser = str
		return nil
	case "process.ancestors.file.change_time":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.CTime"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "process.ancestors.file.filesystem":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.Filesystem"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.Filesystem = str
		return nil
	case "process.ancestors.file.gid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.GID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "process.ancestors.file.group":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Group"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Group = str
		return nil
	case "process.ancestors.file.in_upper_layer":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "process.ancestors.file.inode":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Inode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "process.ancestors.file.mode":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "process.ancestors.file.modification_time":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.MTime"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "process.ancestors.file.mount_id":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.MountID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "process.ancestors.file.name":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.BasenameStr"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.BasenameStr = str
		return nil
	case "process.ancestors.file.name.length":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.file.name.length"}
	case "process.ancestors.file.path":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.PathnameStr"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.PathnameStr = str
		return nil
	case "process.ancestors.file.path.length":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.file.path.length"}
	case "process.ancestors.file.rights":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "process.ancestors.file.uid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.UID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "process.ancestors.file.user":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.User"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.FileFields.User = str
		return nil
	case "process.ancestors.fsgid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGID = uint32(v)
		return nil
	case "process.ancestors.fsgroup":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGroup"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGroup = str
		return nil
	case "process.ancestors.fsuid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUID = uint32(v)
		return nil
	case "process.ancestors.fsuser":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUser"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUser = str
		return nil
	case "process.ancestors.gid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.GID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.GID = uint32(v)
		return nil
	case "process.ancestors.group":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.Group"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.Group = str
		return nil
	case "process.ancestors.interpreter.file.change_time":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "process.ancestors.interpreter.file.filesystem":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem = str
		return nil
	case "process.ancestors.interpreter.file.gid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "process.ancestors.interpreter.file.group":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group = str
		return nil
	case "process.ancestors.interpreter.file.in_upper_layer":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "process.ancestors.interpreter.file.inode":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "process.ancestors.interpreter.file.mode":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "process.ancestors.interpreter.file.modification_time":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "process.ancestors.interpreter.file.mount_id":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "process.ancestors.interpreter.file.name":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr = str
		return nil
	case "process.ancestors.interpreter.file.name.length":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.interpreter.file.name.length"}
	case "process.ancestors.interpreter.file.path":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr = str
		return nil
	case "process.ancestors.interpreter.file.path.length":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.interpreter.file.path.length"}
	case "process.ancestors.interpreter.file.rights":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "process.ancestors.interpreter.file.uid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "process.ancestors.interpreter.file.user":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User = str
		return nil
	case "process.ancestors.is_kworker":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.IsKworker, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PIDContext.IsKworker"}
		}
		return nil
	case "process.ancestors.is_thread":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.IsThread, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.IsThread"}
		}
		return nil
	case "process.ancestors.pid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Pid"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Pid = uint32(v)
		return nil
	case "process.ancestors.ppid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PPid"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.PPid = uint32(v)
		return nil
	case "process.ancestors.tid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Tid"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Tid = uint32(v)
		return nil
	case "process.ancestors.tty_name":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.TTYName"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.TTYName = str
		return nil
	case "process.ancestors.uid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.UID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.UID = uint32(v)
		return nil
	case "process.ancestors.user":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.User"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.User = str
		return nil
	case "process.args":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Args"}
		}
		e.ProcessContext.Process.Args = str
		return nil
	case "process.args_flags":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Process.Argv = append(e.ProcessContext.Process.Argv, str)
		return nil
	case "process.args_options":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Process.Argv = append(e.ProcessContext.Process.Argv, str)
		return nil
	case "process.args_truncated":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		var ok bool
		if e.ProcessContext.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.ArgsTruncated"}
		}
		return nil
	case "process.argv":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Process.Argv = append(e.ProcessContext.Process.Argv, str)
		return nil
	case "process.argv0":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv0"}
		}
		e.ProcessContext.Process.Argv0 = str
		return nil
	case "process.cap_effective":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.CapEffective"}
		}
		e.ProcessContext.Process.Credentials.CapEffective = uint64(v)
		return nil
	case "process.cap_permitted":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.CapPermitted"}
		}
		e.ProcessContext.Process.Credentials.CapPermitted = uint64(v)
		return nil
	case "process.comm":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Comm"}
		}
		e.ProcessContext.Process.Comm = str
		return nil
	case "process.container.id":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.ContainerID"}
		}
		e.ProcessContext.Process.ContainerID = str
		return nil
	case "process.cookie":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Cookie"}
		}
		e.ProcessContext.Process.Cookie = uint32(v)
		return nil
	case "process.created_at":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.CreatedAt"}
		}
		e.ProcessContext.Process.CreatedAt = uint64(v)
		return nil
	case "process.egid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EGID"}
		}
		e.ProcessContext.Process.Credentials.EGID = uint32(v)
		return nil
	case "process.egroup":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EGroup"}
		}
		e.ProcessContext.Process.Credentials.EGroup = str
		return nil
	case "process.envp":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Envp"}
		}
		e.ProcessContext.Process.Envp = append(e.ProcessContext.Process.Envp, str)
		return nil
	case "process.envs":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Envs"}
		}
		e.ProcessContext.Process.Envs = append(e.ProcessContext.Process.Envs, str)
		return nil
	case "process.envs_truncated":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		var ok bool
		if e.ProcessContext.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.EnvsTruncated"}
		}
		return nil
	case "process.euid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EUID"}
		}
		e.ProcessContext.Process.Credentials.EUID = uint32(v)
		return nil
	case "process.euser":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EUser"}
		}
		e.ProcessContext.Process.Credentials.EUser = str
		return nil
	case "process.file.change_time":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.CTime"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "process.file.filesystem":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.Filesystem"}
		}
		e.ProcessContext.Process.FileEvent.Filesystem = str
		return nil
	case "process.file.gid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.GID"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "process.file.group":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.Group"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.Group = str
		return nil
	case "process.file.in_upper_layer":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		var ok bool
		if e.ProcessContext.Process.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "process.file.inode":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.Inode"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "process.file.mode":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "process.file.modification_time":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.MTime"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "process.file.mount_id":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.MountID"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "process.file.name":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.BasenameStr"}
		}
		e.ProcessContext.Process.FileEvent.BasenameStr = str
		return nil
	case "process.file.name.length":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.file.name.length"}
	case "process.file.path":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.PathnameStr"}
		}
		e.ProcessContext.Process.FileEvent.PathnameStr = str
		return nil
	case "process.file.path.length":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.file.path.length"}
	case "process.file.rights":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "process.file.uid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.UID"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "process.file.user":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileEvent.FileFields.User"}
		}
		e.ProcessContext.Process.FileEvent.FileFields.User = str
		return nil
	case "process.fsgid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSGID"}
		}
		e.ProcessContext.Process.Credentials.FSGID = uint32(v)
		return nil
	case "process.fsgroup":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSGroup"}
		}
		e.ProcessContext.Process.Credentials.FSGroup = str
		return nil
	case "process.fsuid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSUID"}
		}
		e.ProcessContext.Process.Credentials.FSUID = uint32(v)
		return nil
	case "process.fsuser":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSUser"}
		}
		e.ProcessContext.Process.Credentials.FSUser = str
		return nil
	case "process.gid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.GID"}
		}
		e.ProcessContext.Process.Credentials.GID = uint32(v)
		return nil
	case "process.group":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.Group"}
		}
		e.ProcessContext.Process.Credentials.Group = str
		return nil
	case "process.interpreter.file.change_time":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "process.interpreter.file.filesystem":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem = str
		return nil
	case "process.interpreter.file.gid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "process.interpreter.file.group":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group = str
		return nil
	case "process.interpreter.file.in_upper_layer":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		var ok bool
		if e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "process.interpreter.file.inode":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "process.interpreter.file.mode":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "process.interpreter.file.modification_time":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "process.interpreter.file.mount_id":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "process.interpreter.file.name":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr = str
		return nil
	case "process.interpreter.file.name.length":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.interpreter.file.name.length"}
	case "process.interpreter.file.path":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr = str
		return nil
	case "process.interpreter.file.path.length":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.interpreter.file.path.length"}
	case "process.interpreter.file.rights":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "process.interpreter.file.uid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "process.interpreter.file.user":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		e.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User = str
		return nil
	case "process.is_kworker":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		var ok bool
		if e.ProcessContext.Process.PIDContext.IsKworker, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PIDContext.IsKworker"}
		}
		return nil
	case "process.is_thread":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		var ok bool
		if e.ProcessContext.Process.IsThread, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.IsThread"}
		}
		return nil
	case "process.pid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PIDContext.Pid"}
		}
		e.ProcessContext.Process.PIDContext.Pid = uint32(v)
		return nil
	case "process.ppid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PPid"}
		}
		e.ProcessContext.Process.PPid = uint32(v)
		return nil
	case "process.tid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PIDContext.Tid"}
		}
		e.ProcessContext.Process.PIDContext.Tid = uint32(v)
		return nil
	case "process.tty_name":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.TTYName"}
		}
		e.ProcessContext.Process.TTYName = str
		return nil
	case "process.uid":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.UID"}
		}
		e.ProcessContext.Process.Credentials.UID = uint32(v)
		return nil
	case "process.user":
		if e.ProcessContext == nil {
			e.ProcessContext = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.User"}
		}
		e.ProcessContext.Process.Credentials.User = str
		return nil
	case "ptrace.request":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Request"}
		}
		e.PTrace.Request = uint32(v)
		return nil
	case "ptrace.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.SyscallEvent.Retval"}
		}
		e.PTrace.SyscallEvent.Retval = int64(v)
		return nil
	case "ptrace.tracee.ancestors.args":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Args"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Args = str
		return nil
	case "ptrace.tracee.ancestors.args_flags":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Argv"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "ptrace.tracee.ancestors.args_options":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Argv"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "ptrace.tracee.ancestors.args_truncated":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.ArgsTruncated"}
		}
		return nil
	case "ptrace.tracee.ancestors.argv":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Argv"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "ptrace.tracee.ancestors.argv0":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Argv0"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv0 = str
		return nil
	case "ptrace.tracee.ancestors.cap_effective":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.CapEffective"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.CapEffective = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.cap_permitted":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.CapPermitted"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.CapPermitted = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.comm":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Comm"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Comm = str
		return nil
	case "ptrace.tracee.ancestors.container.id":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.ContainerID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.ContainerID = str
		return nil
	case "ptrace.tracee.ancestors.cookie":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Cookie"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Cookie = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.created_at":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.CreatedAt"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.CreatedAt = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.egid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EGID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EGID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.egroup":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EGroup"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EGroup = str
		return nil
	case "ptrace.tracee.ancestors.envp":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Envp"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Envp = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Envp, str)
		return nil
	case "ptrace.tracee.ancestors.envs":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Envs"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Envs = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Envs, str)
		return nil
	case "ptrace.tracee.ancestors.envs_truncated":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.EnvsTruncated"}
		}
		return nil
	case "ptrace.tracee.ancestors.euid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EUID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EUID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.euser":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EUser"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EUser = str
		return nil
	case "ptrace.tracee.ancestors.file.change_time":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.CTime"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.file.filesystem":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.Filesystem"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.Filesystem = str
		return nil
	case "ptrace.tracee.ancestors.file.gid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.GID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.file.group":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.Group"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.Group = str
		return nil
	case "ptrace.tracee.ancestors.file.in_upper_layer":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "ptrace.tracee.ancestors.file.inode":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.Inode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.file.mode":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "ptrace.tracee.ancestors.file.modification_time":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.MTime"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.file.mount_id":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.MountID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.file.name":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.BasenameStr"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.BasenameStr = str
		return nil
	case "ptrace.tracee.ancestors.file.name.length":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "ptrace.tracee.ancestors.file.name.length"}
	case "ptrace.tracee.ancestors.file.path":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.PathnameStr"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.PathnameStr = str
		return nil
	case "ptrace.tracee.ancestors.file.path.length":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "ptrace.tracee.ancestors.file.path.length"}
	case "ptrace.tracee.ancestors.file.rights":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "ptrace.tracee.ancestors.file.uid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.UID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.file.user":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.User"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileEvent.FileFields.User = str
		return nil
	case "ptrace.tracee.ancestors.fsgid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSGID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSGID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.fsgroup":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSGroup"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSGroup = str
		return nil
	case "ptrace.tracee.ancestors.fsuid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSUID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSUID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.fsuser":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSUser"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSUser = str
		return nil
	case "ptrace.tracee.ancestors.gid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.GID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.GID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.group":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.Group"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.Group = str
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.change_time":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.filesystem":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem = str
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.gid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.group":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group = str
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.in_upper_layer":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.inode":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.mode":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.modification_time":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.mount_id":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.name":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr = str
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.name.length":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "ptrace.tracee.ancestors.interpreter.file.name.length"}
	case "ptrace.tracee.ancestors.interpreter.file.path":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr = str
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.path.length":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "ptrace.tracee.ancestors.interpreter.file.path.length"}
	case "ptrace.tracee.ancestors.interpreter.file.rights":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.uid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.interpreter.file.user":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User = str
		return nil
	case "ptrace.tracee.ancestors.is_kworker":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.PIDContext.IsKworker, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.PIDContext.IsKworker"}
		}
		return nil
	case "ptrace.tracee.ancestors.is_thread":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.IsThread, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.IsThread"}
		}
		return nil
	case "ptrace.tracee.ancestors.pid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.PIDContext.Pid"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.PIDContext.Pid = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.ppid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.PPid"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.PPid = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.tid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.PIDContext.Tid"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.PIDContext.Tid = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.tty_name":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.TTYName"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.TTYName = str
		return nil
	case "ptrace.tracee.ancestors.uid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.UID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.UID = uint32(v)
		return nil
	case "ptrace.tracee.ancestors.user":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.User"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.User = str
		return nil
	case "ptrace.tracee.args":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Args"}
		}
		e.PTrace.Tracee.Process.Args = str
		return nil
	case "ptrace.tracee.args_flags":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Argv"}
		}
		e.PTrace.Tracee.Process.Argv = append(e.PTrace.Tracee.Process.Argv, str)
		return nil
	case "ptrace.tracee.args_options":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Argv"}
		}
		e.PTrace.Tracee.Process.Argv = append(e.PTrace.Tracee.Process.Argv, str)
		return nil
	case "ptrace.tracee.args_truncated":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		var ok bool
		if e.PTrace.Tracee.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.ArgsTruncated"}
		}
		return nil
	case "ptrace.tracee.argv":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Argv"}
		}
		e.PTrace.Tracee.Process.Argv = append(e.PTrace.Tracee.Process.Argv, str)
		return nil
	case "ptrace.tracee.argv0":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Argv0"}
		}
		e.PTrace.Tracee.Process.Argv0 = str
		return nil
	case "ptrace.tracee.cap_effective":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.CapEffective"}
		}
		e.PTrace.Tracee.Process.Credentials.CapEffective = uint64(v)
		return nil
	case "ptrace.tracee.cap_permitted":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.CapPermitted"}
		}
		e.PTrace.Tracee.Process.Credentials.CapPermitted = uint64(v)
		return nil
	case "ptrace.tracee.comm":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Comm"}
		}
		e.PTrace.Tracee.Process.Comm = str
		return nil
	case "ptrace.tracee.container.id":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.ContainerID"}
		}
		e.PTrace.Tracee.Process.ContainerID = str
		return nil
	case "ptrace.tracee.cookie":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Cookie"}
		}
		e.PTrace.Tracee.Process.Cookie = uint32(v)
		return nil
	case "ptrace.tracee.created_at":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.CreatedAt"}
		}
		e.PTrace.Tracee.Process.CreatedAt = uint64(v)
		return nil
	case "ptrace.tracee.egid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.EGID"}
		}
		e.PTrace.Tracee.Process.Credentials.EGID = uint32(v)
		return nil
	case "ptrace.tracee.egroup":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.EGroup"}
		}
		e.PTrace.Tracee.Process.Credentials.EGroup = str
		return nil
	case "ptrace.tracee.envp":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Envp"}
		}
		e.PTrace.Tracee.Process.Envp = append(e.PTrace.Tracee.Process.Envp, str)
		return nil
	case "ptrace.tracee.envs":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Envs"}
		}
		e.PTrace.Tracee.Process.Envs = append(e.PTrace.Tracee.Process.Envs, str)
		return nil
	case "ptrace.tracee.envs_truncated":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		var ok bool
		if e.PTrace.Tracee.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.EnvsTruncated"}
		}
		return nil
	case "ptrace.tracee.euid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.EUID"}
		}
		e.PTrace.Tracee.Process.Credentials.EUID = uint32(v)
		return nil
	case "ptrace.tracee.euser":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.EUser"}
		}
		e.PTrace.Tracee.Process.Credentials.EUser = str
		return nil
	case "ptrace.tracee.file.change_time":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.CTime"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "ptrace.tracee.file.filesystem":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.Filesystem"}
		}
		e.PTrace.Tracee.Process.FileEvent.Filesystem = str
		return nil
	case "ptrace.tracee.file.gid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.GID"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "ptrace.tracee.file.group":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.Group"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.Group = str
		return nil
	case "ptrace.tracee.file.in_upper_layer":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		var ok bool
		if e.PTrace.Tracee.Process.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "ptrace.tracee.file.inode":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.Inode"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "ptrace.tracee.file.mode":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.Mode"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "ptrace.tracee.file.modification_time":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.MTime"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "ptrace.tracee.file.mount_id":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.MountID"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "ptrace.tracee.file.name":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.BasenameStr"}
		}
		e.PTrace.Tracee.Process.FileEvent.BasenameStr = str
		return nil
	case "ptrace.tracee.file.name.length":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "ptrace.tracee.file.name.length"}
	case "ptrace.tracee.file.path":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.PathnameStr"}
		}
		e.PTrace.Tracee.Process.FileEvent.PathnameStr = str
		return nil
	case "ptrace.tracee.file.path.length":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "ptrace.tracee.file.path.length"}
	case "ptrace.tracee.file.rights":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.Mode"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "ptrace.tracee.file.uid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.UID"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "ptrace.tracee.file.user":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileEvent.FileFields.User"}
		}
		e.PTrace.Tracee.Process.FileEvent.FileFields.User = str
		return nil
	case "ptrace.tracee.fsgid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.FSGID"}
		}
		e.PTrace.Tracee.Process.Credentials.FSGID = uint32(v)
		return nil
	case "ptrace.tracee.fsgroup":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.FSGroup"}
		}
		e.PTrace.Tracee.Process.Credentials.FSGroup = str
		return nil
	case "ptrace.tracee.fsuid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.FSUID"}
		}
		e.PTrace.Tracee.Process.Credentials.FSUID = uint32(v)
		return nil
	case "ptrace.tracee.fsuser":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.FSUser"}
		}
		e.PTrace.Tracee.Process.Credentials.FSUser = str
		return nil
	case "ptrace.tracee.gid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.GID"}
		}
		e.PTrace.Tracee.Process.Credentials.GID = uint32(v)
		return nil
	case "ptrace.tracee.group":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.Group"}
		}
		e.PTrace.Tracee.Process.Credentials.Group = str
		return nil
	case "ptrace.tracee.interpreter.file.change_time":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "ptrace.tracee.interpreter.file.filesystem":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.Filesystem = str
		return nil
	case "ptrace.tracee.interpreter.file.gid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "ptrace.tracee.interpreter.file.group":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Group = str
		return nil
	case "ptrace.tracee.interpreter.file.in_upper_layer":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		var ok bool
		if e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "ptrace.tracee.interpreter.file.inode":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "ptrace.tracee.interpreter.file.mode":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "ptrace.tracee.interpreter.file.modification_time":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "ptrace.tracee.interpreter.file.mount_id":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "ptrace.tracee.interpreter.file.name":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.BasenameStr = str
		return nil
	case "ptrace.tracee.interpreter.file.name.length":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "ptrace.tracee.interpreter.file.name.length"}
	case "ptrace.tracee.interpreter.file.path":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.PathnameStr = str
		return nil
	case "ptrace.tracee.interpreter.file.path.length":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "ptrace.tracee.interpreter.file.path.length"}
	case "ptrace.tracee.interpreter.file.rights":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "ptrace.tracee.interpreter.file.uid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "ptrace.tracee.interpreter.file.user":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		e.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.User = str
		return nil
	case "ptrace.tracee.is_kworker":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		var ok bool
		if e.PTrace.Tracee.Process.PIDContext.IsKworker, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.PIDContext.IsKworker"}
		}
		return nil
	case "ptrace.tracee.is_thread":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		var ok bool
		if e.PTrace.Tracee.Process.IsThread, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.IsThread"}
		}
		return nil
	case "ptrace.tracee.pid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.PIDContext.Pid"}
		}
		e.PTrace.Tracee.Process.PIDContext.Pid = uint32(v)
		return nil
	case "ptrace.tracee.ppid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.PPid"}
		}
		e.PTrace.Tracee.Process.PPid = uint32(v)
		return nil
	case "ptrace.tracee.tid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.PIDContext.Tid"}
		}
		e.PTrace.Tracee.Process.PIDContext.Tid = uint32(v)
		return nil
	case "ptrace.tracee.tty_name":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.TTYName"}
		}
		e.PTrace.Tracee.Process.TTYName = str
		return nil
	case "ptrace.tracee.uid":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.UID"}
		}
		e.PTrace.Tracee.Process.Credentials.UID = uint32(v)
		return nil
	case "ptrace.tracee.user":
		if e.PTrace.Tracee == nil {
			e.PTrace.Tracee = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.User"}
		}
		e.PTrace.Tracee.Process.Credentials.User = str
		return nil
	case "removexattr.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.CTime"}
		}
		e.RemoveXAttr.File.FileFields.CTime = uint64(v)
		return nil
	case "removexattr.file.destination.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Name"}
		}
		e.RemoveXAttr.Name = str
		return nil
	case "removexattr.file.destination.namespace":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Namespace"}
		}
		e.RemoveXAttr.Namespace = str
		return nil
	case "removexattr.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.Filesystem"}
		}
		e.RemoveXAttr.File.Filesystem = str
		return nil
	case "removexattr.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.GID"}
		}
		e.RemoveXAttr.File.FileFields.GID = uint32(v)
		return nil
	case "removexattr.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.Group"}
		}
		e.RemoveXAttr.File.FileFields.Group = str
		return nil
	case "removexattr.file.in_upper_layer":
		var ok bool
		if e.RemoveXAttr.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.InUpperLayer"}
		}
		return nil
	case "removexattr.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.Inode"}
		}
		e.RemoveXAttr.File.FileFields.Inode = uint64(v)
		return nil
	case "removexattr.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.Mode"}
		}
		e.RemoveXAttr.File.FileFields.Mode = uint16(v)
		return nil
	case "removexattr.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.MTime"}
		}
		e.RemoveXAttr.File.FileFields.MTime = uint64(v)
		return nil
	case "removexattr.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.MountID"}
		}
		e.RemoveXAttr.File.FileFields.MountID = uint32(v)
		return nil
	case "removexattr.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.BasenameStr"}
		}
		e.RemoveXAttr.File.BasenameStr = str
		return nil
	case "removexattr.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "removexattr.file.name.length"}
	case "removexattr.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.PathnameStr"}
		}
		e.RemoveXAttr.File.PathnameStr = str
		return nil
	case "removexattr.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "removexattr.file.path.length"}
	case "removexattr.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.Mode"}
		}
		e.RemoveXAttr.File.FileFields.Mode = uint16(v)
		return nil
	case "removexattr.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.UID"}
		}
		e.RemoveXAttr.File.FileFields.UID = uint32(v)
		return nil
	case "removexattr.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.User"}
		}
		e.RemoveXAttr.File.FileFields.User = str
		return nil
	case "removexattr.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.SyscallEvent.Retval"}
		}
		e.RemoveXAttr.SyscallEvent.Retval = int64(v)
		return nil
	case "rename.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.CTime"}
		}
		e.Rename.Old.FileFields.CTime = uint64(v)
		return nil
	case "rename.file.destination.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.CTime"}
		}
		e.Rename.New.FileFields.CTime = uint64(v)
		return nil
	case "rename.file.destination.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.Filesystem"}
		}
		e.Rename.New.Filesystem = str
		return nil
	case "rename.file.destination.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.GID"}
		}
		e.Rename.New.FileFields.GID = uint32(v)
		return nil
	case "rename.file.destination.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Group"}
		}
		e.Rename.New.FileFields.Group = str
		return nil
	case "rename.file.destination.in_upper_layer":
		var ok bool
		if e.Rename.New.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.InUpperLayer"}
		}
		return nil
	case "rename.file.destination.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Inode"}
		}
		e.Rename.New.FileFields.Inode = uint64(v)
		return nil
	case "rename.file.destination.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Mode"}
		}
		e.Rename.New.FileFields.Mode = uint16(v)
		return nil
	case "rename.file.destination.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.MTime"}
		}
		e.Rename.New.FileFields.MTime = uint64(v)
		return nil
	case "rename.file.destination.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.MountID"}
		}
		e.Rename.New.FileFields.MountID = uint32(v)
		return nil
	case "rename.file.destination.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.BasenameStr"}
		}
		e.Rename.New.BasenameStr = str
		return nil
	case "rename.file.destination.name.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.destination.name.length"}
	case "rename.file.destination.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.PathnameStr"}
		}
		e.Rename.New.PathnameStr = str
		return nil
	case "rename.file.destination.path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.destination.path.length"}
	case "rename.file.destination.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Mode"}
		}
		e.Rename.New.FileFields.Mode = uint16(v)
		return nil
	case "rename.file.destination.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.UID"}
		}
		e.Rename.New.FileFields.UID = uint32(v)
		return nil
	case "rename.file.destination.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.User"}
		}
		e.Rename.New.FileFields.User = str
		return nil
	case "rename.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.Filesystem"}
		}
		e.Rename.Old.Filesystem = str
		return nil
	case "rename.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.GID"}
		}
		e.Rename.Old.FileFields.GID = uint32(v)
		return nil
	case "rename.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Group"}
		}
		e.Rename.Old.FileFields.Group = str
		return nil
	case "rename.file.in_upper_layer":
		var ok bool
		if e.Rename.Old.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.InUpperLayer"}
		}
		return nil
	case "rename.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Inode"}
		}
		e.Rename.Old.FileFields.Inode = uint64(v)
		return nil
	case "rename.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Mode"}
		}
		e.Rename.Old.FileFields.Mode = uint16(v)
		return nil
	case "rename.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.MTime"}
		}
		e.Rename.Old.FileFields.MTime = uint64(v)
		return nil
	case "rename.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.MountID"}
		}
		e.Rename.Old.FileFields.MountID = uint32(v)
		return nil
	case "rename.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.BasenameStr"}
		}
		e.Rename.Old.BasenameStr = str
		return nil
	case "rename.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.name.length"}
	case "rename.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.PathnameStr"}
		}
		e.Rename.Old.PathnameStr = str
		return nil
	case "rename.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.path.length"}
	case "rename.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Mode"}
		}
		e.Rename.Old.FileFields.Mode = uint16(v)
		return nil
	case "rename.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.UID"}
		}
		e.Rename.Old.FileFields.UID = uint32(v)
		return nil
	case "rename.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.User"}
		}
		e.Rename.Old.FileFields.User = str
		return nil
	case "rename.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.SyscallEvent.Retval"}
		}
		e.Rename.SyscallEvent.Retval = int64(v)
		return nil
	case "rmdir.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.CTime"}
		}
		e.Rmdir.File.FileFields.CTime = uint64(v)
		return nil
	case "rmdir.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.Filesystem"}
		}
		e.Rmdir.File.Filesystem = str
		return nil
	case "rmdir.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.GID"}
		}
		e.Rmdir.File.FileFields.GID = uint32(v)
		return nil
	case "rmdir.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.Group"}
		}
		e.Rmdir.File.FileFields.Group = str
		return nil
	case "rmdir.file.in_upper_layer":
		var ok bool
		if e.Rmdir.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.InUpperLayer"}
		}
		return nil
	case "rmdir.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.Inode"}
		}
		e.Rmdir.File.FileFields.Inode = uint64(v)
		return nil
	case "rmdir.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.Mode"}
		}
		e.Rmdir.File.FileFields.Mode = uint16(v)
		return nil
	case "rmdir.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.MTime"}
		}
		e.Rmdir.File.FileFields.MTime = uint64(v)
		return nil
	case "rmdir.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.MountID"}
		}
		e.Rmdir.File.FileFields.MountID = uint32(v)
		return nil
	case "rmdir.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.BasenameStr"}
		}
		e.Rmdir.File.BasenameStr = str
		return nil
	case "rmdir.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "rmdir.file.name.length"}
	case "rmdir.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.PathnameStr"}
		}
		e.Rmdir.File.PathnameStr = str
		return nil
	case "rmdir.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "rmdir.file.path.length"}
	case "rmdir.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.Mode"}
		}
		e.Rmdir.File.FileFields.Mode = uint16(v)
		return nil
	case "rmdir.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.UID"}
		}
		e.Rmdir.File.FileFields.UID = uint32(v)
		return nil
	case "rmdir.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.User"}
		}
		e.Rmdir.File.FileFields.User = str
		return nil
	case "rmdir.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.SyscallEvent.Retval"}
		}
		e.Rmdir.SyscallEvent.Retval = int64(v)
		return nil
	case "selinux.bool.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SELinux.BoolName"}
		}
		e.SELinux.BoolName = str
		return nil
	case "selinux.bool.state":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SELinux.BoolChangeValue"}
		}
		e.SELinux.BoolChangeValue = str
		return nil
	case "selinux.bool_commit.state":
		var ok bool
		if e.SELinux.BoolCommitValue, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SELinux.BoolCommitValue"}
		}
		return nil
	case "selinux.enforce.status":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SELinux.EnforceStatus"}
		}
		e.SELinux.EnforceStatus = str
		return nil
	case "setgid.egid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.EGID"}
		}
		e.SetGID.EGID = uint32(v)
		return nil
	case "setgid.egroup":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.EGroup"}
		}
		e.SetGID.EGroup = str
		return nil
	case "setgid.fsgid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.FSGID"}
		}
		e.SetGID.FSGID = uint32(v)
		return nil
	case "setgid.fsgroup":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.FSGroup"}
		}
		e.SetGID.FSGroup = str
		return nil
	case "setgid.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.GID"}
		}
		e.SetGID.GID = uint32(v)
		return nil
	case "setgid.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.Group"}
		}
		e.SetGID.Group = str
		return nil
	case "setuid.euid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.EUID"}
		}
		e.SetUID.EUID = uint32(v)
		return nil
	case "setuid.euser":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.EUser"}
		}
		e.SetUID.EUser = str
		return nil
	case "setuid.fsuid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.FSUID"}
		}
		e.SetUID.FSUID = uint32(v)
		return nil
	case "setuid.fsuser":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.FSUser"}
		}
		e.SetUID.FSUser = str
		return nil
	case "setuid.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.UID"}
		}
		e.SetUID.UID = uint32(v)
		return nil
	case "setuid.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.User"}
		}
		e.SetUID.User = str
		return nil
	case "setxattr.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.CTime"}
		}
		e.SetXAttr.File.FileFields.CTime = uint64(v)
		return nil
	case "setxattr.file.destination.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Name"}
		}
		e.SetXAttr.Name = str
		return nil
	case "setxattr.file.destination.namespace":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Namespace"}
		}
		e.SetXAttr.Namespace = str
		return nil
	case "setxattr.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.Filesystem"}
		}
		e.SetXAttr.File.Filesystem = str
		return nil
	case "setxattr.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.GID"}
		}
		e.SetXAttr.File.FileFields.GID = uint32(v)
		return nil
	case "setxattr.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.Group"}
		}
		e.SetXAttr.File.FileFields.Group = str
		return nil
	case "setxattr.file.in_upper_layer":
		var ok bool
		if e.SetXAttr.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.InUpperLayer"}
		}
		return nil
	case "setxattr.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.Inode"}
		}
		e.SetXAttr.File.FileFields.Inode = uint64(v)
		return nil
	case "setxattr.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.Mode"}
		}
		e.SetXAttr.File.FileFields.Mode = uint16(v)
		return nil
	case "setxattr.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.MTime"}
		}
		e.SetXAttr.File.FileFields.MTime = uint64(v)
		return nil
	case "setxattr.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.MountID"}
		}
		e.SetXAttr.File.FileFields.MountID = uint32(v)
		return nil
	case "setxattr.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.BasenameStr"}
		}
		e.SetXAttr.File.BasenameStr = str
		return nil
	case "setxattr.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "setxattr.file.name.length"}
	case "setxattr.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.PathnameStr"}
		}
		e.SetXAttr.File.PathnameStr = str
		return nil
	case "setxattr.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "setxattr.file.path.length"}
	case "setxattr.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.Mode"}
		}
		e.SetXAttr.File.FileFields.Mode = uint16(v)
		return nil
	case "setxattr.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.UID"}
		}
		e.SetXAttr.File.FileFields.UID = uint32(v)
		return nil
	case "setxattr.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.User"}
		}
		e.SetXAttr.File.FileFields.User = str
		return nil
	case "setxattr.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.SyscallEvent.Retval"}
		}
		e.SetXAttr.SyscallEvent.Retval = int64(v)
		return nil
	case "signal.pid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.PID"}
		}
		e.Signal.PID = uint32(v)
		return nil
	case "signal.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.SyscallEvent.Retval"}
		}
		e.Signal.SyscallEvent.Retval = int64(v)
		return nil
	case "signal.target.ancestors.args":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Args"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Args = str
		return nil
	case "signal.target.ancestors.args_flags":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Argv"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Argv = append(e.Signal.Target.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "signal.target.ancestors.args_options":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Argv"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Argv = append(e.Signal.Target.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "signal.target.ancestors.args_truncated":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.ArgsTruncated"}
		}
		return nil
	case "signal.target.ancestors.argv":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Argv"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Argv = append(e.Signal.Target.Ancestor.ProcessContext.Process.Argv, str)
		return nil
	case "signal.target.ancestors.argv0":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Argv0"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Argv0 = str
		return nil
	case "signal.target.ancestors.cap_effective":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.CapEffective"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.CapEffective = uint64(v)
		return nil
	case "signal.target.ancestors.cap_permitted":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.CapPermitted"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.CapPermitted = uint64(v)
		return nil
	case "signal.target.ancestors.comm":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Comm"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Comm = str
		return nil
	case "signal.target.ancestors.container.id":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.ContainerID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.ContainerID = str
		return nil
	case "signal.target.ancestors.cookie":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Cookie"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Cookie = uint32(v)
		return nil
	case "signal.target.ancestors.created_at":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.CreatedAt"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.CreatedAt = uint64(v)
		return nil
	case "signal.target.ancestors.egid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.EGID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.EGID = uint32(v)
		return nil
	case "signal.target.ancestors.egroup":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.EGroup"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.EGroup = str
		return nil
	case "signal.target.ancestors.envp":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Envp"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Envp = append(e.Signal.Target.Ancestor.ProcessContext.Process.Envp, str)
		return nil
	case "signal.target.ancestors.envs":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Envs"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Envs = append(e.Signal.Target.Ancestor.ProcessContext.Process.Envs, str)
		return nil
	case "signal.target.ancestors.envs_truncated":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.EnvsTruncated"}
		}
		return nil
	case "signal.target.ancestors.euid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.EUID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.EUID = uint32(v)
		return nil
	case "signal.target.ancestors.euser":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.EUser"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.EUser = str
		return nil
	case "signal.target.ancestors.file.change_time":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.CTime"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "signal.target.ancestors.file.filesystem":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.Filesystem"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.Filesystem = str
		return nil
	case "signal.target.ancestors.file.gid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.GID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "signal.target.ancestors.file.group":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.Group"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.Group = str
		return nil
	case "signal.target.ancestors.file.in_upper_layer":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "signal.target.ancestors.file.inode":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.Inode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "signal.target.ancestors.file.mode":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "signal.target.ancestors.file.modification_time":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.MTime"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "signal.target.ancestors.file.mount_id":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.MountID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "signal.target.ancestors.file.name":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.BasenameStr"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.BasenameStr = str
		return nil
	case "signal.target.ancestors.file.name.length":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "signal.target.ancestors.file.name.length"}
	case "signal.target.ancestors.file.path":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.PathnameStr"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.PathnameStr = str
		return nil
	case "signal.target.ancestors.file.path.length":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "signal.target.ancestors.file.path.length"}
	case "signal.target.ancestors.file.rights":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "signal.target.ancestors.file.uid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.UID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "signal.target.ancestors.file.user":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.User"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileEvent.FileFields.User = str
		return nil
	case "signal.target.ancestors.fsgid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSGID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSGID = uint32(v)
		return nil
	case "signal.target.ancestors.fsgroup":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSGroup"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSGroup = str
		return nil
	case "signal.target.ancestors.fsuid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSUID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSUID = uint32(v)
		return nil
	case "signal.target.ancestors.fsuser":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSUser"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSUser = str
		return nil
	case "signal.target.ancestors.gid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.GID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.GID = uint32(v)
		return nil
	case "signal.target.ancestors.group":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.Group"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.Group = str
		return nil
	case "signal.target.ancestors.interpreter.file.change_time":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "signal.target.ancestors.interpreter.file.filesystem":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.Filesystem = str
		return nil
	case "signal.target.ancestors.interpreter.file.gid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "signal.target.ancestors.interpreter.file.group":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Group = str
		return nil
	case "signal.target.ancestors.interpreter.file.in_upper_layer":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "signal.target.ancestors.interpreter.file.inode":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "signal.target.ancestors.interpreter.file.mode":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "signal.target.ancestors.interpreter.file.modification_time":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "signal.target.ancestors.interpreter.file.mount_id":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "signal.target.ancestors.interpreter.file.name":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.BasenameStr = str
		return nil
	case "signal.target.ancestors.interpreter.file.name.length":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "signal.target.ancestors.interpreter.file.name.length"}
	case "signal.target.ancestors.interpreter.file.path":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.PathnameStr = str
		return nil
	case "signal.target.ancestors.interpreter.file.path.length":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "signal.target.ancestors.interpreter.file.path.length"}
	case "signal.target.ancestors.interpreter.file.rights":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "signal.target.ancestors.interpreter.file.uid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "signal.target.ancestors.interpreter.file.user":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.User = str
		return nil
	case "signal.target.ancestors.is_kworker":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.PIDContext.IsKworker, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.PIDContext.IsKworker"}
		}
		return nil
	case "signal.target.ancestors.is_thread":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.IsThread, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.IsThread"}
		}
		return nil
	case "signal.target.ancestors.pid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.PIDContext.Pid"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.PIDContext.Pid = uint32(v)
		return nil
	case "signal.target.ancestors.ppid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.PPid"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.PPid = uint32(v)
		return nil
	case "signal.target.ancestors.tid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.PIDContext.Tid"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.PIDContext.Tid = uint32(v)
		return nil
	case "signal.target.ancestors.tty_name":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.TTYName"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.TTYName = str
		return nil
	case "signal.target.ancestors.uid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.UID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.UID = uint32(v)
		return nil
	case "signal.target.ancestors.user":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &ProcessCacheEntry{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.User"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.User = str
		return nil
	case "signal.target.args":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Args"}
		}
		e.Signal.Target.Process.Args = str
		return nil
	case "signal.target.args_flags":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Argv"}
		}
		e.Signal.Target.Process.Argv = append(e.Signal.Target.Process.Argv, str)
		return nil
	case "signal.target.args_options":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Argv"}
		}
		e.Signal.Target.Process.Argv = append(e.Signal.Target.Process.Argv, str)
		return nil
	case "signal.target.args_truncated":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		var ok bool
		if e.Signal.Target.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.ArgsTruncated"}
		}
		return nil
	case "signal.target.argv":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Argv"}
		}
		e.Signal.Target.Process.Argv = append(e.Signal.Target.Process.Argv, str)
		return nil
	case "signal.target.argv0":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Argv0"}
		}
		e.Signal.Target.Process.Argv0 = str
		return nil
	case "signal.target.cap_effective":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.CapEffective"}
		}
		e.Signal.Target.Process.Credentials.CapEffective = uint64(v)
		return nil
	case "signal.target.cap_permitted":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.CapPermitted"}
		}
		e.Signal.Target.Process.Credentials.CapPermitted = uint64(v)
		return nil
	case "signal.target.comm":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Comm"}
		}
		e.Signal.Target.Process.Comm = str
		return nil
	case "signal.target.container.id":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.ContainerID"}
		}
		e.Signal.Target.Process.ContainerID = str
		return nil
	case "signal.target.cookie":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Cookie"}
		}
		e.Signal.Target.Process.Cookie = uint32(v)
		return nil
	case "signal.target.created_at":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.CreatedAt"}
		}
		e.Signal.Target.Process.CreatedAt = uint64(v)
		return nil
	case "signal.target.egid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.EGID"}
		}
		e.Signal.Target.Process.Credentials.EGID = uint32(v)
		return nil
	case "signal.target.egroup":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.EGroup"}
		}
		e.Signal.Target.Process.Credentials.EGroup = str
		return nil
	case "signal.target.envp":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Envp"}
		}
		e.Signal.Target.Process.Envp = append(e.Signal.Target.Process.Envp, str)
		return nil
	case "signal.target.envs":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Envs"}
		}
		e.Signal.Target.Process.Envs = append(e.Signal.Target.Process.Envs, str)
		return nil
	case "signal.target.envs_truncated":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		var ok bool
		if e.Signal.Target.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.EnvsTruncated"}
		}
		return nil
	case "signal.target.euid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.EUID"}
		}
		e.Signal.Target.Process.Credentials.EUID = uint32(v)
		return nil
	case "signal.target.euser":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.EUser"}
		}
		e.Signal.Target.Process.Credentials.EUser = str
		return nil
	case "signal.target.file.change_time":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.CTime"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "signal.target.file.filesystem":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.Filesystem"}
		}
		e.Signal.Target.Process.FileEvent.Filesystem = str
		return nil
	case "signal.target.file.gid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.GID"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "signal.target.file.group":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.Group"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.Group = str
		return nil
	case "signal.target.file.in_upper_layer":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		var ok bool
		if e.Signal.Target.Process.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "signal.target.file.inode":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.Inode"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "signal.target.file.mode":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.Mode"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "signal.target.file.modification_time":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.MTime"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "signal.target.file.mount_id":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.MountID"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "signal.target.file.name":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.BasenameStr"}
		}
		e.Signal.Target.Process.FileEvent.BasenameStr = str
		return nil
	case "signal.target.file.name.length":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "signal.target.file.name.length"}
	case "signal.target.file.path":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.PathnameStr"}
		}
		e.Signal.Target.Process.FileEvent.PathnameStr = str
		return nil
	case "signal.target.file.path.length":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "signal.target.file.path.length"}
	case "signal.target.file.rights":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.Mode"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "signal.target.file.uid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.UID"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "signal.target.file.user":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileEvent.FileFields.User"}
		}
		e.Signal.Target.Process.FileEvent.FileFields.User = str
		return nil
	case "signal.target.fsgid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.FSGID"}
		}
		e.Signal.Target.Process.Credentials.FSGID = uint32(v)
		return nil
	case "signal.target.fsgroup":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.FSGroup"}
		}
		e.Signal.Target.Process.Credentials.FSGroup = str
		return nil
	case "signal.target.fsuid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.FSUID"}
		}
		e.Signal.Target.Process.Credentials.FSUID = uint32(v)
		return nil
	case "signal.target.fsuser":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.FSUser"}
		}
		e.Signal.Target.Process.Credentials.FSUser = str
		return nil
	case "signal.target.gid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.GID"}
		}
		e.Signal.Target.Process.Credentials.GID = uint32(v)
		return nil
	case "signal.target.group":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.Group"}
		}
		e.Signal.Target.Process.Credentials.Group = str
		return nil
	case "signal.target.interpreter.file.change_time":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime = uint64(v)
		return nil
	case "signal.target.interpreter.file.filesystem":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.Filesystem"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.Filesystem = str
		return nil
	case "signal.target.interpreter.file.gid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID = uint32(v)
		return nil
	case "signal.target.interpreter.file.group":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Group"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Group = str
		return nil
	case "signal.target.interpreter.file.in_upper_layer":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		var ok bool
		if e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.InUpperLayer"}
		}
		return nil
	case "signal.target.interpreter.file.inode":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Inode"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Inode = uint64(v)
		return nil
	case "signal.target.interpreter.file.mode":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "signal.target.interpreter.file.modification_time":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime = uint64(v)
		return nil
	case "signal.target.interpreter.file.mount_id":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MountID"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MountID = uint32(v)
		return nil
	case "signal.target.interpreter.file.name":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.BasenameStr"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.BasenameStr = str
		return nil
	case "signal.target.interpreter.file.name.length":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "signal.target.interpreter.file.name.length"}
	case "signal.target.interpreter.file.path":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.PathnameStr"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.PathnameStr = str
		return nil
	case "signal.target.interpreter.file.path.length":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "signal.target.interpreter.file.path.length"}
	case "signal.target.interpreter.file.rights":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode = uint16(v)
		return nil
	case "signal.target.interpreter.file.uid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID = uint32(v)
		return nil
	case "signal.target.interpreter.file.user":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.User"}
		}
		e.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.User = str
		return nil
	case "signal.target.is_kworker":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		var ok bool
		if e.Signal.Target.Process.PIDContext.IsKworker, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.PIDContext.IsKworker"}
		}
		return nil
	case "signal.target.is_thread":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		var ok bool
		if e.Signal.Target.Process.IsThread, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.IsThread"}
		}
		return nil
	case "signal.target.pid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.PIDContext.Pid"}
		}
		e.Signal.Target.Process.PIDContext.Pid = uint32(v)
		return nil
	case "signal.target.ppid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.PPid"}
		}
		e.Signal.Target.Process.PPid = uint32(v)
		return nil
	case "signal.target.tid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.PIDContext.Tid"}
		}
		e.Signal.Target.Process.PIDContext.Tid = uint32(v)
		return nil
	case "signal.target.tty_name":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.TTYName"}
		}
		e.Signal.Target.Process.TTYName = str
		return nil
	case "signal.target.uid":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.UID"}
		}
		e.Signal.Target.Process.Credentials.UID = uint32(v)
		return nil
	case "signal.target.user":
		if e.Signal.Target == nil {
			e.Signal.Target = &ProcessContext{}
		}
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.User"}
		}
		e.Signal.Target.Process.Credentials.User = str
		return nil
	case "signal.type":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Type"}
		}
		e.Signal.Type = uint32(v)
		return nil
	case "splice.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.CTime"}
		}
		e.Splice.File.FileFields.CTime = uint64(v)
		return nil
	case "splice.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.Filesystem"}
		}
		e.Splice.File.Filesystem = str
		return nil
	case "splice.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.GID"}
		}
		e.Splice.File.FileFields.GID = uint32(v)
		return nil
	case "splice.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.Group"}
		}
		e.Splice.File.FileFields.Group = str
		return nil
	case "splice.file.in_upper_layer":
		var ok bool
		if e.Splice.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.InUpperLayer"}
		}
		return nil
	case "splice.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.Inode"}
		}
		e.Splice.File.FileFields.Inode = uint64(v)
		return nil
	case "splice.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.Mode"}
		}
		e.Splice.File.FileFields.Mode = uint16(v)
		return nil
	case "splice.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.MTime"}
		}
		e.Splice.File.FileFields.MTime = uint64(v)
		return nil
	case "splice.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.MountID"}
		}
		e.Splice.File.FileFields.MountID = uint32(v)
		return nil
	case "splice.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.BasenameStr"}
		}
		e.Splice.File.BasenameStr = str
		return nil
	case "splice.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "splice.file.name.length"}
	case "splice.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.PathnameStr"}
		}
		e.Splice.File.PathnameStr = str
		return nil
	case "splice.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "splice.file.path.length"}
	case "splice.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.Mode"}
		}
		e.Splice.File.FileFields.Mode = uint16(v)
		return nil
	case "splice.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.UID"}
		}
		e.Splice.File.FileFields.UID = uint32(v)
		return nil
	case "splice.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.User"}
		}
		e.Splice.File.FileFields.User = str
		return nil
	case "splice.pipe_entry_flag":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.PipeEntryFlag"}
		}
		e.Splice.PipeEntryFlag = uint32(v)
		return nil
	case "splice.pipe_exit_flag":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.PipeExitFlag"}
		}
		e.Splice.PipeExitFlag = uint32(v)
		return nil
	case "splice.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.SyscallEvent.Retval"}
		}
		e.Splice.SyscallEvent.Retval = int64(v)
		return nil
	case "unlink.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.CTime"}
		}
		e.Unlink.File.FileFields.CTime = uint64(v)
		return nil
	case "unlink.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.Filesystem"}
		}
		e.Unlink.File.Filesystem = str
		return nil
	case "unlink.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.GID"}
		}
		e.Unlink.File.FileFields.GID = uint32(v)
		return nil
	case "unlink.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.Group"}
		}
		e.Unlink.File.FileFields.Group = str
		return nil
	case "unlink.file.in_upper_layer":
		var ok bool
		if e.Unlink.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.InUpperLayer"}
		}
		return nil
	case "unlink.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.Inode"}
		}
		e.Unlink.File.FileFields.Inode = uint64(v)
		return nil
	case "unlink.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.Mode"}
		}
		e.Unlink.File.FileFields.Mode = uint16(v)
		return nil
	case "unlink.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.MTime"}
		}
		e.Unlink.File.FileFields.MTime = uint64(v)
		return nil
	case "unlink.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.MountID"}
		}
		e.Unlink.File.FileFields.MountID = uint32(v)
		return nil
	case "unlink.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.BasenameStr"}
		}
		e.Unlink.File.BasenameStr = str
		return nil
	case "unlink.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "unlink.file.name.length"}
	case "unlink.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.PathnameStr"}
		}
		e.Unlink.File.PathnameStr = str
		return nil
	case "unlink.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "unlink.file.path.length"}
	case "unlink.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.Mode"}
		}
		e.Unlink.File.FileFields.Mode = uint16(v)
		return nil
	case "unlink.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.UID"}
		}
		e.Unlink.File.FileFields.UID = uint32(v)
		return nil
	case "unlink.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.User"}
		}
		e.Unlink.File.FileFields.User = str
		return nil
	case "unlink.flags":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.Flags"}
		}
		e.Unlink.Flags = uint32(v)
		return nil
	case "unlink.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.SyscallEvent.Retval"}
		}
		e.Unlink.SyscallEvent.Retval = int64(v)
		return nil
	case "unload_module.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "UnloadModule.Name"}
		}
		e.UnloadModule.Name = str
		return nil
	case "unload_module.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "UnloadModule.SyscallEvent.Retval"}
		}
		e.UnloadModule.SyscallEvent.Retval = int64(v)
		return nil
	case "utimes.file.change_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.CTime"}
		}
		e.Utimes.File.FileFields.CTime = uint64(v)
		return nil
	case "utimes.file.filesystem":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.Filesystem"}
		}
		e.Utimes.File.Filesystem = str
		return nil
	case "utimes.file.gid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.GID"}
		}
		e.Utimes.File.FileFields.GID = uint32(v)
		return nil
	case "utimes.file.group":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.Group"}
		}
		e.Utimes.File.FileFields.Group = str
		return nil
	case "utimes.file.in_upper_layer":
		var ok bool
		if e.Utimes.File.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.InUpperLayer"}
		}
		return nil
	case "utimes.file.inode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.Inode"}
		}
		e.Utimes.File.FileFields.Inode = uint64(v)
		return nil
	case "utimes.file.mode":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.Mode"}
		}
		e.Utimes.File.FileFields.Mode = uint16(v)
		return nil
	case "utimes.file.modification_time":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.MTime"}
		}
		e.Utimes.File.FileFields.MTime = uint64(v)
		return nil
	case "utimes.file.mount_id":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.MountID"}
		}
		e.Utimes.File.FileFields.MountID = uint32(v)
		return nil
	case "utimes.file.name":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.BasenameStr"}
		}
		e.Utimes.File.BasenameStr = str
		return nil
	case "utimes.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "utimes.file.name.length"}
	case "utimes.file.path":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.PathnameStr"}
		}
		e.Utimes.File.PathnameStr = str
		return nil
	case "utimes.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "utimes.file.path.length"}
	case "utimes.file.rights":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.Mode"}
		}
		e.Utimes.File.FileFields.Mode = uint16(v)
		return nil
	case "utimes.file.uid":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.UID"}
		}
		e.Utimes.File.FileFields.UID = uint32(v)
		return nil
	case "utimes.file.user":
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.User"}
		}
		e.Utimes.File.FileFields.User = str
		return nil
	case "utimes.retval":
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.SyscallEvent.Retval"}
		}
		e.Utimes.SyscallEvent.Retval = int64(v)
		return nil
	}
	return &eval.ErrFieldNotFound{Field: field}
}
