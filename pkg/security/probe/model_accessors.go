// +build linux

// Code generated - DO NOT EDIT.

package probe

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

func (m *Model) GetEvaluator(field eval.Field) (eval.Evaluator, error) {
	switch field {

	case "chmod.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chmod.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "chmod.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chmod.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "chmod.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chmod.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "chmod.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chmod.Inode) },

			Field: field,
		}, nil

	case "chmod.mode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chmod.Mode) },

			Field: field,
		}, nil

	case "chmod.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chmod.OverlayNumLower) },

			Field: field,
		}, nil

	case "chmod.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chmod.Retval) },

			Field: field,
		}, nil

	case "chown.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "chown.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "chown.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Chown.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "chown.gid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chown.GID) },

			Field: field,
		}, nil

	case "chown.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chown.Inode) },

			Field: field,
		}, nil

	case "chown.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chown.OverlayNumLower) },

			Field: field,
		}, nil

	case "chown.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chown.Retval) },

			Field: field,
		}, nil

	case "chown.uid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chown.UID) },

			Field: field,
		}, nil

	case "container.id":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Container.ResolveContainerID((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.Retval) },

			Field: field,
		}, nil

	case "link.source.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Source.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.source.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Source.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.source.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Source.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.source.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.Source.Inode) },

			Field: field,
		}, nil

	case "link.source.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.Source.OverlayNumLower) },

			Field: field,
		}, nil

	case "link.target.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Target.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.target.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Target.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.target.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.Target.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.target.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.Target.Inode) },

			Field: field,
		}, nil

	case "link.target.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.Target.OverlayNumLower) },

			Field: field,
		}, nil

	case "mkdir.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Mkdir.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "mkdir.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Mkdir.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "mkdir.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Mkdir.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "mkdir.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Mkdir.Inode) },

			Field: field,
		}, nil

	case "mkdir.mode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Mkdir.Mode) },

			Field: field,
		}, nil

	case "mkdir.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Mkdir.OverlayNumLower) },

			Field: field,
		}, nil

	case "mkdir.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Mkdir.Retval) },

			Field: field,
		}, nil

	case "open.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Open.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "open.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Open.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "open.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Open.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "open.flags":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Open.Flags) },

			Field: field,
		}, nil

	case "open.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Open.Inode) },

			Field: field,
		}, nil

	case "open.mode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Open.Mode) },

			Field: field,
		}, nil

	case "open.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Open.OverlayNumLower) },

			Field: field,
		}, nil

	case "open.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Open.Retval) },

			Field: field,
		}, nil

	case "process.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Process.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "process.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Process.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "process.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Process.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "process.gid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Process.GID) },

			Field: field,
		}, nil

	case "process.group":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Process.ResolveGroup((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "process.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Process.Inode) },

			Field: field,
		}, nil

	case "process.name":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Process.ResolveComm((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "process.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Process.OverlayNumLower) },

			Field: field,
		}, nil

	case "process.pid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Process.Pid) },

			Field: field,
		}, nil

	case "process.pidns":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Process.Pidns) },

			Field: field,
		}, nil

	case "process.tid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Process.Tid) },

			Field: field,
		}, nil

	case "process.tty_name":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Process.ResolveTTY((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "process.uid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Process.UID) },

			Field: field,
		}, nil

	case "process.user":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Process.ResolveUser((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "removexattr.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "removexattr.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "removexattr.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "removexattr.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).RemoveXAttr.Inode) },

			Field: field,
		}, nil

	case "removexattr.name":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.GetName((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "removexattr.namespace":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).RemoveXAttr.GetNamespace((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "removexattr.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).RemoveXAttr.OverlayNumLower) },

			Field: field,
		}, nil

	case "removexattr.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).RemoveXAttr.Retval) },

			Field: field,
		}, nil

	case "rename.new.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.New.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.new.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.New.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.new.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.New.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.new.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.New.Inode) },

			Field: field,
		}, nil

	case "rename.new.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.New.OverlayNumLower) },

			Field: field,
		}, nil

	case "rename.old.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.Old.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.old.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.Old.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.old.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.Old.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.old.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.Old.Inode) },

			Field: field,
		}, nil

	case "rename.old.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.Old.OverlayNumLower) },

			Field: field,
		}, nil

	case "rename.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.Retval) },

			Field: field,
		}, nil

	case "rmdir.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rmdir.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rmdir.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rmdir.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rmdir.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rmdir.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rmdir.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rmdir.Inode) },

			Field: field,
		}, nil

	case "rmdir.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rmdir.OverlayNumLower) },

			Field: field,
		}, nil

	case "rmdir.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rmdir.Retval) },

			Field: field,
		}, nil

	case "setxattr.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "setxattr.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "setxattr.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "setxattr.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).SetXAttr.Inode) },

			Field: field,
		}, nil

	case "setxattr.name":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.GetName((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "setxattr.namespace":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).SetXAttr.GetNamespace((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "setxattr.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).SetXAttr.OverlayNumLower) },

			Field: field,
		}, nil

	case "setxattr.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).SetXAttr.Retval) },

			Field: field,
		}, nil

	case "unlink.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Unlink.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "unlink.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Unlink.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "unlink.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Unlink.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "unlink.flags":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Unlink.Flags) },

			Field: field,
		}, nil

	case "unlink.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Unlink.Inode) },

			Field: field,
		}, nil

	case "unlink.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Unlink.OverlayNumLower) },

			Field: field,
		}, nil

	case "unlink.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Unlink.Retval) },

			Field: field,
		}, nil

	case "utimes.basename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Utimes.ResolveBasename((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "utimes.container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Utimes.ResolveContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "utimes.filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Utimes.ResolveInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "utimes.inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Utimes.Inode) },

			Field: field,
		}, nil

	case "utimes.overlay_numlower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Utimes.OverlayNumLower) },

			Field: field,
		}, nil

	case "utimes.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Utimes.Retval) },

			Field: field,
		}, nil

	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {

	case "chmod.basename":

		return e.Chmod.ResolveBasename(e.resolvers), nil

	case "chmod.container_path":

		return e.Chmod.ResolveContainerPath(e.resolvers), nil

	case "chmod.filename":

		return e.Chmod.ResolveInode(e.resolvers), nil

	case "chmod.inode":

		return int(e.Chmod.Inode), nil

	case "chmod.mode":

		return int(e.Chmod.Mode), nil

	case "chmod.overlay_numlower":

		return int(e.Chmod.OverlayNumLower), nil

	case "chmod.retval":

		return int(e.Chmod.Retval), nil

	case "chown.basename":

		return e.Chown.ResolveBasename(e.resolvers), nil

	case "chown.container_path":

		return e.Chown.ResolveContainerPath(e.resolvers), nil

	case "chown.filename":

		return e.Chown.ResolveInode(e.resolvers), nil

	case "chown.gid":

		return int(e.Chown.GID), nil

	case "chown.inode":

		return int(e.Chown.Inode), nil

	case "chown.overlay_numlower":

		return int(e.Chown.OverlayNumLower), nil

	case "chown.retval":

		return int(e.Chown.Retval), nil

	case "chown.uid":

		return int(e.Chown.UID), nil

	case "container.id":

		return e.Container.ResolveContainerID(e.resolvers), nil

	case "link.retval":

		return int(e.Link.Retval), nil

	case "link.source.basename":

		return e.Link.Source.ResolveBasename(e.resolvers), nil

	case "link.source.container_path":

		return e.Link.Source.ResolveContainerPath(e.resolvers), nil

	case "link.source.filename":

		return e.Link.Source.ResolveInode(e.resolvers), nil

	case "link.source.inode":

		return int(e.Link.Source.Inode), nil

	case "link.source.overlay_numlower":

		return int(e.Link.Source.OverlayNumLower), nil

	case "link.target.basename":

		return e.Link.Target.ResolveBasename(e.resolvers), nil

	case "link.target.container_path":

		return e.Link.Target.ResolveContainerPath(e.resolvers), nil

	case "link.target.filename":

		return e.Link.Target.ResolveInode(e.resolvers), nil

	case "link.target.inode":

		return int(e.Link.Target.Inode), nil

	case "link.target.overlay_numlower":

		return int(e.Link.Target.OverlayNumLower), nil

	case "mkdir.basename":

		return e.Mkdir.ResolveBasename(e.resolvers), nil

	case "mkdir.container_path":

		return e.Mkdir.ResolveContainerPath(e.resolvers), nil

	case "mkdir.filename":

		return e.Mkdir.ResolveInode(e.resolvers), nil

	case "mkdir.inode":

		return int(e.Mkdir.Inode), nil

	case "mkdir.mode":

		return int(e.Mkdir.Mode), nil

	case "mkdir.overlay_numlower":

		return int(e.Mkdir.OverlayNumLower), nil

	case "mkdir.retval":

		return int(e.Mkdir.Retval), nil

	case "open.basename":

		return e.Open.ResolveBasename(e.resolvers), nil

	case "open.container_path":

		return e.Open.ResolveContainerPath(e.resolvers), nil

	case "open.filename":

		return e.Open.ResolveInode(e.resolvers), nil

	case "open.flags":

		return int(e.Open.Flags), nil

	case "open.inode":

		return int(e.Open.Inode), nil

	case "open.mode":

		return int(e.Open.Mode), nil

	case "open.overlay_numlower":

		return int(e.Open.OverlayNumLower), nil

	case "open.retval":

		return int(e.Open.Retval), nil

	case "process.basename":

		return e.Process.ResolveBasename(e.resolvers), nil

	case "process.container_path":

		return e.Process.ResolveContainerPath(e.resolvers), nil

	case "process.filename":

		return e.Process.ResolveInode(e.resolvers), nil

	case "process.gid":

		return int(e.Process.GID), nil

	case "process.group":

		return e.Process.ResolveGroup(e.resolvers), nil

	case "process.inode":

		return int(e.Process.Inode), nil

	case "process.name":

		return e.Process.ResolveComm(e.resolvers), nil

	case "process.overlay_numlower":

		return int(e.Process.OverlayNumLower), nil

	case "process.pid":

		return int(e.Process.Pid), nil

	case "process.pidns":

		return int(e.Process.Pidns), nil

	case "process.tid":

		return int(e.Process.Tid), nil

	case "process.tty_name":

		return e.Process.ResolveTTY(e.resolvers), nil

	case "process.uid":

		return int(e.Process.UID), nil

	case "process.user":

		return e.Process.ResolveUser(e.resolvers), nil

	case "removexattr.basename":

		return e.RemoveXAttr.ResolveBasename(e.resolvers), nil

	case "removexattr.container_path":

		return e.RemoveXAttr.ResolveContainerPath(e.resolvers), nil

	case "removexattr.filename":

		return e.RemoveXAttr.ResolveInode(e.resolvers), nil

	case "removexattr.inode":

		return int(e.RemoveXAttr.Inode), nil

	case "removexattr.name":

		return e.RemoveXAttr.GetName(e.resolvers), nil

	case "removexattr.namespace":

		return e.RemoveXAttr.GetNamespace(e.resolvers), nil

	case "removexattr.overlay_numlower":

		return int(e.RemoveXAttr.OverlayNumLower), nil

	case "removexattr.retval":

		return int(e.RemoveXAttr.Retval), nil

	case "rename.new.basename":

		return e.Rename.New.ResolveBasename(e.resolvers), nil

	case "rename.new.container_path":

		return e.Rename.New.ResolveContainerPath(e.resolvers), nil

	case "rename.new.filename":

		return e.Rename.New.ResolveInode(e.resolvers), nil

	case "rename.new.inode":

		return int(e.Rename.New.Inode), nil

	case "rename.new.overlay_numlower":

		return int(e.Rename.New.OverlayNumLower), nil

	case "rename.old.basename":

		return e.Rename.Old.ResolveBasename(e.resolvers), nil

	case "rename.old.container_path":

		return e.Rename.Old.ResolveContainerPath(e.resolvers), nil

	case "rename.old.filename":

		return e.Rename.Old.ResolveInode(e.resolvers), nil

	case "rename.old.inode":

		return int(e.Rename.Old.Inode), nil

	case "rename.old.overlay_numlower":

		return int(e.Rename.Old.OverlayNumLower), nil

	case "rename.retval":

		return int(e.Rename.Retval), nil

	case "rmdir.basename":

		return e.Rmdir.ResolveBasename(e.resolvers), nil

	case "rmdir.container_path":

		return e.Rmdir.ResolveContainerPath(e.resolvers), nil

	case "rmdir.filename":

		return e.Rmdir.ResolveInode(e.resolvers), nil

	case "rmdir.inode":

		return int(e.Rmdir.Inode), nil

	case "rmdir.overlay_numlower":

		return int(e.Rmdir.OverlayNumLower), nil

	case "rmdir.retval":

		return int(e.Rmdir.Retval), nil

	case "setxattr.basename":

		return e.SetXAttr.ResolveBasename(e.resolvers), nil

	case "setxattr.container_path":

		return e.SetXAttr.ResolveContainerPath(e.resolvers), nil

	case "setxattr.filename":

		return e.SetXAttr.ResolveInode(e.resolvers), nil

	case "setxattr.inode":

		return int(e.SetXAttr.Inode), nil

	case "setxattr.name":

		return e.SetXAttr.GetName(e.resolvers), nil

	case "setxattr.namespace":

		return e.SetXAttr.GetNamespace(e.resolvers), nil

	case "setxattr.overlay_numlower":

		return int(e.SetXAttr.OverlayNumLower), nil

	case "setxattr.retval":

		return int(e.SetXAttr.Retval), nil

	case "unlink.basename":

		return e.Unlink.ResolveBasename(e.resolvers), nil

	case "unlink.container_path":

		return e.Unlink.ResolveContainerPath(e.resolvers), nil

	case "unlink.filename":

		return e.Unlink.ResolveInode(e.resolvers), nil

	case "unlink.flags":

		return int(e.Unlink.Flags), nil

	case "unlink.inode":

		return int(e.Unlink.Inode), nil

	case "unlink.overlay_numlower":

		return int(e.Unlink.OverlayNumLower), nil

	case "unlink.retval":

		return int(e.Unlink.Retval), nil

	case "utimes.basename":

		return e.Utimes.ResolveBasename(e.resolvers), nil

	case "utimes.container_path":

		return e.Utimes.ResolveContainerPath(e.resolvers), nil

	case "utimes.filename":

		return e.Utimes.ResolveInode(e.resolvers), nil

	case "utimes.inode":

		return int(e.Utimes.Inode), nil

	case "utimes.overlay_numlower":

		return int(e.Utimes.OverlayNumLower), nil

	case "utimes.retval":

		return int(e.Utimes.Retval), nil

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

	case "process.basename":
		return "*", nil

	case "process.container_path":
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

	case "process.pidns":
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

	case "process.basename":

		return reflect.String, nil

	case "process.container_path":

		return reflect.String, nil

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

	case "process.pidns":

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
	var ok bool
	switch field {

	case "chmod.basename":

		if e.Chmod.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.BasenameStr"}
		}
		return nil

	case "chmod.container_path":

		if e.Chmod.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.ContainerPath"}
		}
		return nil

	case "chmod.filename":

		if e.Chmod.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.PathnameStr"}
		}
		return nil

	case "chmod.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Inode"}
		}
		e.Chmod.Inode = uint64(v)
		return nil

	case "chmod.mode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Mode"}
		}
		e.Chmod.Mode = uint32(v)
		return nil

	case "chmod.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.OverlayNumLower"}
		}
		e.Chmod.OverlayNumLower = int32(v)
		return nil

	case "chmod.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.Retval"}
		}
		e.Chmod.Retval = int64(v)
		return nil

	case "chown.basename":

		if e.Chown.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.BasenameStr"}
		}
		return nil

	case "chown.container_path":

		if e.Chown.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.ContainerPath"}
		}
		return nil

	case "chown.filename":

		if e.Chown.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.PathnameStr"}
		}
		return nil

	case "chown.gid":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.GID"}
		}
		e.Chown.GID = int32(v)
		return nil

	case "chown.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.Inode"}
		}
		e.Chown.Inode = uint64(v)
		return nil

	case "chown.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.OverlayNumLower"}
		}
		e.Chown.OverlayNumLower = int32(v)
		return nil

	case "chown.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.Retval"}
		}
		e.Chown.Retval = int64(v)
		return nil

	case "chown.uid":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.UID"}
		}
		e.Chown.UID = int32(v)
		return nil

	case "container.id":

		if e.Container.ID, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Container.ID"}
		}
		return nil

	case "link.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Retval"}
		}
		e.Link.Retval = int64(v)
		return nil

	case "link.source.basename":

		if e.Link.Source.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.BasenameStr"}
		}
		return nil

	case "link.source.container_path":

		if e.Link.Source.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.ContainerPath"}
		}
		return nil

	case "link.source.filename":

		if e.Link.Source.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.PathnameStr"}
		}
		return nil

	case "link.source.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.Inode"}
		}
		e.Link.Source.Inode = uint64(v)
		return nil

	case "link.source.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Source.OverlayNumLower"}
		}
		e.Link.Source.OverlayNumLower = int32(v)
		return nil

	case "link.target.basename":

		if e.Link.Target.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.BasenameStr"}
		}
		return nil

	case "link.target.container_path":

		if e.Link.Target.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.ContainerPath"}
		}
		return nil

	case "link.target.filename":

		if e.Link.Target.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.PathnameStr"}
		}
		return nil

	case "link.target.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.Inode"}
		}
		e.Link.Target.Inode = uint64(v)
		return nil

	case "link.target.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.Target.OverlayNumLower"}
		}
		e.Link.Target.OverlayNumLower = int32(v)
		return nil

	case "mkdir.basename":

		if e.Mkdir.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.BasenameStr"}
		}
		return nil

	case "mkdir.container_path":

		if e.Mkdir.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.ContainerPath"}
		}
		return nil

	case "mkdir.filename":

		if e.Mkdir.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.PathnameStr"}
		}
		return nil

	case "mkdir.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Inode"}
		}
		e.Mkdir.Inode = uint64(v)
		return nil

	case "mkdir.mode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Mode"}
		}
		e.Mkdir.Mode = int32(v)
		return nil

	case "mkdir.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.OverlayNumLower"}
		}
		e.Mkdir.OverlayNumLower = int32(v)
		return nil

	case "mkdir.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.Retval"}
		}
		e.Mkdir.Retval = int64(v)
		return nil

	case "open.basename":

		if e.Open.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.BasenameStr"}
		}
		return nil

	case "open.container_path":

		if e.Open.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.ContainerPath"}
		}
		return nil

	case "open.filename":

		if e.Open.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.PathnameStr"}
		}
		return nil

	case "open.flags":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Flags"}
		}
		e.Open.Flags = uint32(v)
		return nil

	case "open.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Inode"}
		}
		e.Open.Inode = uint64(v)
		return nil

	case "open.mode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Mode"}
		}
		e.Open.Mode = uint32(v)
		return nil

	case "open.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.OverlayNumLower"}
		}
		e.Open.OverlayNumLower = int32(v)
		return nil

	case "open.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.Retval"}
		}
		e.Open.Retval = int64(v)
		return nil

	case "process.basename":

		if e.Process.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.BasenameStr"}
		}
		return nil

	case "process.container_path":

		if e.Process.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.ContainerPath"}
		}
		return nil

	case "process.filename":

		if e.Process.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.PathnameStr"}
		}
		return nil

	case "process.gid":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.GID"}
		}
		e.Process.GID = uint32(v)
		return nil

	case "process.group":

		if e.Process.Group, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Group"}
		}
		return nil

	case "process.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Inode"}
		}
		e.Process.Inode = uint64(v)
		return nil

	case "process.name":

		if e.Process.Comm, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Comm"}
		}
		return nil

	case "process.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.OverlayNumLower"}
		}
		e.Process.OverlayNumLower = int32(v)
		return nil

	case "process.pid":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Pid"}
		}
		e.Process.Pid = uint32(v)
		return nil

	case "process.pidns":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Pidns"}
		}
		e.Process.Pidns = uint64(v)
		return nil

	case "process.tid":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Tid"}
		}
		e.Process.Tid = uint32(v)
		return nil

	case "process.tty_name":

		if e.Process.TTYName, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.TTYName"}
		}
		return nil

	case "process.uid":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.UID"}
		}
		e.Process.UID = uint32(v)
		return nil

	case "process.user":

		if e.Process.User, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.User"}
		}
		return nil

	case "removexattr.basename":

		if e.RemoveXAttr.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.BasenameStr"}
		}
		return nil

	case "removexattr.container_path":

		if e.RemoveXAttr.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.ContainerPath"}
		}
		return nil

	case "removexattr.filename":

		if e.RemoveXAttr.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.PathnameStr"}
		}
		return nil

	case "removexattr.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Inode"}
		}
		e.RemoveXAttr.Inode = uint64(v)
		return nil

	case "removexattr.name":

		if e.RemoveXAttr.Name, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Name"}
		}
		return nil

	case "removexattr.namespace":

		if e.RemoveXAttr.Namespace, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Namespace"}
		}
		return nil

	case "removexattr.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.OverlayNumLower"}
		}
		e.RemoveXAttr.OverlayNumLower = int32(v)
		return nil

	case "removexattr.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RemoveXAttr.Retval"}
		}
		e.RemoveXAttr.Retval = int64(v)
		return nil

	case "rename.new.basename":

		if e.Rename.New.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.BasenameStr"}
		}
		return nil

	case "rename.new.container_path":

		if e.Rename.New.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.ContainerPath"}
		}
		return nil

	case "rename.new.filename":

		if e.Rename.New.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.PathnameStr"}
		}
		return nil

	case "rename.new.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.Inode"}
		}
		e.Rename.New.Inode = uint64(v)
		return nil

	case "rename.new.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.New.OverlayNumLower"}
		}
		e.Rename.New.OverlayNumLower = int32(v)
		return nil

	case "rename.old.basename":

		if e.Rename.Old.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.BasenameStr"}
		}
		return nil

	case "rename.old.container_path":

		if e.Rename.Old.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.ContainerPath"}
		}
		return nil

	case "rename.old.filename":

		if e.Rename.Old.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.PathnameStr"}
		}
		return nil

	case "rename.old.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.Inode"}
		}
		e.Rename.Old.Inode = uint64(v)
		return nil

	case "rename.old.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Old.OverlayNumLower"}
		}
		e.Rename.Old.OverlayNumLower = int32(v)
		return nil

	case "rename.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.Retval"}
		}
		e.Rename.Retval = int64(v)
		return nil

	case "rmdir.basename":

		if e.Rmdir.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.BasenameStr"}
		}
		return nil

	case "rmdir.container_path":

		if e.Rmdir.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.ContainerPath"}
		}
		return nil

	case "rmdir.filename":

		if e.Rmdir.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.PathnameStr"}
		}
		return nil

	case "rmdir.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.Inode"}
		}
		e.Rmdir.Inode = uint64(v)
		return nil

	case "rmdir.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.OverlayNumLower"}
		}
		e.Rmdir.OverlayNumLower = int32(v)
		return nil

	case "rmdir.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.Retval"}
		}
		e.Rmdir.Retval = int64(v)
		return nil

	case "setxattr.basename":

		if e.SetXAttr.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.BasenameStr"}
		}
		return nil

	case "setxattr.container_path":

		if e.SetXAttr.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.ContainerPath"}
		}
		return nil

	case "setxattr.filename":

		if e.SetXAttr.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.PathnameStr"}
		}
		return nil

	case "setxattr.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Inode"}
		}
		e.SetXAttr.Inode = uint64(v)
		return nil

	case "setxattr.name":

		if e.SetXAttr.Name, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Name"}
		}
		return nil

	case "setxattr.namespace":

		if e.SetXAttr.Namespace, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Namespace"}
		}
		return nil

	case "setxattr.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.OverlayNumLower"}
		}
		e.SetXAttr.OverlayNumLower = int32(v)
		return nil

	case "setxattr.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetXAttr.Retval"}
		}
		e.SetXAttr.Retval = int64(v)
		return nil

	case "unlink.basename":

		if e.Unlink.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.BasenameStr"}
		}
		return nil

	case "unlink.container_path":

		if e.Unlink.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.ContainerPath"}
		}
		return nil

	case "unlink.filename":

		if e.Unlink.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.PathnameStr"}
		}
		return nil

	case "unlink.flags":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.Flags"}
		}
		e.Unlink.Flags = uint32(v)
		return nil

	case "unlink.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.Inode"}
		}
		e.Unlink.Inode = uint64(v)
		return nil

	case "unlink.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.OverlayNumLower"}
		}
		e.Unlink.OverlayNumLower = int32(v)
		return nil

	case "unlink.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.Retval"}
		}
		e.Unlink.Retval = int64(v)
		return nil

	case "utimes.basename":

		if e.Utimes.BasenameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.BasenameStr"}
		}
		return nil

	case "utimes.container_path":

		if e.Utimes.ContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.ContainerPath"}
		}
		return nil

	case "utimes.filename":

		if e.Utimes.PathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.PathnameStr"}
		}
		return nil

	case "utimes.inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.Inode"}
		}
		e.Utimes.Inode = uint64(v)
		return nil

	case "utimes.overlay_numlower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.OverlayNumLower"}
		}
		e.Utimes.OverlayNumLower = int32(v)
		return nil

	case "utimes.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.Retval"}
		}
		e.Utimes.Retval = int64(v)
		return nil

	}

	return &eval.ErrFieldNotFound{Field: field}
}
