//go:build linux
// +build linux

// Code generated - DO NOT EDIT.

package probe

import (
	"reflect"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// suppress unused package warning
var (
	_ *unsafe.Pointer
)

func (m *Model) GetIterator(field eval.Field) (eval.Iterator, error) {
	switch field {

	case "process.ancestors":
		return &model.ProcessAncestorsIterator{}, nil

	case "ptrace.tracee.ancestors":
		return &model.ProcessAncestorsIterator{}, nil

	case "signal.target.ancestors":
		return &model.ProcessAncestorsIterator{}, nil

	}

	return nil, &eval.ErrIteratorNotSupported{Field: field}
}

func (m *Model) GetEventTypes() []eval.EventType {
	return []eval.EventType{

		eval.EventType("bpf"),

		eval.EventType("capset"),

		eval.EventType("chmod"),

		eval.EventType("chown"),

		eval.EventType("dns"),

		eval.EventType("exec"),

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

				result := make([]int, len((*Event)(ctx.Object).ResolveHelpers(&(*Event)(ctx.Object).BPF.Program)))
				for i, v := range (*Event)(ctx.Object).ResolveHelpers(&(*Event)(ctx.Object).BPF.Program) {
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Chmod.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Chmod.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Chmod.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Chmod.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Chmod.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "chmod.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Chmod.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Chmod.File.FileFields)
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

				return (*Event)(ctx.Object).ResolveChownGID(&(*Event)(ctx.Object).Chown)
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

				return (*Event)(ctx.Object).ResolveChownUID(&(*Event)(ctx.Object).Chown)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Chown.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Chown.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Chown.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Chown.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Chown.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "chown.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Chown.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Chown.File.FileFields)
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

				return (*Event)(ctx.Object).ResolveContainerID(&(*Event)(ctx.Object).ContainerContext)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveContainerTags(&(*Event)(ctx.Object).ContainerContext)
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
		}, nil

	case "dns.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).DNS.Name
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "dns.qclass":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).DNS.QClass)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "dns.qdcount":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).DNS.QDCount)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "dns.qtype":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).DNS.QType)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "dns.retval":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).DNS.SyscallEvent.Retval)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveProcessArgs(&(*Event)(ctx.Object).Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "exec.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgsFlags(&(*Event)(ctx.Object).Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "exec.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgsOptions(&(*Event)(ctx.Object).Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "exec.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveProcessArgsTruncated(&(*Event)(ctx.Object).Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "exec.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgv(&(*Event)(ctx.Object).Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "exec.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveProcessArgv0(&(*Event)(ctx.Object).Exec.Process)
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

				return int((*Event)(ctx.Object).ResolveProcessCreatedAt(&(*Event)(ctx.Object).Exec.Process))
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

				return (*Event)(ctx.Object).ResolveProcessEnvp(&(*Event)(ctx.Object).Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "exec.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessEnvs(&(*Event)(ctx.Object).Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "exec.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&(*Event)(ctx.Object).Exec.Process)
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

				return int((*Event)(ctx.Object).Exec.Process.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Process.Filesystem
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Process.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Exec.Process.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "exec.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Exec.Process.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "exec.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Process.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Process.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Process.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Process.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Process.BasenameStr
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Exec.Process.PathnameStr
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Exec.Process.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "exec.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Process.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "exec.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Exec.Process.FileFields)
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

	case "exec.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Exec.Process.Pid)
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

				return int((*Event)(ctx.Object).Exec.Process.Tid)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Link.Target)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Link.Target.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.destination.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Link.Target.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Link.Target)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.destination.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Link.Target)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.destination.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Link.Target.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Link.Target.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Link.Source)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Link.Source.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Link.Source.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Link.Source)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Link.Source)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "link.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Link.Source.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Link.Source.FileFields)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).LoadModule.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).LoadModule.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "load_module.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).LoadModule.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).LoadModule.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "load_module.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).LoadModule.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "load_module.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).LoadModule.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).LoadModule.File.FileFields)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Mkdir.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Mkdir.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Mkdir.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Mkdir.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Mkdir.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "mkdir.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Mkdir.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Mkdir.File.FileFields)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).MMap.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).MMap.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "mmap.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).MMap.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).MMap.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "mmap.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).MMap.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "mmap.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).MMap.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).MMap.File.FileFields)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Open.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Open.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "open.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Open.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Open.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "open.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Open.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "open.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Open.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Open.File.FileFields)
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
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgs(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "process.ancestors.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsFlags(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsOptions(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.args_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsTruncated(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgv(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.argv0":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgv0(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "process.ancestors.cap_effective":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.CapEffective)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.cap_permitted":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.CapPermitted)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.comm":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Comm

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.ContainerID

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.cookie":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Cookie)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int((*Event)(ctx.Object).ResolveProcessCreatedAt(&element.Process))

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.egid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.EGID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.egroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.EGroup

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvp(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "process.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvs(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "process.ancestors.envs_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.euid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.EUID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.euser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.EUser

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.CTime)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Filesystem

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.GID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsGroup(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.Inode)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.Mode)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.MTime)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.MountID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.BasenameStr

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.PathnameStr

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int((*Event)(ctx.Object).ResolveRights(&element.FileFields))

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.UID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsUser(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.fsgid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.FSGID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.fsgroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.FSGroup

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.fsuid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.FSUID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.fsuser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.FSUser

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.GID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.Group

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Pid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.PPid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.tid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Tid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.tty_name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.TTYName

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.UID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.User

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "process.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveProcessArgs(&(*Event)(ctx.Object).ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "process.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgsFlags(&(*Event)(ctx.Object).ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "process.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgsOptions(&(*Event)(ctx.Object).ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "process.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveProcessArgsTruncated(&(*Event)(ctx.Object).ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "process.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgv(&(*Event)(ctx.Object).ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "process.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveProcessArgv0(&(*Event)(ctx.Object).ProcessContext.Process)
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

				return int((*Event)(ctx.Object).ResolveProcessCreatedAt(&(*Event)(ctx.Object).ProcessContext.Process))
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

				return (*Event)(ctx.Object).ResolveProcessEnvp(&(*Event)(ctx.Object).ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "process.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessEnvs(&(*Event)(ctx.Object).ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "process.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&(*Event)(ctx.Object).ProcessContext.Process)
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

				return int((*Event)(ctx.Object).ProcessContext.Process.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ProcessContext.Process.Filesystem
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ProcessContext.Process.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).ProcessContext.Process.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "process.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).ProcessContext.Process.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "process.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ProcessContext.Process.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ProcessContext.Process.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ProcessContext.Process.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ProcessContext.Process.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ProcessContext.Process.BasenameStr
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ProcessContext.Process.PathnameStr
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).ProcessContext.Process.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "process.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ProcessContext.Process.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "process.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).ProcessContext.Process.FileFields)
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

	case "process.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ProcessContext.Process.Pid)
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

				return int((*Event)(ctx.Object).ProcessContext.Process.Tid)
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
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgs(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsFlags(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsOptions(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.args_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsTruncated(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgv(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.argv0":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgv0(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.cap_effective":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.CapEffective)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.cap_permitted":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.CapPermitted)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.comm":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Comm

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.ContainerID

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.cookie":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Cookie)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int((*Event)(ctx.Object).ResolveProcessCreatedAt(&element.Process))

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.egid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.EGID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.egroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.EGroup

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvp(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvs(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.envs_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.euid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.EUID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.euser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.EUser

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.CTime)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Filesystem

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.GID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsGroup(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.Inode)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.Mode)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.MTime)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.MountID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.BasenameStr

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.PathnameStr

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int((*Event)(ctx.Object).ResolveRights(&element.FileFields))

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.UID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsUser(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.fsgid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.FSGID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.fsgroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.FSGroup

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.fsuid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.FSUID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.fsuser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.FSUser

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.GID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.Group

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Pid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.PPid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.tid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Tid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.tty_name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.TTYName

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.UID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.User

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "ptrace.tracee.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveProcessArgs(&(*Event)(ctx.Object).PTrace.Tracee.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgsFlags(&(*Event)(ctx.Object).PTrace.Tracee.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgsOptions(&(*Event)(ctx.Object).PTrace.Tracee.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveProcessArgsTruncated(&(*Event)(ctx.Object).PTrace.Tracee.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgv(&(*Event)(ctx.Object).PTrace.Tracee.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveProcessArgv0(&(*Event)(ctx.Object).PTrace.Tracee.Process)
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

				return int((*Event)(ctx.Object).ResolveProcessCreatedAt(&(*Event)(ctx.Object).PTrace.Tracee.Process))
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

				return (*Event)(ctx.Object).ResolveProcessEnvp(&(*Event)(ctx.Object).PTrace.Tracee.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessEnvs(&(*Event)(ctx.Object).PTrace.Tracee.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&(*Event)(ctx.Object).PTrace.Tracee.Process)
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

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).PTrace.Tracee.Process.Filesystem
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).PTrace.Tracee.Process.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).PTrace.Tracee.Process.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).PTrace.Tracee.Process.BasenameStr
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).PTrace.Tracee.Process.PathnameStr
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).PTrace.Tracee.Process.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "ptrace.tracee.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "ptrace.tracee.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).PTrace.Tracee.Process.FileFields)
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

	case "ptrace.tracee.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Pid)
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

				return int((*Event)(ctx.Object).PTrace.Tracee.Process.Tid)
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

				return (*Event)(ctx.Object).ResolveXAttrName(&(*Event)(ctx.Object).RemoveXAttr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.destination.namespace":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveXAttrNamespace(&(*Event)(ctx.Object).RemoveXAttr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).RemoveXAttr.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).RemoveXAttr.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).RemoveXAttr.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).RemoveXAttr.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).RemoveXAttr.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "removexattr.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).RemoveXAttr.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).RemoveXAttr.File.FileFields)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Rename.New)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Rename.New.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.destination.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Rename.New.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Rename.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.destination.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Rename.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.destination.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Rename.New.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Rename.New.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Rename.Old)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Rename.Old.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Rename.Old.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Rename.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Rename.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rename.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Rename.Old.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Rename.Old.FileFields)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Rmdir.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Rmdir.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Rmdir.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Rmdir.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Rmdir.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "rmdir.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Rmdir.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Rmdir.File.FileFields)
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

				return (*Event)(ctx.Object).ResolveSELinuxBoolName(&(*Event)(ctx.Object).SELinux)
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

				return (*Event)(ctx.Object).ResolveSetgidEGroup(&(*Event)(ctx.Object).SetGID)
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

				return (*Event)(ctx.Object).ResolveSetgidFSGroup(&(*Event)(ctx.Object).SetGID)
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

				return (*Event)(ctx.Object).ResolveSetgidGroup(&(*Event)(ctx.Object).SetGID)
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

				return (*Event)(ctx.Object).ResolveSetuidEUser(&(*Event)(ctx.Object).SetUID)
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

				return (*Event)(ctx.Object).ResolveSetuidFSUser(&(*Event)(ctx.Object).SetUID)
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

				return (*Event)(ctx.Object).ResolveSetuidUser(&(*Event)(ctx.Object).SetUID)
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

				return (*Event)(ctx.Object).ResolveXAttrName(&(*Event)(ctx.Object).SetXAttr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.destination.namespace":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveXAttrNamespace(&(*Event)(ctx.Object).SetXAttr)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).SetXAttr.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).SetXAttr.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).SetXAttr.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).SetXAttr.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).SetXAttr.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "setxattr.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).SetXAttr.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).SetXAttr.File.FileFields)
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
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgs(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsFlags(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsOptions(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.args_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgsTruncated(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgv(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.argv0":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessArgv0(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.cap_effective":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.CapEffective)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.cap_permitted":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.CapPermitted)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.comm":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Comm

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.ContainerID

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.cookie":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Cookie)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int((*Event)(ctx.Object).ResolveProcessCreatedAt(&element.Process))

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.egid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.EGID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.egroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.EGroup

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvp(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result []string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvs(&element.Process)

					results = append(results, result...)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.envs_truncated":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&element.Process)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.euid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.EUID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.euser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.EUser

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.change_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.CTime)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.filesystem":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Filesystem

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.GID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsGroup(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.in_upper_layer":
		return &eval.BoolArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []bool {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]bool)(ptr); result != nil {
						return *result
					}
				}
				var results []bool

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result bool

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.inode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.Inode)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.mode":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.Mode)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.modification_time":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.MTime)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.mount_id":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.MountID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.BasenameStr

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.PathnameStr

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.rights":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int((*Event)(ctx.Object).ResolveRights(&element.FileFields))

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.FileFields.UID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.file.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = (*Event)(ctx.Object).ResolveFileFieldsUser(&element.FileFields)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.fsgid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.FSGID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.fsgroup":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.FSGroup

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.fsuid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.FSUID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.fsuser":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.FSUser

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.gid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.GID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.group":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.Group

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Pid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.PPid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.tid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Tid)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.tty_name":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.TTYName

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.uid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]int)(ptr); result != nil {
						return *result
					}
				}
				var results []int

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result int

					element := (*model.ProcessCacheEntry)(value)

					result = int(element.ProcessContext.Process.Credentials.UID)

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if ptr := ctx.Cache[field]; ptr != nil {
					if result := (*[]string)(ptr); result != nil {
						return *result
					}
				}
				var results []string

				iterator := &model.ProcessAncestorsIterator{}

				value := iterator.Front(ctx)
				for value != nil {
					var result string

					element := (*model.ProcessCacheEntry)(value)

					result = element.ProcessContext.Process.Credentials.User

					results = append(results, result)

					value = iterator.Next()
				}
				ctx.Cache[field] = unsafe.Pointer(&results)

				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil

	case "signal.target.args":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveProcessArgs(&(*Event)(ctx.Object).Signal.Target.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "signal.target.args_flags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgsFlags(&(*Event)(ctx.Object).Signal.Target.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "signal.target.args_options":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgsOptions(&(*Event)(ctx.Object).Signal.Target.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "signal.target.args_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveProcessArgsTruncated(&(*Event)(ctx.Object).Signal.Target.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "signal.target.argv":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessArgv(&(*Event)(ctx.Object).Signal.Target.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "signal.target.argv0":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveProcessArgv0(&(*Event)(ctx.Object).Signal.Target.Process)
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

				return int((*Event)(ctx.Object).ResolveProcessCreatedAt(&(*Event)(ctx.Object).Signal.Target.Process))
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

				return (*Event)(ctx.Object).ResolveProcessEnvp(&(*Event)(ctx.Object).Signal.Target.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "signal.target.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {

				return (*Event)(ctx.Object).ResolveProcessEnvs(&(*Event)(ctx.Object).Signal.Target.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil

	case "signal.target.envs_truncated":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&(*Event)(ctx.Object).Signal.Target.Process)
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

				return int((*Event)(ctx.Object).Signal.Target.Process.FileFields.CTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.filesystem":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Signal.Target.Process.Filesystem
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.gid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Signal.Target.Process.FileFields.GID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.group":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Signal.Target.Process.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "signal.target.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Signal.Target.Process.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "signal.target.file.inode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Signal.Target.Process.FileFields.Inode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.mode":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Signal.Target.Process.FileFields.Mode)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.modification_time":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Signal.Target.Process.FileFields.MTime)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.mount_id":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Signal.Target.Process.FileFields.MountID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Signal.Target.Process.BasenameStr
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).Signal.Target.Process.PathnameStr
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Signal.Target.Process.FileFields))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "signal.target.file.uid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Signal.Target.Process.FileFields.UID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil

	case "signal.target.file.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Signal.Target.Process.FileFields)
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

	case "signal.target.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).Signal.Target.Process.Pid)
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

				return int((*Event)(ctx.Object).Signal.Target.Process.Tid)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Splice.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Splice.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "splice.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Splice.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Splice.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "splice.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Splice.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "splice.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Splice.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Splice.File.FileFields)
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Unlink.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Unlink.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Unlink.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Unlink.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Unlink.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "unlink.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Unlink.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Unlink.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
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

				return (*Event)(ctx.Object).ResolveFileFilesystem(&(*Event)(ctx.Object).Utimes.File)
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

				return (*Event)(ctx.Object).ResolveFileFieldsGroup(&(*Event)(ctx.Object).Utimes.File.FileFields)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.file.in_upper_layer":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {

				return (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&(*Event)(ctx.Object).Utimes.File.FileFields)
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
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFileBasename(&(*Event)(ctx.Object).Utimes.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.file.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {

				return (*Event)(ctx.Object).ResolveFilePath(&(*Event)(ctx.Object).Utimes.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil

	case "utimes.file.rights":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {

				return int((*Event)(ctx.Object).ResolveRights(&(*Event)(ctx.Object).Utimes.File.FileFields))
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

				return (*Event)(ctx.Object).ResolveFileFieldsUser(&(*Event)(ctx.Object).Utimes.File.FileFields)
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

		"chmod.file.path",

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

		"chown.file.path",

		"chown.file.rights",

		"chown.file.uid",

		"chown.file.user",

		"chown.retval",

		"container.id",

		"container.tags",

		"dns.name",

		"dns.qclass",

		"dns.qdcount",

		"dns.qtype",

		"dns.retval",

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

		"exec.file.path",

		"exec.file.rights",

		"exec.file.uid",

		"exec.file.user",

		"exec.fsgid",

		"exec.fsgroup",

		"exec.fsuid",

		"exec.fsuser",

		"exec.gid",

		"exec.group",

		"exec.pid",

		"exec.ppid",

		"exec.tid",

		"exec.tty_name",

		"exec.uid",

		"exec.user",

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

		"link.file.destination.path",

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

		"link.file.path",

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

		"load_module.file.path",

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

		"mkdir.file.path",

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

		"mmap.file.path",

		"mmap.file.rights",

		"mmap.file.uid",

		"mmap.file.user",

		"mmap.flags",

		"mmap.protection",

		"mmap.retval",

		"mprotect.req_protection",

		"mprotect.retval",

		"mprotect.vm_protection",

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

		"open.file.path",

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

		"process.ancestors.file.path",

		"process.ancestors.file.rights",

		"process.ancestors.file.uid",

		"process.ancestors.file.user",

		"process.ancestors.fsgid",

		"process.ancestors.fsgroup",

		"process.ancestors.fsuid",

		"process.ancestors.fsuser",

		"process.ancestors.gid",

		"process.ancestors.group",

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

		"process.file.path",

		"process.file.rights",

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

		"ptrace.tracee.ancestors.file.path",

		"ptrace.tracee.ancestors.file.rights",

		"ptrace.tracee.ancestors.file.uid",

		"ptrace.tracee.ancestors.file.user",

		"ptrace.tracee.ancestors.fsgid",

		"ptrace.tracee.ancestors.fsgroup",

		"ptrace.tracee.ancestors.fsuid",

		"ptrace.tracee.ancestors.fsuser",

		"ptrace.tracee.ancestors.gid",

		"ptrace.tracee.ancestors.group",

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

		"ptrace.tracee.file.path",

		"ptrace.tracee.file.rights",

		"ptrace.tracee.file.uid",

		"ptrace.tracee.file.user",

		"ptrace.tracee.fsgid",

		"ptrace.tracee.fsgroup",

		"ptrace.tracee.fsuid",

		"ptrace.tracee.fsuser",

		"ptrace.tracee.gid",

		"ptrace.tracee.group",

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

		"removexattr.file.path",

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

		"rename.file.destination.path",

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

		"rename.file.path",

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

		"rmdir.file.path",

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

		"setxattr.file.path",

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

		"signal.target.ancestors.file.path",

		"signal.target.ancestors.file.rights",

		"signal.target.ancestors.file.uid",

		"signal.target.ancestors.file.user",

		"signal.target.ancestors.fsgid",

		"signal.target.ancestors.fsgroup",

		"signal.target.ancestors.fsuid",

		"signal.target.ancestors.fsuser",

		"signal.target.ancestors.gid",

		"signal.target.ancestors.group",

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

		"signal.target.file.path",

		"signal.target.file.rights",

		"signal.target.file.uid",

		"signal.target.file.user",

		"signal.target.fsgid",

		"signal.target.fsgroup",

		"signal.target.fsuid",

		"signal.target.fsuser",

		"signal.target.gid",

		"signal.target.group",

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

		"splice.file.path",

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

		"unlink.file.path",

		"unlink.file.rights",

		"unlink.file.uid",

		"unlink.file.user",

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

		"utimes.file.path",

		"utimes.file.rights",

		"utimes.file.uid",

		"utimes.file.user",

		"utimes.retval",
	}
}

func (e *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {

	case "bpf.cmd":

		return int(e.BPF.Cmd), nil

	case "bpf.map.name":

		return e.BPF.Map.Name, nil

	case "bpf.map.type":

		return int(e.BPF.Map.Type), nil

	case "bpf.prog.attach_type":

		return int(e.BPF.Program.AttachType), nil

	case "bpf.prog.helpers":

		result := make([]int, len(e.ResolveHelpers(&e.BPF.Program)))
		for i, v := range e.ResolveHelpers(&e.BPF.Program) {
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

		return e.ResolveFileFilesystem(&e.Chmod.File), nil

	case "chmod.file.gid":

		return int(e.Chmod.File.FileFields.GID), nil

	case "chmod.file.group":

		return e.ResolveFileFieldsGroup(&e.Chmod.File.FileFields), nil

	case "chmod.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Chmod.File.FileFields), nil

	case "chmod.file.inode":

		return int(e.Chmod.File.FileFields.Inode), nil

	case "chmod.file.mode":

		return int(e.Chmod.File.FileFields.Mode), nil

	case "chmod.file.modification_time":

		return int(e.Chmod.File.FileFields.MTime), nil

	case "chmod.file.mount_id":

		return int(e.Chmod.File.FileFields.MountID), nil

	case "chmod.file.name":

		return e.ResolveFileBasename(&e.Chmod.File), nil

	case "chmod.file.path":

		return e.ResolveFilePath(&e.Chmod.File), nil

	case "chmod.file.rights":

		return int(e.ResolveRights(&e.Chmod.File.FileFields)), nil

	case "chmod.file.uid":

		return int(e.Chmod.File.FileFields.UID), nil

	case "chmod.file.user":

		return e.ResolveFileFieldsUser(&e.Chmod.File.FileFields), nil

	case "chmod.retval":

		return int(e.Chmod.SyscallEvent.Retval), nil

	case "chown.file.change_time":

		return int(e.Chown.File.FileFields.CTime), nil

	case "chown.file.destination.gid":

		return int(e.Chown.GID), nil

	case "chown.file.destination.group":

		return e.ResolveChownGID(&e.Chown), nil

	case "chown.file.destination.uid":

		return int(e.Chown.UID), nil

	case "chown.file.destination.user":

		return e.ResolveChownUID(&e.Chown), nil

	case "chown.file.filesystem":

		return e.ResolveFileFilesystem(&e.Chown.File), nil

	case "chown.file.gid":

		return int(e.Chown.File.FileFields.GID), nil

	case "chown.file.group":

		return e.ResolveFileFieldsGroup(&e.Chown.File.FileFields), nil

	case "chown.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Chown.File.FileFields), nil

	case "chown.file.inode":

		return int(e.Chown.File.FileFields.Inode), nil

	case "chown.file.mode":

		return int(e.Chown.File.FileFields.Mode), nil

	case "chown.file.modification_time":

		return int(e.Chown.File.FileFields.MTime), nil

	case "chown.file.mount_id":

		return int(e.Chown.File.FileFields.MountID), nil

	case "chown.file.name":

		return e.ResolveFileBasename(&e.Chown.File), nil

	case "chown.file.path":

		return e.ResolveFilePath(&e.Chown.File), nil

	case "chown.file.rights":

		return int(e.ResolveRights(&e.Chown.File.FileFields)), nil

	case "chown.file.uid":

		return int(e.Chown.File.FileFields.UID), nil

	case "chown.file.user":

		return e.ResolveFileFieldsUser(&e.Chown.File.FileFields), nil

	case "chown.retval":

		return int(e.Chown.SyscallEvent.Retval), nil

	case "container.id":

		return e.ResolveContainerID(&e.ContainerContext), nil

	case "container.tags":

		return e.ResolveContainerTags(&e.ContainerContext), nil

	case "dns.name":

		return e.DNS.Name, nil

	case "dns.qclass":

		return int(e.DNS.QClass), nil

	case "dns.qdcount":

		return int(e.DNS.QDCount), nil

	case "dns.qtype":

		return int(e.DNS.QType), nil

	case "dns.retval":

		return int(e.DNS.SyscallEvent.Retval), nil

	case "exec.args":

		return e.ResolveProcessArgs(&e.Exec.Process), nil

	case "exec.args_flags":

		return e.ResolveProcessArgsFlags(&e.Exec.Process), nil

	case "exec.args_options":

		return e.ResolveProcessArgsOptions(&e.Exec.Process), nil

	case "exec.args_truncated":

		return e.ResolveProcessArgsTruncated(&e.Exec.Process), nil

	case "exec.argv":

		return e.ResolveProcessArgv(&e.Exec.Process), nil

	case "exec.argv0":

		return e.ResolveProcessArgv0(&e.Exec.Process), nil

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

		return int(e.ResolveProcessCreatedAt(&e.Exec.Process)), nil

	case "exec.egid":

		return int(e.Exec.Process.Credentials.EGID), nil

	case "exec.egroup":

		return e.Exec.Process.Credentials.EGroup, nil

	case "exec.envp":

		return e.ResolveProcessEnvp(&e.Exec.Process), nil

	case "exec.envs":

		return e.ResolveProcessEnvs(&e.Exec.Process), nil

	case "exec.envs_truncated":

		return e.ResolveProcessEnvsTruncated(&e.Exec.Process), nil

	case "exec.euid":

		return int(e.Exec.Process.Credentials.EUID), nil

	case "exec.euser":

		return e.Exec.Process.Credentials.EUser, nil

	case "exec.file.change_time":

		return int(e.Exec.Process.FileFields.CTime), nil

	case "exec.file.filesystem":

		return e.Exec.Process.Filesystem, nil

	case "exec.file.gid":

		return int(e.Exec.Process.FileFields.GID), nil

	case "exec.file.group":

		return e.ResolveFileFieldsGroup(&e.Exec.Process.FileFields), nil

	case "exec.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Exec.Process.FileFields), nil

	case "exec.file.inode":

		return int(e.Exec.Process.FileFields.Inode), nil

	case "exec.file.mode":

		return int(e.Exec.Process.FileFields.Mode), nil

	case "exec.file.modification_time":

		return int(e.Exec.Process.FileFields.MTime), nil

	case "exec.file.mount_id":

		return int(e.Exec.Process.FileFields.MountID), nil

	case "exec.file.name":

		return e.Exec.Process.BasenameStr, nil

	case "exec.file.path":

		return e.Exec.Process.PathnameStr, nil

	case "exec.file.rights":

		return int(e.ResolveRights(&e.Exec.Process.FileFields)), nil

	case "exec.file.uid":

		return int(e.Exec.Process.FileFields.UID), nil

	case "exec.file.user":

		return e.ResolveFileFieldsUser(&e.Exec.Process.FileFields), nil

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

	case "exec.pid":

		return int(e.Exec.Process.Pid), nil

	case "exec.ppid":

		return int(e.Exec.Process.PPid), nil

	case "exec.tid":

		return int(e.Exec.Process.Tid), nil

	case "exec.tty_name":

		return e.Exec.Process.TTYName, nil

	case "exec.uid":

		return int(e.Exec.Process.Credentials.UID), nil

	case "exec.user":

		return e.Exec.Process.Credentials.User, nil

	case "link.file.change_time":

		return int(e.Link.Source.FileFields.CTime), nil

	case "link.file.destination.change_time":

		return int(e.Link.Target.FileFields.CTime), nil

	case "link.file.destination.filesystem":

		return e.ResolveFileFilesystem(&e.Link.Target), nil

	case "link.file.destination.gid":

		return int(e.Link.Target.FileFields.GID), nil

	case "link.file.destination.group":

		return e.ResolveFileFieldsGroup(&e.Link.Target.FileFields), nil

	case "link.file.destination.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Link.Target.FileFields), nil

	case "link.file.destination.inode":

		return int(e.Link.Target.FileFields.Inode), nil

	case "link.file.destination.mode":

		return int(e.Link.Target.FileFields.Mode), nil

	case "link.file.destination.modification_time":

		return int(e.Link.Target.FileFields.MTime), nil

	case "link.file.destination.mount_id":

		return int(e.Link.Target.FileFields.MountID), nil

	case "link.file.destination.name":

		return e.ResolveFileBasename(&e.Link.Target), nil

	case "link.file.destination.path":

		return e.ResolveFilePath(&e.Link.Target), nil

	case "link.file.destination.rights":

		return int(e.ResolveRights(&e.Link.Target.FileFields)), nil

	case "link.file.destination.uid":

		return int(e.Link.Target.FileFields.UID), nil

	case "link.file.destination.user":

		return e.ResolveFileFieldsUser(&e.Link.Target.FileFields), nil

	case "link.file.filesystem":

		return e.ResolveFileFilesystem(&e.Link.Source), nil

	case "link.file.gid":

		return int(e.Link.Source.FileFields.GID), nil

	case "link.file.group":

		return e.ResolveFileFieldsGroup(&e.Link.Source.FileFields), nil

	case "link.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Link.Source.FileFields), nil

	case "link.file.inode":

		return int(e.Link.Source.FileFields.Inode), nil

	case "link.file.mode":

		return int(e.Link.Source.FileFields.Mode), nil

	case "link.file.modification_time":

		return int(e.Link.Source.FileFields.MTime), nil

	case "link.file.mount_id":

		return int(e.Link.Source.FileFields.MountID), nil

	case "link.file.name":

		return e.ResolveFileBasename(&e.Link.Source), nil

	case "link.file.path":

		return e.ResolveFilePath(&e.Link.Source), nil

	case "link.file.rights":

		return int(e.ResolveRights(&e.Link.Source.FileFields)), nil

	case "link.file.uid":

		return int(e.Link.Source.FileFields.UID), nil

	case "link.file.user":

		return e.ResolveFileFieldsUser(&e.Link.Source.FileFields), nil

	case "link.retval":

		return int(e.Link.SyscallEvent.Retval), nil

	case "load_module.file.change_time":

		return int(e.LoadModule.File.FileFields.CTime), nil

	case "load_module.file.filesystem":

		return e.ResolveFileFilesystem(&e.LoadModule.File), nil

	case "load_module.file.gid":

		return int(e.LoadModule.File.FileFields.GID), nil

	case "load_module.file.group":

		return e.ResolveFileFieldsGroup(&e.LoadModule.File.FileFields), nil

	case "load_module.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.LoadModule.File.FileFields), nil

	case "load_module.file.inode":

		return int(e.LoadModule.File.FileFields.Inode), nil

	case "load_module.file.mode":

		return int(e.LoadModule.File.FileFields.Mode), nil

	case "load_module.file.modification_time":

		return int(e.LoadModule.File.FileFields.MTime), nil

	case "load_module.file.mount_id":

		return int(e.LoadModule.File.FileFields.MountID), nil

	case "load_module.file.name":

		return e.ResolveFileBasename(&e.LoadModule.File), nil

	case "load_module.file.path":

		return e.ResolveFilePath(&e.LoadModule.File), nil

	case "load_module.file.rights":

		return int(e.ResolveRights(&e.LoadModule.File.FileFields)), nil

	case "load_module.file.uid":

		return int(e.LoadModule.File.FileFields.UID), nil

	case "load_module.file.user":

		return e.ResolveFileFieldsUser(&e.LoadModule.File.FileFields), nil

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

		return e.ResolveFileFilesystem(&e.Mkdir.File), nil

	case "mkdir.file.gid":

		return int(e.Mkdir.File.FileFields.GID), nil

	case "mkdir.file.group":

		return e.ResolveFileFieldsGroup(&e.Mkdir.File.FileFields), nil

	case "mkdir.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Mkdir.File.FileFields), nil

	case "mkdir.file.inode":

		return int(e.Mkdir.File.FileFields.Inode), nil

	case "mkdir.file.mode":

		return int(e.Mkdir.File.FileFields.Mode), nil

	case "mkdir.file.modification_time":

		return int(e.Mkdir.File.FileFields.MTime), nil

	case "mkdir.file.mount_id":

		return int(e.Mkdir.File.FileFields.MountID), nil

	case "mkdir.file.name":

		return e.ResolveFileBasename(&e.Mkdir.File), nil

	case "mkdir.file.path":

		return e.ResolveFilePath(&e.Mkdir.File), nil

	case "mkdir.file.rights":

		return int(e.ResolveRights(&e.Mkdir.File.FileFields)), nil

	case "mkdir.file.uid":

		return int(e.Mkdir.File.FileFields.UID), nil

	case "mkdir.file.user":

		return e.ResolveFileFieldsUser(&e.Mkdir.File.FileFields), nil

	case "mkdir.retval":

		return int(e.Mkdir.SyscallEvent.Retval), nil

	case "mmap.file.change_time":

		return int(e.MMap.File.FileFields.CTime), nil

	case "mmap.file.filesystem":

		return e.ResolveFileFilesystem(&e.MMap.File), nil

	case "mmap.file.gid":

		return int(e.MMap.File.FileFields.GID), nil

	case "mmap.file.group":

		return e.ResolveFileFieldsGroup(&e.MMap.File.FileFields), nil

	case "mmap.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.MMap.File.FileFields), nil

	case "mmap.file.inode":

		return int(e.MMap.File.FileFields.Inode), nil

	case "mmap.file.mode":

		return int(e.MMap.File.FileFields.Mode), nil

	case "mmap.file.modification_time":

		return int(e.MMap.File.FileFields.MTime), nil

	case "mmap.file.mount_id":

		return int(e.MMap.File.FileFields.MountID), nil

	case "mmap.file.name":

		return e.ResolveFileBasename(&e.MMap.File), nil

	case "mmap.file.path":

		return e.ResolveFilePath(&e.MMap.File), nil

	case "mmap.file.rights":

		return int(e.ResolveRights(&e.MMap.File.FileFields)), nil

	case "mmap.file.uid":

		return int(e.MMap.File.FileFields.UID), nil

	case "mmap.file.user":

		return e.ResolveFileFieldsUser(&e.MMap.File.FileFields), nil

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

	case "open.file.change_time":

		return int(e.Open.File.FileFields.CTime), nil

	case "open.file.destination.mode":

		return int(e.Open.Mode), nil

	case "open.file.filesystem":

		return e.ResolveFileFilesystem(&e.Open.File), nil

	case "open.file.gid":

		return int(e.Open.File.FileFields.GID), nil

	case "open.file.group":

		return e.ResolveFileFieldsGroup(&e.Open.File.FileFields), nil

	case "open.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Open.File.FileFields), nil

	case "open.file.inode":

		return int(e.Open.File.FileFields.Inode), nil

	case "open.file.mode":

		return int(e.Open.File.FileFields.Mode), nil

	case "open.file.modification_time":

		return int(e.Open.File.FileFields.MTime), nil

	case "open.file.mount_id":

		return int(e.Open.File.FileFields.MountID), nil

	case "open.file.name":

		return e.ResolveFileBasename(&e.Open.File), nil

	case "open.file.path":

		return e.ResolveFilePath(&e.Open.File), nil

	case "open.file.rights":

		return int(e.ResolveRights(&e.Open.File.FileFields)), nil

	case "open.file.uid":

		return int(e.Open.File.FileFields.UID), nil

	case "open.file.user":

		return e.ResolveFileFieldsUser(&e.Open.File.FileFields), nil

	case "open.flags":

		return int(e.Open.Flags), nil

	case "open.retval":

		return int(e.Open.SyscallEvent.Retval), nil

	case "process.ancestors.args":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgs(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.args_flags":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsFlags(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.args_options":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsOptions(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.args_truncated":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsTruncated(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.argv":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgv(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.argv0":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgv0(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.cap_effective":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.CapEffective)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.cap_permitted":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.CapPermitted)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.comm":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Comm

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.container.id":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.ContainerID

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.cookie":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Cookie)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.created_at":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int((*Event)(ctx.Object).ResolveProcessCreatedAt(&element.Process))

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.egid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.EGID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.egroup":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.EGroup

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.envp":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvp(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.envs":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvs(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.envs_truncated":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.euid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.EUID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.euser":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.EUser

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.change_time":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.CTime)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.filesystem":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Filesystem

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.gid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.GID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.group":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsGroup(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.in_upper_layer":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.inode":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.Inode)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.mode":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.Mode)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.modification_time":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.MTime)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.mount_id":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.MountID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.name":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.BasenameStr

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.path":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.PathnameStr

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.rights":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int((*Event)(ctx.Object).ResolveRights(&element.FileFields))

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.uid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.UID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.file.user":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsUser(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.fsgid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.FSGID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.fsgroup":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.FSGroup

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.fsuid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.FSUID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.fsuser":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.FSUser

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.gid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.GID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.group":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.Group

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.pid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Pid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.ppid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.PPid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.tid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Tid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.tty_name":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.TTYName

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.uid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.UID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.ancestors.user":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.User

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "process.args":

		return e.ResolveProcessArgs(&e.ProcessContext.Process), nil

	case "process.args_flags":

		return e.ResolveProcessArgsFlags(&e.ProcessContext.Process), nil

	case "process.args_options":

		return e.ResolveProcessArgsOptions(&e.ProcessContext.Process), nil

	case "process.args_truncated":

		return e.ResolveProcessArgsTruncated(&e.ProcessContext.Process), nil

	case "process.argv":

		return e.ResolveProcessArgv(&e.ProcessContext.Process), nil

	case "process.argv0":

		return e.ResolveProcessArgv0(&e.ProcessContext.Process), nil

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

		return int(e.ResolveProcessCreatedAt(&e.ProcessContext.Process)), nil

	case "process.egid":

		return int(e.ProcessContext.Process.Credentials.EGID), nil

	case "process.egroup":

		return e.ProcessContext.Process.Credentials.EGroup, nil

	case "process.envp":

		return e.ResolveProcessEnvp(&e.ProcessContext.Process), nil

	case "process.envs":

		return e.ResolveProcessEnvs(&e.ProcessContext.Process), nil

	case "process.envs_truncated":

		return e.ResolveProcessEnvsTruncated(&e.ProcessContext.Process), nil

	case "process.euid":

		return int(e.ProcessContext.Process.Credentials.EUID), nil

	case "process.euser":

		return e.ProcessContext.Process.Credentials.EUser, nil

	case "process.file.change_time":

		return int(e.ProcessContext.Process.FileFields.CTime), nil

	case "process.file.filesystem":

		return e.ProcessContext.Process.Filesystem, nil

	case "process.file.gid":

		return int(e.ProcessContext.Process.FileFields.GID), nil

	case "process.file.group":

		return e.ResolveFileFieldsGroup(&e.ProcessContext.Process.FileFields), nil

	case "process.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.ProcessContext.Process.FileFields), nil

	case "process.file.inode":

		return int(e.ProcessContext.Process.FileFields.Inode), nil

	case "process.file.mode":

		return int(e.ProcessContext.Process.FileFields.Mode), nil

	case "process.file.modification_time":

		return int(e.ProcessContext.Process.FileFields.MTime), nil

	case "process.file.mount_id":

		return int(e.ProcessContext.Process.FileFields.MountID), nil

	case "process.file.name":

		return e.ProcessContext.Process.BasenameStr, nil

	case "process.file.path":

		return e.ProcessContext.Process.PathnameStr, nil

	case "process.file.rights":

		return int(e.ResolveRights(&e.ProcessContext.Process.FileFields)), nil

	case "process.file.uid":

		return int(e.ProcessContext.Process.FileFields.UID), nil

	case "process.file.user":

		return e.ResolveFileFieldsUser(&e.ProcessContext.Process.FileFields), nil

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

	case "process.pid":

		return int(e.ProcessContext.Process.Pid), nil

	case "process.ppid":

		return int(e.ProcessContext.Process.PPid), nil

	case "process.tid":

		return int(e.ProcessContext.Process.Tid), nil

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

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgs(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.args_flags":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsFlags(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.args_options":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsOptions(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.args_truncated":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsTruncated(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.argv":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgv(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.argv0":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgv0(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.cap_effective":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.CapEffective)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.cap_permitted":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.CapPermitted)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.comm":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Comm

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.container.id":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.ContainerID

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.cookie":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Cookie)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.created_at":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int((*Event)(ctx.Object).ResolveProcessCreatedAt(&element.Process))

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.egid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.EGID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.egroup":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.EGroup

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.envp":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvp(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.envs":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvs(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.envs_truncated":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.euid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.EUID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.euser":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.EUser

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.change_time":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.CTime)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.filesystem":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Filesystem

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.gid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.GID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.group":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsGroup(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.in_upper_layer":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.inode":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.Inode)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.mode":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.Mode)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.modification_time":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.MTime)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.mount_id":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.MountID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.name":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.BasenameStr

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.path":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.PathnameStr

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.rights":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int((*Event)(ctx.Object).ResolveRights(&element.FileFields))

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.uid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.UID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.file.user":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsUser(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.fsgid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.FSGID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.fsgroup":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.FSGroup

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.fsuid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.FSUID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.fsuser":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.FSUser

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.gid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.GID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.group":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.Group

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.pid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Pid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.ppid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.PPid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.tid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Tid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.tty_name":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.TTYName

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.uid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.UID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.ancestors.user":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.User

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "ptrace.tracee.args":

		return e.ResolveProcessArgs(&e.PTrace.Tracee.Process), nil

	case "ptrace.tracee.args_flags":

		return e.ResolveProcessArgsFlags(&e.PTrace.Tracee.Process), nil

	case "ptrace.tracee.args_options":

		return e.ResolveProcessArgsOptions(&e.PTrace.Tracee.Process), nil

	case "ptrace.tracee.args_truncated":

		return e.ResolveProcessArgsTruncated(&e.PTrace.Tracee.Process), nil

	case "ptrace.tracee.argv":

		return e.ResolveProcessArgv(&e.PTrace.Tracee.Process), nil

	case "ptrace.tracee.argv0":

		return e.ResolveProcessArgv0(&e.PTrace.Tracee.Process), nil

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

		return int(e.ResolveProcessCreatedAt(&e.PTrace.Tracee.Process)), nil

	case "ptrace.tracee.egid":

		return int(e.PTrace.Tracee.Process.Credentials.EGID), nil

	case "ptrace.tracee.egroup":

		return e.PTrace.Tracee.Process.Credentials.EGroup, nil

	case "ptrace.tracee.envp":

		return e.ResolveProcessEnvp(&e.PTrace.Tracee.Process), nil

	case "ptrace.tracee.envs":

		return e.ResolveProcessEnvs(&e.PTrace.Tracee.Process), nil

	case "ptrace.tracee.envs_truncated":

		return e.ResolveProcessEnvsTruncated(&e.PTrace.Tracee.Process), nil

	case "ptrace.tracee.euid":

		return int(e.PTrace.Tracee.Process.Credentials.EUID), nil

	case "ptrace.tracee.euser":

		return e.PTrace.Tracee.Process.Credentials.EUser, nil

	case "ptrace.tracee.file.change_time":

		return int(e.PTrace.Tracee.Process.FileFields.CTime), nil

	case "ptrace.tracee.file.filesystem":

		return e.PTrace.Tracee.Process.Filesystem, nil

	case "ptrace.tracee.file.gid":

		return int(e.PTrace.Tracee.Process.FileFields.GID), nil

	case "ptrace.tracee.file.group":

		return e.ResolveFileFieldsGroup(&e.PTrace.Tracee.Process.FileFields), nil

	case "ptrace.tracee.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.PTrace.Tracee.Process.FileFields), nil

	case "ptrace.tracee.file.inode":

		return int(e.PTrace.Tracee.Process.FileFields.Inode), nil

	case "ptrace.tracee.file.mode":

		return int(e.PTrace.Tracee.Process.FileFields.Mode), nil

	case "ptrace.tracee.file.modification_time":

		return int(e.PTrace.Tracee.Process.FileFields.MTime), nil

	case "ptrace.tracee.file.mount_id":

		return int(e.PTrace.Tracee.Process.FileFields.MountID), nil

	case "ptrace.tracee.file.name":

		return e.PTrace.Tracee.Process.BasenameStr, nil

	case "ptrace.tracee.file.path":

		return e.PTrace.Tracee.Process.PathnameStr, nil

	case "ptrace.tracee.file.rights":

		return int(e.ResolveRights(&e.PTrace.Tracee.Process.FileFields)), nil

	case "ptrace.tracee.file.uid":

		return int(e.PTrace.Tracee.Process.FileFields.UID), nil

	case "ptrace.tracee.file.user":

		return e.ResolveFileFieldsUser(&e.PTrace.Tracee.Process.FileFields), nil

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

	case "ptrace.tracee.pid":

		return int(e.PTrace.Tracee.Process.Pid), nil

	case "ptrace.tracee.ppid":

		return int(e.PTrace.Tracee.Process.PPid), nil

	case "ptrace.tracee.tid":

		return int(e.PTrace.Tracee.Process.Tid), nil

	case "ptrace.tracee.tty_name":

		return e.PTrace.Tracee.Process.TTYName, nil

	case "ptrace.tracee.uid":

		return int(e.PTrace.Tracee.Process.Credentials.UID), nil

	case "ptrace.tracee.user":

		return e.PTrace.Tracee.Process.Credentials.User, nil

	case "removexattr.file.change_time":

		return int(e.RemoveXAttr.File.FileFields.CTime), nil

	case "removexattr.file.destination.name":

		return e.ResolveXAttrName(&e.RemoveXAttr), nil

	case "removexattr.file.destination.namespace":

		return e.ResolveXAttrNamespace(&e.RemoveXAttr), nil

	case "removexattr.file.filesystem":

		return e.ResolveFileFilesystem(&e.RemoveXAttr.File), nil

	case "removexattr.file.gid":

		return int(e.RemoveXAttr.File.FileFields.GID), nil

	case "removexattr.file.group":

		return e.ResolveFileFieldsGroup(&e.RemoveXAttr.File.FileFields), nil

	case "removexattr.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.RemoveXAttr.File.FileFields), nil

	case "removexattr.file.inode":

		return int(e.RemoveXAttr.File.FileFields.Inode), nil

	case "removexattr.file.mode":

		return int(e.RemoveXAttr.File.FileFields.Mode), nil

	case "removexattr.file.modification_time":

		return int(e.RemoveXAttr.File.FileFields.MTime), nil

	case "removexattr.file.mount_id":

		return int(e.RemoveXAttr.File.FileFields.MountID), nil

	case "removexattr.file.name":

		return e.ResolveFileBasename(&e.RemoveXAttr.File), nil

	case "removexattr.file.path":

		return e.ResolveFilePath(&e.RemoveXAttr.File), nil

	case "removexattr.file.rights":

		return int(e.ResolveRights(&e.RemoveXAttr.File.FileFields)), nil

	case "removexattr.file.uid":

		return int(e.RemoveXAttr.File.FileFields.UID), nil

	case "removexattr.file.user":

		return e.ResolveFileFieldsUser(&e.RemoveXAttr.File.FileFields), nil

	case "removexattr.retval":

		return int(e.RemoveXAttr.SyscallEvent.Retval), nil

	case "rename.file.change_time":

		return int(e.Rename.Old.FileFields.CTime), nil

	case "rename.file.destination.change_time":

		return int(e.Rename.New.FileFields.CTime), nil

	case "rename.file.destination.filesystem":

		return e.ResolveFileFilesystem(&e.Rename.New), nil

	case "rename.file.destination.gid":

		return int(e.Rename.New.FileFields.GID), nil

	case "rename.file.destination.group":

		return e.ResolveFileFieldsGroup(&e.Rename.New.FileFields), nil

	case "rename.file.destination.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Rename.New.FileFields), nil

	case "rename.file.destination.inode":

		return int(e.Rename.New.FileFields.Inode), nil

	case "rename.file.destination.mode":

		return int(e.Rename.New.FileFields.Mode), nil

	case "rename.file.destination.modification_time":

		return int(e.Rename.New.FileFields.MTime), nil

	case "rename.file.destination.mount_id":

		return int(e.Rename.New.FileFields.MountID), nil

	case "rename.file.destination.name":

		return e.ResolveFileBasename(&e.Rename.New), nil

	case "rename.file.destination.path":

		return e.ResolveFilePath(&e.Rename.New), nil

	case "rename.file.destination.rights":

		return int(e.ResolveRights(&e.Rename.New.FileFields)), nil

	case "rename.file.destination.uid":

		return int(e.Rename.New.FileFields.UID), nil

	case "rename.file.destination.user":

		return e.ResolveFileFieldsUser(&e.Rename.New.FileFields), nil

	case "rename.file.filesystem":

		return e.ResolveFileFilesystem(&e.Rename.Old), nil

	case "rename.file.gid":

		return int(e.Rename.Old.FileFields.GID), nil

	case "rename.file.group":

		return e.ResolveFileFieldsGroup(&e.Rename.Old.FileFields), nil

	case "rename.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Rename.Old.FileFields), nil

	case "rename.file.inode":

		return int(e.Rename.Old.FileFields.Inode), nil

	case "rename.file.mode":

		return int(e.Rename.Old.FileFields.Mode), nil

	case "rename.file.modification_time":

		return int(e.Rename.Old.FileFields.MTime), nil

	case "rename.file.mount_id":

		return int(e.Rename.Old.FileFields.MountID), nil

	case "rename.file.name":

		return e.ResolveFileBasename(&e.Rename.Old), nil

	case "rename.file.path":

		return e.ResolveFilePath(&e.Rename.Old), nil

	case "rename.file.rights":

		return int(e.ResolveRights(&e.Rename.Old.FileFields)), nil

	case "rename.file.uid":

		return int(e.Rename.Old.FileFields.UID), nil

	case "rename.file.user":

		return e.ResolveFileFieldsUser(&e.Rename.Old.FileFields), nil

	case "rename.retval":

		return int(e.Rename.SyscallEvent.Retval), nil

	case "rmdir.file.change_time":

		return int(e.Rmdir.File.FileFields.CTime), nil

	case "rmdir.file.filesystem":

		return e.ResolveFileFilesystem(&e.Rmdir.File), nil

	case "rmdir.file.gid":

		return int(e.Rmdir.File.FileFields.GID), nil

	case "rmdir.file.group":

		return e.ResolveFileFieldsGroup(&e.Rmdir.File.FileFields), nil

	case "rmdir.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Rmdir.File.FileFields), nil

	case "rmdir.file.inode":

		return int(e.Rmdir.File.FileFields.Inode), nil

	case "rmdir.file.mode":

		return int(e.Rmdir.File.FileFields.Mode), nil

	case "rmdir.file.modification_time":

		return int(e.Rmdir.File.FileFields.MTime), nil

	case "rmdir.file.mount_id":

		return int(e.Rmdir.File.FileFields.MountID), nil

	case "rmdir.file.name":

		return e.ResolveFileBasename(&e.Rmdir.File), nil

	case "rmdir.file.path":

		return e.ResolveFilePath(&e.Rmdir.File), nil

	case "rmdir.file.rights":

		return int(e.ResolveRights(&e.Rmdir.File.FileFields)), nil

	case "rmdir.file.uid":

		return int(e.Rmdir.File.FileFields.UID), nil

	case "rmdir.file.user":

		return e.ResolveFileFieldsUser(&e.Rmdir.File.FileFields), nil

	case "rmdir.retval":

		return int(e.Rmdir.SyscallEvent.Retval), nil

	case "selinux.bool.name":

		return e.ResolveSELinuxBoolName(&e.SELinux), nil

	case "selinux.bool.state":

		return e.SELinux.BoolChangeValue, nil

	case "selinux.bool_commit.state":

		return e.SELinux.BoolCommitValue, nil

	case "selinux.enforce.status":

		return e.SELinux.EnforceStatus, nil

	case "setgid.egid":

		return int(e.SetGID.EGID), nil

	case "setgid.egroup":

		return e.ResolveSetgidEGroup(&e.SetGID), nil

	case "setgid.fsgid":

		return int(e.SetGID.FSGID), nil

	case "setgid.fsgroup":

		return e.ResolveSetgidFSGroup(&e.SetGID), nil

	case "setgid.gid":

		return int(e.SetGID.GID), nil

	case "setgid.group":

		return e.ResolveSetgidGroup(&e.SetGID), nil

	case "setuid.euid":

		return int(e.SetUID.EUID), nil

	case "setuid.euser":

		return e.ResolveSetuidEUser(&e.SetUID), nil

	case "setuid.fsuid":

		return int(e.SetUID.FSUID), nil

	case "setuid.fsuser":

		return e.ResolveSetuidFSUser(&e.SetUID), nil

	case "setuid.uid":

		return int(e.SetUID.UID), nil

	case "setuid.user":

		return e.ResolveSetuidUser(&e.SetUID), nil

	case "setxattr.file.change_time":

		return int(e.SetXAttr.File.FileFields.CTime), nil

	case "setxattr.file.destination.name":

		return e.ResolveXAttrName(&e.SetXAttr), nil

	case "setxattr.file.destination.namespace":

		return e.ResolveXAttrNamespace(&e.SetXAttr), nil

	case "setxattr.file.filesystem":

		return e.ResolveFileFilesystem(&e.SetXAttr.File), nil

	case "setxattr.file.gid":

		return int(e.SetXAttr.File.FileFields.GID), nil

	case "setxattr.file.group":

		return e.ResolveFileFieldsGroup(&e.SetXAttr.File.FileFields), nil

	case "setxattr.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.SetXAttr.File.FileFields), nil

	case "setxattr.file.inode":

		return int(e.SetXAttr.File.FileFields.Inode), nil

	case "setxattr.file.mode":

		return int(e.SetXAttr.File.FileFields.Mode), nil

	case "setxattr.file.modification_time":

		return int(e.SetXAttr.File.FileFields.MTime), nil

	case "setxattr.file.mount_id":

		return int(e.SetXAttr.File.FileFields.MountID), nil

	case "setxattr.file.name":

		return e.ResolveFileBasename(&e.SetXAttr.File), nil

	case "setxattr.file.path":

		return e.ResolveFilePath(&e.SetXAttr.File), nil

	case "setxattr.file.rights":

		return int(e.ResolveRights(&e.SetXAttr.File.FileFields)), nil

	case "setxattr.file.uid":

		return int(e.SetXAttr.File.FileFields.UID), nil

	case "setxattr.file.user":

		return e.ResolveFileFieldsUser(&e.SetXAttr.File.FileFields), nil

	case "setxattr.retval":

		return int(e.SetXAttr.SyscallEvent.Retval), nil

	case "signal.pid":

		return int(e.Signal.PID), nil

	case "signal.retval":

		return int(e.Signal.SyscallEvent.Retval), nil

	case "signal.target.ancestors.args":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgs(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.args_flags":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsFlags(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.args_options":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsOptions(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.args_truncated":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgsTruncated(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.argv":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgv(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.argv0":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessArgv0(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.cap_effective":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.CapEffective)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.cap_permitted":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.CapPermitted)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.comm":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Comm

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.container.id":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.ContainerID

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.cookie":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Cookie)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.created_at":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int((*Event)(ctx.Object).ResolveProcessCreatedAt(&element.Process))

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.egid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.EGID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.egroup":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.EGroup

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.envp":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvp(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.envs":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvs(&element.Process)

			values = append(values, result...)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.envs_truncated":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveProcessEnvsTruncated(&element.Process)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.euid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.EUID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.euser":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.EUser

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.change_time":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.CTime)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.filesystem":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Filesystem

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.gid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.GID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.group":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsGroup(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.in_upper_layer":

		var values []bool

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsInUpperLayer(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.inode":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.Inode)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.mode":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.Mode)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.modification_time":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.MTime)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.mount_id":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.MountID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.name":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.BasenameStr

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.path":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.PathnameStr

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.rights":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int((*Event)(ctx.Object).ResolveRights(&element.FileFields))

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.uid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.FileFields.UID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.file.user":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := (*Event)(ctx.Object).ResolveFileFieldsUser(&element.FileFields)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.fsgid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.FSGID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.fsgroup":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.FSGroup

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.fsuid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.FSUID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.fsuser":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.FSUser

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.gid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.GID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.group":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.Group

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.pid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Pid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.ppid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.PPid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.tid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Tid)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.tty_name":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.TTYName

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.uid":

		var values []int

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := int(element.ProcessContext.Process.Credentials.UID)

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.ancestors.user":

		var values []string

		ctx := eval.NewContext(unsafe.Pointer(e))

		iterator := &model.ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)

		for ptr != nil {

			element := (*model.ProcessCacheEntry)(ptr)

			result := element.ProcessContext.Process.Credentials.User

			values = append(values, result)

			ptr = iterator.Next()
		}

		return values, nil

	case "signal.target.args":

		return e.ResolveProcessArgs(&e.Signal.Target.Process), nil

	case "signal.target.args_flags":

		return e.ResolveProcessArgsFlags(&e.Signal.Target.Process), nil

	case "signal.target.args_options":

		return e.ResolveProcessArgsOptions(&e.Signal.Target.Process), nil

	case "signal.target.args_truncated":

		return e.ResolveProcessArgsTruncated(&e.Signal.Target.Process), nil

	case "signal.target.argv":

		return e.ResolveProcessArgv(&e.Signal.Target.Process), nil

	case "signal.target.argv0":

		return e.ResolveProcessArgv0(&e.Signal.Target.Process), nil

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

		return int(e.ResolveProcessCreatedAt(&e.Signal.Target.Process)), nil

	case "signal.target.egid":

		return int(e.Signal.Target.Process.Credentials.EGID), nil

	case "signal.target.egroup":

		return e.Signal.Target.Process.Credentials.EGroup, nil

	case "signal.target.envp":

		return e.ResolveProcessEnvp(&e.Signal.Target.Process), nil

	case "signal.target.envs":

		return e.ResolveProcessEnvs(&e.Signal.Target.Process), nil

	case "signal.target.envs_truncated":

		return e.ResolveProcessEnvsTruncated(&e.Signal.Target.Process), nil

	case "signal.target.euid":

		return int(e.Signal.Target.Process.Credentials.EUID), nil

	case "signal.target.euser":

		return e.Signal.Target.Process.Credentials.EUser, nil

	case "signal.target.file.change_time":

		return int(e.Signal.Target.Process.FileFields.CTime), nil

	case "signal.target.file.filesystem":

		return e.Signal.Target.Process.Filesystem, nil

	case "signal.target.file.gid":

		return int(e.Signal.Target.Process.FileFields.GID), nil

	case "signal.target.file.group":

		return e.ResolveFileFieldsGroup(&e.Signal.Target.Process.FileFields), nil

	case "signal.target.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Signal.Target.Process.FileFields), nil

	case "signal.target.file.inode":

		return int(e.Signal.Target.Process.FileFields.Inode), nil

	case "signal.target.file.mode":

		return int(e.Signal.Target.Process.FileFields.Mode), nil

	case "signal.target.file.modification_time":

		return int(e.Signal.Target.Process.FileFields.MTime), nil

	case "signal.target.file.mount_id":

		return int(e.Signal.Target.Process.FileFields.MountID), nil

	case "signal.target.file.name":

		return e.Signal.Target.Process.BasenameStr, nil

	case "signal.target.file.path":

		return e.Signal.Target.Process.PathnameStr, nil

	case "signal.target.file.rights":

		return int(e.ResolveRights(&e.Signal.Target.Process.FileFields)), nil

	case "signal.target.file.uid":

		return int(e.Signal.Target.Process.FileFields.UID), nil

	case "signal.target.file.user":

		return e.ResolveFileFieldsUser(&e.Signal.Target.Process.FileFields), nil

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

	case "signal.target.pid":

		return int(e.Signal.Target.Process.Pid), nil

	case "signal.target.ppid":

		return int(e.Signal.Target.Process.PPid), nil

	case "signal.target.tid":

		return int(e.Signal.Target.Process.Tid), nil

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

		return e.ResolveFileFilesystem(&e.Splice.File), nil

	case "splice.file.gid":

		return int(e.Splice.File.FileFields.GID), nil

	case "splice.file.group":

		return e.ResolveFileFieldsGroup(&e.Splice.File.FileFields), nil

	case "splice.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Splice.File.FileFields), nil

	case "splice.file.inode":

		return int(e.Splice.File.FileFields.Inode), nil

	case "splice.file.mode":

		return int(e.Splice.File.FileFields.Mode), nil

	case "splice.file.modification_time":

		return int(e.Splice.File.FileFields.MTime), nil

	case "splice.file.mount_id":

		return int(e.Splice.File.FileFields.MountID), nil

	case "splice.file.name":

		return e.ResolveFileBasename(&e.Splice.File), nil

	case "splice.file.path":

		return e.ResolveFilePath(&e.Splice.File), nil

	case "splice.file.rights":

		return int(e.ResolveRights(&e.Splice.File.FileFields)), nil

	case "splice.file.uid":

		return int(e.Splice.File.FileFields.UID), nil

	case "splice.file.user":

		return e.ResolveFileFieldsUser(&e.Splice.File.FileFields), nil

	case "splice.pipe_entry_flag":

		return int(e.Splice.PipeEntryFlag), nil

	case "splice.pipe_exit_flag":

		return int(e.Splice.PipeExitFlag), nil

	case "splice.retval":

		return int(e.Splice.SyscallEvent.Retval), nil

	case "unlink.file.change_time":

		return int(e.Unlink.File.FileFields.CTime), nil

	case "unlink.file.filesystem":

		return e.ResolveFileFilesystem(&e.Unlink.File), nil

	case "unlink.file.gid":

		return int(e.Unlink.File.FileFields.GID), nil

	case "unlink.file.group":

		return e.ResolveFileFieldsGroup(&e.Unlink.File.FileFields), nil

	case "unlink.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Unlink.File.FileFields), nil

	case "unlink.file.inode":

		return int(e.Unlink.File.FileFields.Inode), nil

	case "unlink.file.mode":

		return int(e.Unlink.File.FileFields.Mode), nil

	case "unlink.file.modification_time":

		return int(e.Unlink.File.FileFields.MTime), nil

	case "unlink.file.mount_id":

		return int(e.Unlink.File.FileFields.MountID), nil

	case "unlink.file.name":

		return e.ResolveFileBasename(&e.Unlink.File), nil

	case "unlink.file.path":

		return e.ResolveFilePath(&e.Unlink.File), nil

	case "unlink.file.rights":

		return int(e.ResolveRights(&e.Unlink.File.FileFields)), nil

	case "unlink.file.uid":

		return int(e.Unlink.File.FileFields.UID), nil

	case "unlink.file.user":

		return e.ResolveFileFieldsUser(&e.Unlink.File.FileFields), nil

	case "unlink.retval":

		return int(e.Unlink.SyscallEvent.Retval), nil

	case "unload_module.name":

		return e.UnloadModule.Name, nil

	case "unload_module.retval":

		return int(e.UnloadModule.SyscallEvent.Retval), nil

	case "utimes.file.change_time":

		return int(e.Utimes.File.FileFields.CTime), nil

	case "utimes.file.filesystem":

		return e.ResolveFileFilesystem(&e.Utimes.File), nil

	case "utimes.file.gid":

		return int(e.Utimes.File.FileFields.GID), nil

	case "utimes.file.group":

		return e.ResolveFileFieldsGroup(&e.Utimes.File.FileFields), nil

	case "utimes.file.in_upper_layer":

		return e.ResolveFileFieldsInUpperLayer(&e.Utimes.File.FileFields), nil

	case "utimes.file.inode":

		return int(e.Utimes.File.FileFields.Inode), nil

	case "utimes.file.mode":

		return int(e.Utimes.File.FileFields.Mode), nil

	case "utimes.file.modification_time":

		return int(e.Utimes.File.FileFields.MTime), nil

	case "utimes.file.mount_id":

		return int(e.Utimes.File.FileFields.MountID), nil

	case "utimes.file.name":

		return e.ResolveFileBasename(&e.Utimes.File), nil

	case "utimes.file.path":

		return e.ResolveFilePath(&e.Utimes.File), nil

	case "utimes.file.rights":

		return int(e.ResolveRights(&e.Utimes.File.FileFields)), nil

	case "utimes.file.uid":

		return int(e.Utimes.File.FileFields.UID), nil

	case "utimes.file.user":

		return e.ResolveFileFieldsUser(&e.Utimes.File.FileFields), nil

	case "utimes.retval":

		return int(e.Utimes.SyscallEvent.Retval), nil

	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {

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

	case "chmod.file.path":
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

	case "chown.file.path":
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

	case "dns.name":
		return "dns", nil

	case "dns.qclass":
		return "dns", nil

	case "dns.qdcount":
		return "dns", nil

	case "dns.qtype":
		return "dns", nil

	case "dns.retval":
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

	case "exec.file.path":
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

	case "link.file.destination.path":
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

	case "link.file.path":
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

	case "load_module.file.path":
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

	case "mkdir.file.path":
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

	case "mmap.file.path":
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

	case "open.file.path":
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

	case "process.ancestors.file.path":
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

	case "process.file.path":
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

	case "ptrace.tracee.ancestors.file.path":
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

	case "ptrace.tracee.file.path":
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

	case "removexattr.file.path":
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

	case "rename.file.destination.path":
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

	case "rename.file.path":
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

	case "rmdir.file.path":
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

	case "setxattr.file.path":
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

	case "signal.target.ancestors.file.path":
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

	case "signal.target.file.path":
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

	case "splice.file.path":
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

	case "unlink.file.path":
		return "unlink", nil

	case "unlink.file.rights":
		return "unlink", nil

	case "unlink.file.uid":
		return "unlink", nil

	case "unlink.file.user":
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

	case "utimes.file.path":
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

	case "chmod.file.path":

		return reflect.String, nil

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

	case "chown.file.path":

		return reflect.String, nil

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

	case "dns.name":

		return reflect.String, nil

	case "dns.qclass":

		return reflect.Int, nil

	case "dns.qdcount":

		return reflect.Int, nil

	case "dns.qtype":

		return reflect.Int, nil

	case "dns.retval":

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

	case "exec.file.path":

		return reflect.String, nil

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

	case "link.file.destination.path":

		return reflect.String, nil

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

	case "link.file.path":

		return reflect.String, nil

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

	case "load_module.file.path":

		return reflect.String, nil

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

	case "mkdir.file.path":

		return reflect.String, nil

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

	case "mmap.file.path":

		return reflect.String, nil

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

	case "open.file.path":

		return reflect.String, nil

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

	case "process.ancestors.file.path":

		return reflect.String, nil

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

	case "process.file.path":

		return reflect.String, nil

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

	case "ptrace.tracee.ancestors.file.path":

		return reflect.String, nil

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

	case "ptrace.tracee.file.path":

		return reflect.String, nil

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

	case "removexattr.file.path":

		return reflect.String, nil

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

	case "rename.file.destination.path":

		return reflect.String, nil

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

	case "rename.file.path":

		return reflect.String, nil

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

	case "rmdir.file.path":

		return reflect.String, nil

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

	case "setxattr.file.path":

		return reflect.String, nil

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

	case "signal.target.ancestors.file.path":

		return reflect.String, nil

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

	case "signal.target.file.path":

		return reflect.String, nil

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

	case "splice.file.path":

		return reflect.String, nil

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

	case "unlink.file.path":

		return reflect.String, nil

	case "unlink.file.rights":

		return reflect.Int, nil

	case "unlink.file.uid":

		return reflect.Int, nil

	case "unlink.file.user":

		return reflect.String, nil

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

	case "utimes.file.path":

		return reflect.String, nil

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

	case "bpf.cmd":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Cmd"}
		}
		e.BPF.Cmd = uint32(v)

		return nil

	case "bpf.map.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Map.Name"}
		}
		e.BPF.Map.Name = str

		return nil

	case "bpf.map.type":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Map.Type"}
		}
		e.BPF.Map.Type = uint32(v)

		return nil

	case "bpf.prog.attach_type":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.AttachType"}
		}
		e.BPF.Program.AttachType = uint32(v)

		return nil

	case "bpf.prog.helpers":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.Helpers"}
		}
		e.BPF.Program.Helpers = append(e.BPF.Program.Helpers, uint32(v))

		return nil

	case "bpf.prog.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.Name"}
		}
		e.BPF.Program.Name = str

		return nil

	case "bpf.prog.tag":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.Tag"}
		}
		e.BPF.Program.Tag = str

		return nil

	case "bpf.prog.type":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.Program.Type"}
		}
		e.BPF.Program.Type = uint32(v)

		return nil

	case "bpf.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BPF.SyscallEvent.Retval"}
		}
		e.BPF.SyscallEvent.Retval = int64(v)

		return nil

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

	case "chmod.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.CTime"}
		}
		e.Chmod.File.FileFields.CTime = uint64(v)

		return nil

	case "chmod.file.destination.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Mode"}
		}
		e.Chmod.Mode = uint32(v)

		return nil

	case "chmod.file.destination.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Mode"}
		}
		e.Chmod.Mode = uint32(v)

		return nil

	case "chmod.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.Filesytem"}
		}
		e.Chmod.File.Filesytem = str

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

	case "chmod.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.MTime"}
		}
		e.Chmod.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.BasenameStr"}
		}
		e.Chmod.File.BasenameStr = str

		return nil

	case "chmod.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.PathnameStr"}
		}
		e.Chmod.File.PathnameStr = str

		return nil

	case "chmod.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.Mode"}
		}
		e.Chmod.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.File.FileFields.User"}
		}
		e.Chmod.File.FileFields.User = str

		return nil

	case "chmod.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.SyscallEvent.Retval"}
		}
		e.Chmod.SyscallEvent.Retval = int64(v)

		return nil

	case "chown.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.CTime"}
		}
		e.Chown.File.FileFields.CTime = uint64(v)

		return nil

	case "chown.file.destination.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.GID"}
		}
		e.Chown.GID = int64(v)

		return nil

	case "chown.file.destination.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.Group"}
		}
		e.Chown.Group = str

		return nil

	case "chown.file.destination.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.UID"}
		}
		e.Chown.UID = int64(v)

		return nil

	case "chown.file.destination.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.User"}
		}
		e.Chown.User = str

		return nil

	case "chown.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.Filesytem"}
		}
		e.Chown.File.Filesytem = str

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

	case "chown.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.MTime"}
		}
		e.Chown.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.BasenameStr"}
		}
		e.Chown.File.BasenameStr = str

		return nil

	case "chown.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.PathnameStr"}
		}
		e.Chown.File.PathnameStr = str

		return nil

	case "chown.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.Mode"}
		}
		e.Chown.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.File.FileFields.User"}
		}
		e.Chown.File.FileFields.User = str

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ContainerContext.ID"}
		}
		e.ContainerContext.ID = str

		return nil

	case "container.tags":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ContainerContext.Tags"}
		}
		e.ContainerContext.Tags = append(e.ContainerContext.Tags, str)

		return nil

	case "dns.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.Name"}
		}
		e.DNS.Name = str

		return nil

	case "dns.qclass":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.QClass"}
		}
		e.DNS.QClass = uint16(v)

		return nil

	case "dns.qdcount":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.QDCount"}
		}
		e.DNS.QDCount = uint16(v)

		return nil

	case "dns.qtype":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.QType"}
		}
		e.DNS.QType = uint16(v)

		return nil

	case "dns.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DNS.SyscallEvent.Retval"}
		}
		e.DNS.SyscallEvent.Retval = int64(v)

		return nil

	case "exec.args":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Args"}
		}
		e.Exec.Process.Args = str

		return nil

	case "exec.args_flags":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		e.Exec.Process.Argv = append(e.Exec.Process.Argv, str)

		return nil

	case "exec.args_options":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		e.Exec.Process.Argv = append(e.Exec.Process.Argv, str)

		return nil

	case "exec.args_truncated":

		var ok bool
		if e.Exec.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.ArgsTruncated"}
		}
		return nil

	case "exec.argv":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv"}
		}
		e.Exec.Process.Argv = append(e.Exec.Process.Argv, str)

		return nil

	case "exec.argv0":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Argv0"}
		}
		e.Exec.Process.Argv0 = str

		return nil

	case "exec.cap_effective":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.CapEffective"}
		}
		e.Exec.Process.Credentials.CapEffective = uint64(v)

		return nil

	case "exec.cap_permitted":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.CapPermitted"}
		}
		e.Exec.Process.Credentials.CapPermitted = uint64(v)

		return nil

	case "exec.comm":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Comm"}
		}
		e.Exec.Process.Comm = str

		return nil

	case "exec.container.id":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.ContainerID"}
		}
		e.Exec.Process.ContainerID = str

		return nil

	case "exec.cookie":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Cookie"}
		}
		e.Exec.Process.Cookie = uint32(v)

		return nil

	case "exec.created_at":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.CreatedAt"}
		}
		e.Exec.Process.CreatedAt = uint64(v)

		return nil

	case "exec.egid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EGID"}
		}
		e.Exec.Process.Credentials.EGID = uint32(v)

		return nil

	case "exec.egroup":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EGroup"}
		}
		e.Exec.Process.Credentials.EGroup = str

		return nil

	case "exec.envp":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Envp"}
		}
		e.Exec.Process.Envp = append(e.Exec.Process.Envp, str)

		return nil

	case "exec.envs":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Envs"}
		}
		e.Exec.Process.Envs = append(e.Exec.Process.Envs, str)

		return nil

	case "exec.envs_truncated":

		var ok bool
		if e.Exec.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.EnvsTruncated"}
		}
		return nil

	case "exec.euid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EUID"}
		}
		e.Exec.Process.Credentials.EUID = uint32(v)

		return nil

	case "exec.euser":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.EUser"}
		}
		e.Exec.Process.Credentials.EUser = str

		return nil

	case "exec.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.CTime"}
		}
		e.Exec.Process.FileFields.CTime = uint64(v)

		return nil

	case "exec.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Filesystem"}
		}
		e.Exec.Process.Filesystem = str

		return nil

	case "exec.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.GID"}
		}
		e.Exec.Process.FileFields.GID = uint32(v)

		return nil

	case "exec.file.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.Group"}
		}
		e.Exec.Process.FileFields.Group = str

		return nil

	case "exec.file.in_upper_layer":

		var ok bool
		if e.Exec.Process.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.InUpperLayer"}
		}
		return nil

	case "exec.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.Inode"}
		}
		e.Exec.Process.FileFields.Inode = uint64(v)

		return nil

	case "exec.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.Mode"}
		}
		e.Exec.Process.FileFields.Mode = uint16(v)

		return nil

	case "exec.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.MTime"}
		}
		e.Exec.Process.FileFields.MTime = uint64(v)

		return nil

	case "exec.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.MountID"}
		}
		e.Exec.Process.FileFields.MountID = uint32(v)

		return nil

	case "exec.file.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.BasenameStr"}
		}
		e.Exec.Process.BasenameStr = str

		return nil

	case "exec.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PathnameStr"}
		}
		e.Exec.Process.PathnameStr = str

		return nil

	case "exec.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.Mode"}
		}
		e.Exec.Process.FileFields.Mode = uint16(v)

		return nil

	case "exec.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.UID"}
		}
		e.Exec.Process.FileFields.UID = uint32(v)

		return nil

	case "exec.file.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileFields.User"}
		}
		e.Exec.Process.FileFields.User = str

		return nil

	case "exec.fsgid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSGID"}
		}
		e.Exec.Process.Credentials.FSGID = uint32(v)

		return nil

	case "exec.fsgroup":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSGroup"}
		}
		e.Exec.Process.Credentials.FSGroup = str

		return nil

	case "exec.fsuid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSUID"}
		}
		e.Exec.Process.Credentials.FSUID = uint32(v)

		return nil

	case "exec.fsuser":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.FSUser"}
		}
		e.Exec.Process.Credentials.FSUser = str

		return nil

	case "exec.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.GID"}
		}
		e.Exec.Process.Credentials.GID = uint32(v)

		return nil

	case "exec.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.Group"}
		}
		e.Exec.Process.Credentials.Group = str

		return nil

	case "exec.pid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Pid"}
		}
		e.Exec.Process.Pid = uint32(v)

		return nil

	case "exec.ppid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PPid"}
		}
		e.Exec.Process.PPid = uint32(v)

		return nil

	case "exec.tid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Tid"}
		}
		e.Exec.Process.Tid = uint32(v)

		return nil

	case "exec.tty_name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.TTYName"}
		}
		e.Exec.Process.TTYName = str

		return nil

	case "exec.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.UID"}
		}
		e.Exec.Process.Credentials.UID = uint32(v)

		return nil

	case "exec.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Credentials.User"}
		}
		e.Exec.Process.Credentials.User = str

		return nil

	case "link.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.CTime"}
		}
		e.Link.Source.FileFields.CTime = uint64(v)

		return nil

	case "link.file.destination.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.CTime"}
		}
		e.Link.Target.FileFields.CTime = uint64(v)

		return nil

	case "link.file.destination.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.Filesytem"}
		}
		e.Link.Target.Filesytem = str

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

	case "link.file.destination.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.MTime"}
		}
		e.Link.Target.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.BasenameStr"}
		}
		e.Link.Target.BasenameStr = str

		return nil

	case "link.file.destination.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.PathnameStr"}
		}
		e.Link.Target.PathnameStr = str

		return nil

	case "link.file.destination.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.Mode"}
		}
		e.Link.Target.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.FileFields.User"}
		}
		e.Link.Target.FileFields.User = str

		return nil

	case "link.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.Filesytem"}
		}
		e.Link.Source.Filesytem = str

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

	case "link.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.MTime"}
		}
		e.Link.Source.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.BasenameStr"}
		}
		e.Link.Source.BasenameStr = str

		return nil

	case "link.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.PathnameStr"}
		}
		e.Link.Source.PathnameStr = str

		return nil

	case "link.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.Mode"}
		}
		e.Link.Source.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.FileFields.User"}
		}
		e.Link.Source.FileFields.User = str

		return nil

	case "link.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.SyscallEvent.Retval"}
		}
		e.Link.SyscallEvent.Retval = int64(v)

		return nil

	case "load_module.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.CTime"}
		}
		e.LoadModule.File.FileFields.CTime = uint64(v)

		return nil

	case "load_module.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.Filesytem"}
		}
		e.LoadModule.File.Filesytem = str

		return nil

	case "load_module.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.GID"}
		}
		e.LoadModule.File.FileFields.GID = uint32(v)

		return nil

	case "load_module.file.group":

		var ok bool
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

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.Inode"}
		}
		e.LoadModule.File.FileFields.Inode = uint64(v)

		return nil

	case "load_module.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.Mode"}
		}
		e.LoadModule.File.FileFields.Mode = uint16(v)

		return nil

	case "load_module.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.MTime"}
		}
		e.LoadModule.File.FileFields.MTime = uint64(v)

		return nil

	case "load_module.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.MountID"}
		}
		e.LoadModule.File.FileFields.MountID = uint32(v)

		return nil

	case "load_module.file.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.BasenameStr"}
		}
		e.LoadModule.File.BasenameStr = str

		return nil

	case "load_module.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.PathnameStr"}
		}
		e.LoadModule.File.PathnameStr = str

		return nil

	case "load_module.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.Mode"}
		}
		e.LoadModule.File.FileFields.Mode = uint16(v)

		return nil

	case "load_module.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.File.FileFields.UID"}
		}
		e.LoadModule.File.FileFields.UID = uint32(v)

		return nil

	case "load_module.file.user":

		var ok bool
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

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.Name"}
		}
		e.LoadModule.Name = str

		return nil

	case "load_module.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "LoadModule.SyscallEvent.Retval"}
		}
		e.LoadModule.SyscallEvent.Retval = int64(v)

		return nil

	case "mkdir.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.CTime"}
		}
		e.Mkdir.File.FileFields.CTime = uint64(v)

		return nil

	case "mkdir.file.destination.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Mode"}
		}
		e.Mkdir.Mode = uint32(v)

		return nil

	case "mkdir.file.destination.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Mode"}
		}
		e.Mkdir.Mode = uint32(v)

		return nil

	case "mkdir.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.Filesytem"}
		}
		e.Mkdir.File.Filesytem = str

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

	case "mkdir.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.MTime"}
		}
		e.Mkdir.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.BasenameStr"}
		}
		e.Mkdir.File.BasenameStr = str

		return nil

	case "mkdir.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.PathnameStr"}
		}
		e.Mkdir.File.PathnameStr = str

		return nil

	case "mkdir.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.Mode"}
		}
		e.Mkdir.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.File.FileFields.User"}
		}
		e.Mkdir.File.FileFields.User = str

		return nil

	case "mkdir.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.SyscallEvent.Retval"}
		}
		e.Mkdir.SyscallEvent.Retval = int64(v)

		return nil

	case "mmap.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.CTime"}
		}
		e.MMap.File.FileFields.CTime = uint64(v)

		return nil

	case "mmap.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.Filesytem"}
		}
		e.MMap.File.Filesytem = str

		return nil

	case "mmap.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.GID"}
		}
		e.MMap.File.FileFields.GID = uint32(v)

		return nil

	case "mmap.file.group":

		var ok bool
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

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.Inode"}
		}
		e.MMap.File.FileFields.Inode = uint64(v)

		return nil

	case "mmap.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.Mode"}
		}
		e.MMap.File.FileFields.Mode = uint16(v)

		return nil

	case "mmap.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.MTime"}
		}
		e.MMap.File.FileFields.MTime = uint64(v)

		return nil

	case "mmap.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.MountID"}
		}
		e.MMap.File.FileFields.MountID = uint32(v)

		return nil

	case "mmap.file.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.BasenameStr"}
		}
		e.MMap.File.BasenameStr = str

		return nil

	case "mmap.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.PathnameStr"}
		}
		e.MMap.File.PathnameStr = str

		return nil

	case "mmap.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.Mode"}
		}
		e.MMap.File.FileFields.Mode = uint16(v)

		return nil

	case "mmap.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.UID"}
		}
		e.MMap.File.FileFields.UID = uint32(v)

		return nil

	case "mmap.file.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.File.FileFields.User"}
		}
		e.MMap.File.FileFields.User = str

		return nil

	case "mmap.flags":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.Flags"}
		}
		e.MMap.Flags = int(v)

		return nil

	case "mmap.protection":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.Protection"}
		}
		e.MMap.Protection = int(v)

		return nil

	case "mmap.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MMap.SyscallEvent.Retval"}
		}
		e.MMap.SyscallEvent.Retval = int64(v)

		return nil

	case "mprotect.req_protection":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MProtect.ReqProtection"}
		}
		e.MProtect.ReqProtection = int(v)

		return nil

	case "mprotect.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MProtect.SyscallEvent.Retval"}
		}
		e.MProtect.SyscallEvent.Retval = int64(v)

		return nil

	case "mprotect.vm_protection":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "MProtect.VMProtection"}
		}
		e.MProtect.VMProtection = int(v)

		return nil

	case "open.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.CTime"}
		}
		e.Open.File.FileFields.CTime = uint64(v)

		return nil

	case "open.file.destination.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Mode"}
		}
		e.Open.Mode = uint32(v)

		return nil

	case "open.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.Filesytem"}
		}
		e.Open.File.Filesytem = str

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

	case "open.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.MTime"}
		}
		e.Open.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.BasenameStr"}
		}
		e.Open.File.BasenameStr = str

		return nil

	case "open.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.PathnameStr"}
		}
		e.Open.File.PathnameStr = str

		return nil

	case "open.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.Mode"}
		}
		e.Open.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.File.FileFields.User"}
		}
		e.Open.File.FileFields.User = str

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

	case "process.ancestors.args":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Args"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Args = str

		return nil

	case "process.ancestors.args_flags":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(e.ProcessContext.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "process.ancestors.args_options":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(e.ProcessContext.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "process.ancestors.args_truncated":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.ArgsTruncated"}
		}
		return nil

	case "process.ancestors.argv":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Argv = append(e.ProcessContext.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "process.ancestors.argv0":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Argv0"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Argv0 = str

		return nil

	case "process.ancestors.cap_effective":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapEffective"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapEffective = uint64(v)

		return nil

	case "process.ancestors.cap_permitted":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapPermitted"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.CapPermitted = uint64(v)

		return nil

	case "process.ancestors.comm":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Comm"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Comm = str

		return nil

	case "process.ancestors.container.id":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.ContainerID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.ContainerID = str

		return nil

	case "process.ancestors.cookie":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Cookie"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Cookie = uint32(v)

		return nil

	case "process.ancestors.created_at":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.CreatedAt"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.CreatedAt = uint64(v)

		return nil

	case "process.ancestors.egid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGID = uint32(v)

		return nil

	case "process.ancestors.egroup":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGroup"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EGroup = str

		return nil

	case "process.ancestors.envp":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Envp"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Envp = append(e.ProcessContext.Ancestor.ProcessContext.Process.Envp, str)

		return nil

	case "process.ancestors.envs":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Envs"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Envs = append(e.ProcessContext.Ancestor.ProcessContext.Process.Envs, str)

		return nil

	case "process.ancestors.envs_truncated":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.EnvsTruncated"}
		}
		return nil

	case "process.ancestors.euid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUID = uint32(v)

		return nil

	case "process.ancestors.euser":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUser"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.EUser = str

		return nil

	case "process.ancestors.file.change_time":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.CTime"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.CTime = uint64(v)

		return nil

	case "process.ancestors.file.filesystem":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Filesystem"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Filesystem = str

		return nil

	case "process.ancestors.file.gid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.GID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.GID = uint32(v)

		return nil

	case "process.ancestors.file.group":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.Group"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.Group = str

		return nil

	case "process.ancestors.file.in_upper_layer":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.InUpperLayer"}
		}
		return nil

	case "process.ancestors.file.inode":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.Inode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.Inode = uint64(v)

		return nil

	case "process.ancestors.file.mode":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.Mode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.Mode = uint16(v)

		return nil

	case "process.ancestors.file.modification_time":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.MTime"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.MTime = uint64(v)

		return nil

	case "process.ancestors.file.mount_id":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.MountID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.MountID = uint32(v)

		return nil

	case "process.ancestors.file.name":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.BasenameStr"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.BasenameStr = str

		return nil

	case "process.ancestors.file.path":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PathnameStr"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.PathnameStr = str

		return nil

	case "process.ancestors.file.rights":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.Mode"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.Mode = uint16(v)

		return nil

	case "process.ancestors.file.uid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.UID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.UID = uint32(v)

		return nil

	case "process.ancestors.file.user":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.FileFields.User"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.FileFields.User = str

		return nil

	case "process.ancestors.fsgid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGID = uint32(v)

		return nil

	case "process.ancestors.fsgroup":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGroup"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSGroup = str

		return nil

	case "process.ancestors.fsuid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUID = uint32(v)

		return nil

	case "process.ancestors.fsuser":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUser"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.FSUser = str

		return nil

	case "process.ancestors.gid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.GID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.GID = uint32(v)

		return nil

	case "process.ancestors.group":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.Group"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.Group = str

		return nil

	case "process.ancestors.pid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Pid"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Pid = uint32(v)

		return nil

	case "process.ancestors.ppid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.PPid"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.PPid = uint32(v)

		return nil

	case "process.ancestors.tid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Tid"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Tid = uint32(v)

		return nil

	case "process.ancestors.tty_name":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.TTYName"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.TTYName = str

		return nil

	case "process.ancestors.uid":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.UID"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.UID = uint32(v)

		return nil

	case "process.ancestors.user":

		if e.ProcessContext.Ancestor == nil {
			e.ProcessContext.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Ancestor.ProcessContext.Process.Credentials.User"}
		}
		e.ProcessContext.Ancestor.ProcessContext.Process.Credentials.User = str

		return nil

	case "process.args":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Args"}
		}
		e.ProcessContext.Process.Args = str

		return nil

	case "process.args_flags":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Process.Argv = append(e.ProcessContext.Process.Argv, str)

		return nil

	case "process.args_options":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Process.Argv = append(e.ProcessContext.Process.Argv, str)

		return nil

	case "process.args_truncated":

		var ok bool
		if e.ProcessContext.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.ArgsTruncated"}
		}
		return nil

	case "process.argv":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv"}
		}
		e.ProcessContext.Process.Argv = append(e.ProcessContext.Process.Argv, str)

		return nil

	case "process.argv0":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Argv0"}
		}
		e.ProcessContext.Process.Argv0 = str

		return nil

	case "process.cap_effective":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.CapEffective"}
		}
		e.ProcessContext.Process.Credentials.CapEffective = uint64(v)

		return nil

	case "process.cap_permitted":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.CapPermitted"}
		}
		e.ProcessContext.Process.Credentials.CapPermitted = uint64(v)

		return nil

	case "process.comm":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Comm"}
		}
		e.ProcessContext.Process.Comm = str

		return nil

	case "process.container.id":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.ContainerID"}
		}
		e.ProcessContext.Process.ContainerID = str

		return nil

	case "process.cookie":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Cookie"}
		}
		e.ProcessContext.Process.Cookie = uint32(v)

		return nil

	case "process.created_at":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.CreatedAt"}
		}
		e.ProcessContext.Process.CreatedAt = uint64(v)

		return nil

	case "process.egid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EGID"}
		}
		e.ProcessContext.Process.Credentials.EGID = uint32(v)

		return nil

	case "process.egroup":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EGroup"}
		}
		e.ProcessContext.Process.Credentials.EGroup = str

		return nil

	case "process.envp":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Envp"}
		}
		e.ProcessContext.Process.Envp = append(e.ProcessContext.Process.Envp, str)

		return nil

	case "process.envs":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Envs"}
		}
		e.ProcessContext.Process.Envs = append(e.ProcessContext.Process.Envs, str)

		return nil

	case "process.envs_truncated":

		var ok bool
		if e.ProcessContext.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.EnvsTruncated"}
		}
		return nil

	case "process.euid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EUID"}
		}
		e.ProcessContext.Process.Credentials.EUID = uint32(v)

		return nil

	case "process.euser":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.EUser"}
		}
		e.ProcessContext.Process.Credentials.EUser = str

		return nil

	case "process.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.CTime"}
		}
		e.ProcessContext.Process.FileFields.CTime = uint64(v)

		return nil

	case "process.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Filesystem"}
		}
		e.ProcessContext.Process.Filesystem = str

		return nil

	case "process.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.GID"}
		}
		e.ProcessContext.Process.FileFields.GID = uint32(v)

		return nil

	case "process.file.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.Group"}
		}
		e.ProcessContext.Process.FileFields.Group = str

		return nil

	case "process.file.in_upper_layer":

		var ok bool
		if e.ProcessContext.Process.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.InUpperLayer"}
		}
		return nil

	case "process.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.Inode"}
		}
		e.ProcessContext.Process.FileFields.Inode = uint64(v)

		return nil

	case "process.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.Mode"}
		}
		e.ProcessContext.Process.FileFields.Mode = uint16(v)

		return nil

	case "process.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.MTime"}
		}
		e.ProcessContext.Process.FileFields.MTime = uint64(v)

		return nil

	case "process.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.MountID"}
		}
		e.ProcessContext.Process.FileFields.MountID = uint32(v)

		return nil

	case "process.file.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.BasenameStr"}
		}
		e.ProcessContext.Process.BasenameStr = str

		return nil

	case "process.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PathnameStr"}
		}
		e.ProcessContext.Process.PathnameStr = str

		return nil

	case "process.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.Mode"}
		}
		e.ProcessContext.Process.FileFields.Mode = uint16(v)

		return nil

	case "process.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.UID"}
		}
		e.ProcessContext.Process.FileFields.UID = uint32(v)

		return nil

	case "process.file.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.FileFields.User"}
		}
		e.ProcessContext.Process.FileFields.User = str

		return nil

	case "process.fsgid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSGID"}
		}
		e.ProcessContext.Process.Credentials.FSGID = uint32(v)

		return nil

	case "process.fsgroup":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSGroup"}
		}
		e.ProcessContext.Process.Credentials.FSGroup = str

		return nil

	case "process.fsuid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSUID"}
		}
		e.ProcessContext.Process.Credentials.FSUID = uint32(v)

		return nil

	case "process.fsuser":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.FSUser"}
		}
		e.ProcessContext.Process.Credentials.FSUser = str

		return nil

	case "process.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.GID"}
		}
		e.ProcessContext.Process.Credentials.GID = uint32(v)

		return nil

	case "process.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.Group"}
		}
		e.ProcessContext.Process.Credentials.Group = str

		return nil

	case "process.pid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Pid"}
		}
		e.ProcessContext.Process.Pid = uint32(v)

		return nil

	case "process.ppid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.PPid"}
		}
		e.ProcessContext.Process.PPid = uint32(v)

		return nil

	case "process.tid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Tid"}
		}
		e.ProcessContext.Process.Tid = uint32(v)

		return nil

	case "process.tty_name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.TTYName"}
		}
		e.ProcessContext.Process.TTYName = str

		return nil

	case "process.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.UID"}
		}
		e.ProcessContext.Process.Credentials.UID = uint32(v)

		return nil

	case "process.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ProcessContext.Process.Credentials.User"}
		}
		e.ProcessContext.Process.Credentials.User = str

		return nil

	case "ptrace.request":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Request"}
		}
		e.PTrace.Request = uint32(v)

		return nil

	case "ptrace.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.SyscallEvent.Retval"}
		}
		e.PTrace.SyscallEvent.Retval = int64(v)

		return nil

	case "ptrace.tracee.ancestors.args":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Args"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Args = str

		return nil

	case "ptrace.tracee.ancestors.args_flags":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Argv"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "ptrace.tracee.ancestors.args_options":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Argv"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "ptrace.tracee.ancestors.args_truncated":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.ArgsTruncated"}
		}
		return nil

	case "ptrace.tracee.ancestors.argv":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Argv"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "ptrace.tracee.ancestors.argv0":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Argv0"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Argv0 = str

		return nil

	case "ptrace.tracee.ancestors.cap_effective":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.CapEffective"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.CapEffective = uint64(v)

		return nil

	case "ptrace.tracee.ancestors.cap_permitted":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.CapPermitted"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.CapPermitted = uint64(v)

		return nil

	case "ptrace.tracee.ancestors.comm":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Comm"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Comm = str

		return nil

	case "ptrace.tracee.ancestors.container.id":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.ContainerID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.ContainerID = str

		return nil

	case "ptrace.tracee.ancestors.cookie":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Cookie"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Cookie = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.created_at":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.CreatedAt"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.CreatedAt = uint64(v)

		return nil

	case "ptrace.tracee.ancestors.egid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EGID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EGID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.egroup":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EGroup"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EGroup = str

		return nil

	case "ptrace.tracee.ancestors.envp":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Envp"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Envp = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Envp, str)

		return nil

	case "ptrace.tracee.ancestors.envs":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Envs"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Envs = append(e.PTrace.Tracee.Ancestor.ProcessContext.Process.Envs, str)

		return nil

	case "ptrace.tracee.ancestors.envs_truncated":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.EnvsTruncated"}
		}
		return nil

	case "ptrace.tracee.ancestors.euid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EUID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EUID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.euser":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EUser"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.EUser = str

		return nil

	case "ptrace.tracee.ancestors.file.change_time":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.CTime"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.CTime = uint64(v)

		return nil

	case "ptrace.tracee.ancestors.file.filesystem":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Filesystem"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Filesystem = str

		return nil

	case "ptrace.tracee.ancestors.file.gid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.GID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.GID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.file.group":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.Group"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.Group = str

		return nil

	case "ptrace.tracee.ancestors.file.in_upper_layer":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.InUpperLayer"}
		}
		return nil

	case "ptrace.tracee.ancestors.file.inode":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.Inode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.Inode = uint64(v)

		return nil

	case "ptrace.tracee.ancestors.file.mode":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.Mode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.Mode = uint16(v)

		return nil

	case "ptrace.tracee.ancestors.file.modification_time":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.MTime"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.MTime = uint64(v)

		return nil

	case "ptrace.tracee.ancestors.file.mount_id":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.MountID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.MountID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.file.name":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.BasenameStr"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.BasenameStr = str

		return nil

	case "ptrace.tracee.ancestors.file.path":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.PathnameStr"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.PathnameStr = str

		return nil

	case "ptrace.tracee.ancestors.file.rights":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.Mode"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.Mode = uint16(v)

		return nil

	case "ptrace.tracee.ancestors.file.uid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.UID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.UID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.file.user":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.User"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.FileFields.User = str

		return nil

	case "ptrace.tracee.ancestors.fsgid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSGID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSGID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.fsgroup":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSGroup"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSGroup = str

		return nil

	case "ptrace.tracee.ancestors.fsuid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSUID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSUID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.fsuser":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSUser"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.FSUser = str

		return nil

	case "ptrace.tracee.ancestors.gid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.GID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.GID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.group":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.Group"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.Group = str

		return nil

	case "ptrace.tracee.ancestors.pid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Pid"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Pid = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.ppid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.PPid"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.PPid = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.tid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Tid"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Tid = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.tty_name":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.TTYName"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.TTYName = str

		return nil

	case "ptrace.tracee.ancestors.uid":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.UID"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.UID = uint32(v)

		return nil

	case "ptrace.tracee.ancestors.user":

		if e.PTrace.Tracee.Ancestor == nil {
			e.PTrace.Tracee.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.User"}
		}
		e.PTrace.Tracee.Ancestor.ProcessContext.Process.Credentials.User = str

		return nil

	case "ptrace.tracee.args":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Args"}
		}
		e.PTrace.Tracee.Process.Args = str

		return nil

	case "ptrace.tracee.args_flags":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Argv"}
		}
		e.PTrace.Tracee.Process.Argv = append(e.PTrace.Tracee.Process.Argv, str)

		return nil

	case "ptrace.tracee.args_options":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Argv"}
		}
		e.PTrace.Tracee.Process.Argv = append(e.PTrace.Tracee.Process.Argv, str)

		return nil

	case "ptrace.tracee.args_truncated":

		var ok bool
		if e.PTrace.Tracee.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.ArgsTruncated"}
		}
		return nil

	case "ptrace.tracee.argv":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Argv"}
		}
		e.PTrace.Tracee.Process.Argv = append(e.PTrace.Tracee.Process.Argv, str)

		return nil

	case "ptrace.tracee.argv0":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Argv0"}
		}
		e.PTrace.Tracee.Process.Argv0 = str

		return nil

	case "ptrace.tracee.cap_effective":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.CapEffective"}
		}
		e.PTrace.Tracee.Process.Credentials.CapEffective = uint64(v)

		return nil

	case "ptrace.tracee.cap_permitted":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.CapPermitted"}
		}
		e.PTrace.Tracee.Process.Credentials.CapPermitted = uint64(v)

		return nil

	case "ptrace.tracee.comm":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Comm"}
		}
		e.PTrace.Tracee.Process.Comm = str

		return nil

	case "ptrace.tracee.container.id":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.ContainerID"}
		}
		e.PTrace.Tracee.Process.ContainerID = str

		return nil

	case "ptrace.tracee.cookie":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Cookie"}
		}
		e.PTrace.Tracee.Process.Cookie = uint32(v)

		return nil

	case "ptrace.tracee.created_at":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.CreatedAt"}
		}
		e.PTrace.Tracee.Process.CreatedAt = uint64(v)

		return nil

	case "ptrace.tracee.egid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.EGID"}
		}
		e.PTrace.Tracee.Process.Credentials.EGID = uint32(v)

		return nil

	case "ptrace.tracee.egroup":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.EGroup"}
		}
		e.PTrace.Tracee.Process.Credentials.EGroup = str

		return nil

	case "ptrace.tracee.envp":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Envp"}
		}
		e.PTrace.Tracee.Process.Envp = append(e.PTrace.Tracee.Process.Envp, str)

		return nil

	case "ptrace.tracee.envs":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Envs"}
		}
		e.PTrace.Tracee.Process.Envs = append(e.PTrace.Tracee.Process.Envs, str)

		return nil

	case "ptrace.tracee.envs_truncated":

		var ok bool
		if e.PTrace.Tracee.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.EnvsTruncated"}
		}
		return nil

	case "ptrace.tracee.euid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.EUID"}
		}
		e.PTrace.Tracee.Process.Credentials.EUID = uint32(v)

		return nil

	case "ptrace.tracee.euser":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.EUser"}
		}
		e.PTrace.Tracee.Process.Credentials.EUser = str

		return nil

	case "ptrace.tracee.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.CTime"}
		}
		e.PTrace.Tracee.Process.FileFields.CTime = uint64(v)

		return nil

	case "ptrace.tracee.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Filesystem"}
		}
		e.PTrace.Tracee.Process.Filesystem = str

		return nil

	case "ptrace.tracee.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.GID"}
		}
		e.PTrace.Tracee.Process.FileFields.GID = uint32(v)

		return nil

	case "ptrace.tracee.file.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.Group"}
		}
		e.PTrace.Tracee.Process.FileFields.Group = str

		return nil

	case "ptrace.tracee.file.in_upper_layer":

		var ok bool
		if e.PTrace.Tracee.Process.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.InUpperLayer"}
		}
		return nil

	case "ptrace.tracee.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.Inode"}
		}
		e.PTrace.Tracee.Process.FileFields.Inode = uint64(v)

		return nil

	case "ptrace.tracee.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.Mode"}
		}
		e.PTrace.Tracee.Process.FileFields.Mode = uint16(v)

		return nil

	case "ptrace.tracee.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.MTime"}
		}
		e.PTrace.Tracee.Process.FileFields.MTime = uint64(v)

		return nil

	case "ptrace.tracee.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.MountID"}
		}
		e.PTrace.Tracee.Process.FileFields.MountID = uint32(v)

		return nil

	case "ptrace.tracee.file.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.BasenameStr"}
		}
		e.PTrace.Tracee.Process.BasenameStr = str

		return nil

	case "ptrace.tracee.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.PathnameStr"}
		}
		e.PTrace.Tracee.Process.PathnameStr = str

		return nil

	case "ptrace.tracee.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.Mode"}
		}
		e.PTrace.Tracee.Process.FileFields.Mode = uint16(v)

		return nil

	case "ptrace.tracee.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.UID"}
		}
		e.PTrace.Tracee.Process.FileFields.UID = uint32(v)

		return nil

	case "ptrace.tracee.file.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.FileFields.User"}
		}
		e.PTrace.Tracee.Process.FileFields.User = str

		return nil

	case "ptrace.tracee.fsgid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.FSGID"}
		}
		e.PTrace.Tracee.Process.Credentials.FSGID = uint32(v)

		return nil

	case "ptrace.tracee.fsgroup":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.FSGroup"}
		}
		e.PTrace.Tracee.Process.Credentials.FSGroup = str

		return nil

	case "ptrace.tracee.fsuid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.FSUID"}
		}
		e.PTrace.Tracee.Process.Credentials.FSUID = uint32(v)

		return nil

	case "ptrace.tracee.fsuser":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.FSUser"}
		}
		e.PTrace.Tracee.Process.Credentials.FSUser = str

		return nil

	case "ptrace.tracee.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.GID"}
		}
		e.PTrace.Tracee.Process.Credentials.GID = uint32(v)

		return nil

	case "ptrace.tracee.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.Group"}
		}
		e.PTrace.Tracee.Process.Credentials.Group = str

		return nil

	case "ptrace.tracee.pid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Pid"}
		}
		e.PTrace.Tracee.Process.Pid = uint32(v)

		return nil

	case "ptrace.tracee.ppid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.PPid"}
		}
		e.PTrace.Tracee.Process.PPid = uint32(v)

		return nil

	case "ptrace.tracee.tid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Tid"}
		}
		e.PTrace.Tracee.Process.Tid = uint32(v)

		return nil

	case "ptrace.tracee.tty_name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.TTYName"}
		}
		e.PTrace.Tracee.Process.TTYName = str

		return nil

	case "ptrace.tracee.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.UID"}
		}
		e.PTrace.Tracee.Process.Credentials.UID = uint32(v)

		return nil

	case "ptrace.tracee.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "PTrace.Tracee.Process.Credentials.User"}
		}
		e.PTrace.Tracee.Process.Credentials.User = str

		return nil

	case "removexattr.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.CTime"}
		}
		e.RemoveXAttr.File.FileFields.CTime = uint64(v)

		return nil

	case "removexattr.file.destination.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Name"}
		}
		e.RemoveXAttr.Name = str

		return nil

	case "removexattr.file.destination.namespace":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Namespace"}
		}
		e.RemoveXAttr.Namespace = str

		return nil

	case "removexattr.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.Filesytem"}
		}
		e.RemoveXAttr.File.Filesytem = str

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

	case "removexattr.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.MTime"}
		}
		e.RemoveXAttr.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.BasenameStr"}
		}
		e.RemoveXAttr.File.BasenameStr = str

		return nil

	case "removexattr.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.PathnameStr"}
		}
		e.RemoveXAttr.File.PathnameStr = str

		return nil

	case "removexattr.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.Mode"}
		}
		e.RemoveXAttr.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.File.FileFields.User"}
		}
		e.RemoveXAttr.File.FileFields.User = str

		return nil

	case "removexattr.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.SyscallEvent.Retval"}
		}
		e.RemoveXAttr.SyscallEvent.Retval = int64(v)

		return nil

	case "rename.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.CTime"}
		}
		e.Rename.Old.FileFields.CTime = uint64(v)

		return nil

	case "rename.file.destination.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.CTime"}
		}
		e.Rename.New.FileFields.CTime = uint64(v)

		return nil

	case "rename.file.destination.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.Filesytem"}
		}
		e.Rename.New.Filesytem = str

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

	case "rename.file.destination.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.MTime"}
		}
		e.Rename.New.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.BasenameStr"}
		}
		e.Rename.New.BasenameStr = str

		return nil

	case "rename.file.destination.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.PathnameStr"}
		}
		e.Rename.New.PathnameStr = str

		return nil

	case "rename.file.destination.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.Mode"}
		}
		e.Rename.New.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.FileFields.User"}
		}
		e.Rename.New.FileFields.User = str

		return nil

	case "rename.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.Filesytem"}
		}
		e.Rename.Old.Filesytem = str

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

	case "rename.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.MTime"}
		}
		e.Rename.Old.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.BasenameStr"}
		}
		e.Rename.Old.BasenameStr = str

		return nil

	case "rename.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.PathnameStr"}
		}
		e.Rename.Old.PathnameStr = str

		return nil

	case "rename.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.Mode"}
		}
		e.Rename.Old.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.FileFields.User"}
		}
		e.Rename.Old.FileFields.User = str

		return nil

	case "rename.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.SyscallEvent.Retval"}
		}
		e.Rename.SyscallEvent.Retval = int64(v)

		return nil

	case "rmdir.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.CTime"}
		}
		e.Rmdir.File.FileFields.CTime = uint64(v)

		return nil

	case "rmdir.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.Filesytem"}
		}
		e.Rmdir.File.Filesytem = str

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

	case "rmdir.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.MTime"}
		}
		e.Rmdir.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.BasenameStr"}
		}
		e.Rmdir.File.BasenameStr = str

		return nil

	case "rmdir.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.PathnameStr"}
		}
		e.Rmdir.File.PathnameStr = str

		return nil

	case "rmdir.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.Mode"}
		}
		e.Rmdir.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.File.FileFields.User"}
		}
		e.Rmdir.File.FileFields.User = str

		return nil

	case "rmdir.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.SyscallEvent.Retval"}
		}
		e.Rmdir.SyscallEvent.Retval = int64(v)

		return nil

	case "selinux.bool.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SELinux.BoolName"}
		}
		e.SELinux.BoolName = str

		return nil

	case "selinux.bool.state":

		var ok bool
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

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SELinux.EnforceStatus"}
		}
		e.SELinux.EnforceStatus = str

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.EGroup"}
		}
		e.SetGID.EGroup = str

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.FSGroup"}
		}
		e.SetGID.FSGroup = str

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetGID.Group"}
		}
		e.SetGID.Group = str

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.EUser"}
		}
		e.SetUID.EUser = str

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.FSUser"}
		}
		e.SetUID.FSUser = str

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetUID.User"}
		}
		e.SetUID.User = str

		return nil

	case "setxattr.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.CTime"}
		}
		e.SetXAttr.File.FileFields.CTime = uint64(v)

		return nil

	case "setxattr.file.destination.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Name"}
		}
		e.SetXAttr.Name = str

		return nil

	case "setxattr.file.destination.namespace":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Namespace"}
		}
		e.SetXAttr.Namespace = str

		return nil

	case "setxattr.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.Filesytem"}
		}
		e.SetXAttr.File.Filesytem = str

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

	case "setxattr.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.MTime"}
		}
		e.SetXAttr.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.BasenameStr"}
		}
		e.SetXAttr.File.BasenameStr = str

		return nil

	case "setxattr.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.PathnameStr"}
		}
		e.SetXAttr.File.PathnameStr = str

		return nil

	case "setxattr.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.Mode"}
		}
		e.SetXAttr.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.File.FileFields.User"}
		}
		e.SetXAttr.File.FileFields.User = str

		return nil

	case "setxattr.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.SyscallEvent.Retval"}
		}
		e.SetXAttr.SyscallEvent.Retval = int64(v)

		return nil

	case "signal.pid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.PID"}
		}
		e.Signal.PID = uint32(v)

		return nil

	case "signal.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.SyscallEvent.Retval"}
		}
		e.Signal.SyscallEvent.Retval = int64(v)

		return nil

	case "signal.target.ancestors.args":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Args"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Args = str

		return nil

	case "signal.target.ancestors.args_flags":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Argv"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Argv = append(e.Signal.Target.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "signal.target.ancestors.args_options":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Argv"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Argv = append(e.Signal.Target.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "signal.target.ancestors.args_truncated":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.ArgsTruncated"}
		}
		return nil

	case "signal.target.ancestors.argv":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Argv"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Argv = append(e.Signal.Target.Ancestor.ProcessContext.Process.Argv, str)

		return nil

	case "signal.target.ancestors.argv0":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Argv0"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Argv0 = str

		return nil

	case "signal.target.ancestors.cap_effective":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.CapEffective"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.CapEffective = uint64(v)

		return nil

	case "signal.target.ancestors.cap_permitted":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.CapPermitted"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.CapPermitted = uint64(v)

		return nil

	case "signal.target.ancestors.comm":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Comm"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Comm = str

		return nil

	case "signal.target.ancestors.container.id":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.ContainerID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.ContainerID = str

		return nil

	case "signal.target.ancestors.cookie":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Cookie"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Cookie = uint32(v)

		return nil

	case "signal.target.ancestors.created_at":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.CreatedAt"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.CreatedAt = uint64(v)

		return nil

	case "signal.target.ancestors.egid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.EGID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.EGID = uint32(v)

		return nil

	case "signal.target.ancestors.egroup":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.EGroup"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.EGroup = str

		return nil

	case "signal.target.ancestors.envp":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Envp"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Envp = append(e.Signal.Target.Ancestor.ProcessContext.Process.Envp, str)

		return nil

	case "signal.target.ancestors.envs":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Envs"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Envs = append(e.Signal.Target.Ancestor.ProcessContext.Process.Envs, str)

		return nil

	case "signal.target.ancestors.envs_truncated":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.EnvsTruncated"}
		}
		return nil

	case "signal.target.ancestors.euid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.EUID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.EUID = uint32(v)

		return nil

	case "signal.target.ancestors.euser":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.EUser"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.EUser = str

		return nil

	case "signal.target.ancestors.file.change_time":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.CTime"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.CTime = uint64(v)

		return nil

	case "signal.target.ancestors.file.filesystem":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Filesystem"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Filesystem = str

		return nil

	case "signal.target.ancestors.file.gid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.GID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.GID = uint32(v)

		return nil

	case "signal.target.ancestors.file.group":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.Group"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.Group = str

		return nil

	case "signal.target.ancestors.file.in_upper_layer":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		if e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.InUpperLayer"}
		}
		return nil

	case "signal.target.ancestors.file.inode":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.Inode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.Inode = uint64(v)

		return nil

	case "signal.target.ancestors.file.mode":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.Mode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.Mode = uint16(v)

		return nil

	case "signal.target.ancestors.file.modification_time":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.MTime"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.MTime = uint64(v)

		return nil

	case "signal.target.ancestors.file.mount_id":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.MountID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.MountID = uint32(v)

		return nil

	case "signal.target.ancestors.file.name":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.BasenameStr"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.BasenameStr = str

		return nil

	case "signal.target.ancestors.file.path":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.PathnameStr"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.PathnameStr = str

		return nil

	case "signal.target.ancestors.file.rights":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.Mode"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.Mode = uint16(v)

		return nil

	case "signal.target.ancestors.file.uid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.UID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.UID = uint32(v)

		return nil

	case "signal.target.ancestors.file.user":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.FileFields.User"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.FileFields.User = str

		return nil

	case "signal.target.ancestors.fsgid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSGID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSGID = uint32(v)

		return nil

	case "signal.target.ancestors.fsgroup":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSGroup"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSGroup = str

		return nil

	case "signal.target.ancestors.fsuid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSUID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSUID = uint32(v)

		return nil

	case "signal.target.ancestors.fsuser":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSUser"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.FSUser = str

		return nil

	case "signal.target.ancestors.gid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.GID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.GID = uint32(v)

		return nil

	case "signal.target.ancestors.group":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.Group"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.Group = str

		return nil

	case "signal.target.ancestors.pid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Pid"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Pid = uint32(v)

		return nil

	case "signal.target.ancestors.ppid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.PPid"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.PPid = uint32(v)

		return nil

	case "signal.target.ancestors.tid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Tid"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Tid = uint32(v)

		return nil

	case "signal.target.ancestors.tty_name":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.TTYName"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.TTYName = str

		return nil

	case "signal.target.ancestors.uid":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.UID"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.UID = uint32(v)

		return nil

	case "signal.target.ancestors.user":

		if e.Signal.Target.Ancestor == nil {
			e.Signal.Target.Ancestor = &model.ProcessCacheEntry{}
		}

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Ancestor.ProcessContext.Process.Credentials.User"}
		}
		e.Signal.Target.Ancestor.ProcessContext.Process.Credentials.User = str

		return nil

	case "signal.target.args":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Args"}
		}
		e.Signal.Target.Process.Args = str

		return nil

	case "signal.target.args_flags":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Argv"}
		}
		e.Signal.Target.Process.Argv = append(e.Signal.Target.Process.Argv, str)

		return nil

	case "signal.target.args_options":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Argv"}
		}
		e.Signal.Target.Process.Argv = append(e.Signal.Target.Process.Argv, str)

		return nil

	case "signal.target.args_truncated":

		var ok bool
		if e.Signal.Target.Process.ArgsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.ArgsTruncated"}
		}
		return nil

	case "signal.target.argv":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Argv"}
		}
		e.Signal.Target.Process.Argv = append(e.Signal.Target.Process.Argv, str)

		return nil

	case "signal.target.argv0":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Argv0"}
		}
		e.Signal.Target.Process.Argv0 = str

		return nil

	case "signal.target.cap_effective":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.CapEffective"}
		}
		e.Signal.Target.Process.Credentials.CapEffective = uint64(v)

		return nil

	case "signal.target.cap_permitted":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.CapPermitted"}
		}
		e.Signal.Target.Process.Credentials.CapPermitted = uint64(v)

		return nil

	case "signal.target.comm":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Comm"}
		}
		e.Signal.Target.Process.Comm = str

		return nil

	case "signal.target.container.id":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.ContainerID"}
		}
		e.Signal.Target.Process.ContainerID = str

		return nil

	case "signal.target.cookie":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Cookie"}
		}
		e.Signal.Target.Process.Cookie = uint32(v)

		return nil

	case "signal.target.created_at":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.CreatedAt"}
		}
		e.Signal.Target.Process.CreatedAt = uint64(v)

		return nil

	case "signal.target.egid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.EGID"}
		}
		e.Signal.Target.Process.Credentials.EGID = uint32(v)

		return nil

	case "signal.target.egroup":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.EGroup"}
		}
		e.Signal.Target.Process.Credentials.EGroup = str

		return nil

	case "signal.target.envp":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Envp"}
		}
		e.Signal.Target.Process.Envp = append(e.Signal.Target.Process.Envp, str)

		return nil

	case "signal.target.envs":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Envs"}
		}
		e.Signal.Target.Process.Envs = append(e.Signal.Target.Process.Envs, str)

		return nil

	case "signal.target.envs_truncated":

		var ok bool
		if e.Signal.Target.Process.EnvsTruncated, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.EnvsTruncated"}
		}
		return nil

	case "signal.target.euid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.EUID"}
		}
		e.Signal.Target.Process.Credentials.EUID = uint32(v)

		return nil

	case "signal.target.euser":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.EUser"}
		}
		e.Signal.Target.Process.Credentials.EUser = str

		return nil

	case "signal.target.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.CTime"}
		}
		e.Signal.Target.Process.FileFields.CTime = uint64(v)

		return nil

	case "signal.target.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Filesystem"}
		}
		e.Signal.Target.Process.Filesystem = str

		return nil

	case "signal.target.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.GID"}
		}
		e.Signal.Target.Process.FileFields.GID = uint32(v)

		return nil

	case "signal.target.file.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.Group"}
		}
		e.Signal.Target.Process.FileFields.Group = str

		return nil

	case "signal.target.file.in_upper_layer":

		var ok bool
		if e.Signal.Target.Process.FileFields.InUpperLayer, ok = value.(bool); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.InUpperLayer"}
		}
		return nil

	case "signal.target.file.inode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.Inode"}
		}
		e.Signal.Target.Process.FileFields.Inode = uint64(v)

		return nil

	case "signal.target.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.Mode"}
		}
		e.Signal.Target.Process.FileFields.Mode = uint16(v)

		return nil

	case "signal.target.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.MTime"}
		}
		e.Signal.Target.Process.FileFields.MTime = uint64(v)

		return nil

	case "signal.target.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.MountID"}
		}
		e.Signal.Target.Process.FileFields.MountID = uint32(v)

		return nil

	case "signal.target.file.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.BasenameStr"}
		}
		e.Signal.Target.Process.BasenameStr = str

		return nil

	case "signal.target.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.PathnameStr"}
		}
		e.Signal.Target.Process.PathnameStr = str

		return nil

	case "signal.target.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.Mode"}
		}
		e.Signal.Target.Process.FileFields.Mode = uint16(v)

		return nil

	case "signal.target.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.UID"}
		}
		e.Signal.Target.Process.FileFields.UID = uint32(v)

		return nil

	case "signal.target.file.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.FileFields.User"}
		}
		e.Signal.Target.Process.FileFields.User = str

		return nil

	case "signal.target.fsgid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.FSGID"}
		}
		e.Signal.Target.Process.Credentials.FSGID = uint32(v)

		return nil

	case "signal.target.fsgroup":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.FSGroup"}
		}
		e.Signal.Target.Process.Credentials.FSGroup = str

		return nil

	case "signal.target.fsuid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.FSUID"}
		}
		e.Signal.Target.Process.Credentials.FSUID = uint32(v)

		return nil

	case "signal.target.fsuser":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.FSUser"}
		}
		e.Signal.Target.Process.Credentials.FSUser = str

		return nil

	case "signal.target.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.GID"}
		}
		e.Signal.Target.Process.Credentials.GID = uint32(v)

		return nil

	case "signal.target.group":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.Group"}
		}
		e.Signal.Target.Process.Credentials.Group = str

		return nil

	case "signal.target.pid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Pid"}
		}
		e.Signal.Target.Process.Pid = uint32(v)

		return nil

	case "signal.target.ppid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.PPid"}
		}
		e.Signal.Target.Process.PPid = uint32(v)

		return nil

	case "signal.target.tid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Tid"}
		}
		e.Signal.Target.Process.Tid = uint32(v)

		return nil

	case "signal.target.tty_name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.TTYName"}
		}
		e.Signal.Target.Process.TTYName = str

		return nil

	case "signal.target.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.UID"}
		}
		e.Signal.Target.Process.Credentials.UID = uint32(v)

		return nil

	case "signal.target.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Target.Process.Credentials.User"}
		}
		e.Signal.Target.Process.Credentials.User = str

		return nil

	case "signal.type":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Signal.Type"}
		}
		e.Signal.Type = uint32(v)

		return nil

	case "splice.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.CTime"}
		}
		e.Splice.File.FileFields.CTime = uint64(v)

		return nil

	case "splice.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.Filesytem"}
		}
		e.Splice.File.Filesytem = str

		return nil

	case "splice.file.gid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.GID"}
		}
		e.Splice.File.FileFields.GID = uint32(v)

		return nil

	case "splice.file.group":

		var ok bool
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

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.Inode"}
		}
		e.Splice.File.FileFields.Inode = uint64(v)

		return nil

	case "splice.file.mode":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.Mode"}
		}
		e.Splice.File.FileFields.Mode = uint16(v)

		return nil

	case "splice.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.MTime"}
		}
		e.Splice.File.FileFields.MTime = uint64(v)

		return nil

	case "splice.file.mount_id":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.MountID"}
		}
		e.Splice.File.FileFields.MountID = uint32(v)

		return nil

	case "splice.file.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.BasenameStr"}
		}
		e.Splice.File.BasenameStr = str

		return nil

	case "splice.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.PathnameStr"}
		}
		e.Splice.File.PathnameStr = str

		return nil

	case "splice.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.Mode"}
		}
		e.Splice.File.FileFields.Mode = uint16(v)

		return nil

	case "splice.file.uid":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.UID"}
		}
		e.Splice.File.FileFields.UID = uint32(v)

		return nil

	case "splice.file.user":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.File.FileFields.User"}
		}
		e.Splice.File.FileFields.User = str

		return nil

	case "splice.pipe_entry_flag":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.PipeEntryFlag"}
		}
		e.Splice.PipeEntryFlag = uint32(v)

		return nil

	case "splice.pipe_exit_flag":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.PipeExitFlag"}
		}
		e.Splice.PipeExitFlag = uint32(v)

		return nil

	case "splice.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Splice.SyscallEvent.Retval"}
		}
		e.Splice.SyscallEvent.Retval = int64(v)

		return nil

	case "unlink.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.CTime"}
		}
		e.Unlink.File.FileFields.CTime = uint64(v)

		return nil

	case "unlink.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.Filesytem"}
		}
		e.Unlink.File.Filesytem = str

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

	case "unlink.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.MTime"}
		}
		e.Unlink.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.BasenameStr"}
		}
		e.Unlink.File.BasenameStr = str

		return nil

	case "unlink.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.PathnameStr"}
		}
		e.Unlink.File.PathnameStr = str

		return nil

	case "unlink.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.Mode"}
		}
		e.Unlink.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.File.FileFields.User"}
		}
		e.Unlink.File.FileFields.User = str

		return nil

	case "unlink.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.SyscallEvent.Retval"}
		}
		e.Unlink.SyscallEvent.Retval = int64(v)

		return nil

	case "unload_module.name":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "UnloadModule.Name"}
		}
		e.UnloadModule.Name = str

		return nil

	case "unload_module.retval":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "UnloadModule.SyscallEvent.Retval"}
		}
		e.UnloadModule.SyscallEvent.Retval = int64(v)

		return nil

	case "utimes.file.change_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.CTime"}
		}
		e.Utimes.File.FileFields.CTime = uint64(v)

		return nil

	case "utimes.file.filesystem":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.Filesytem"}
		}
		e.Utimes.File.Filesytem = str

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

	case "utimes.file.modification_time":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.MTime"}
		}
		e.Utimes.File.FileFields.MTime = uint64(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.BasenameStr"}
		}
		e.Utimes.File.BasenameStr = str

		return nil

	case "utimes.file.path":

		var ok bool
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.PathnameStr"}
		}
		e.Utimes.File.PathnameStr = str

		return nil

	case "utimes.file.rights":

		var ok bool
		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.Mode"}
		}
		e.Utimes.File.FileFields.Mode = uint16(v)

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
		str, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.File.FileFields.User"}
		}
		e.Utimes.File.FileFields.User = str

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
