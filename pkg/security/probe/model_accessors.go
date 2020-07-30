// +build linux_bpf

// Code generated - DO NOT EDIT.

package probe

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

func (m *Model) GetEvaluator(field eval.Field) (eval.Evaluator, error) {
	switch field {

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

	case "chmod.overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chmod.OverlayNumLower) },

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

	case "chown.overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chown.OverlayNumLower) },

			Field: field,
		}, nil

	case "chown.uid":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Chown.UID) },

			Field: field,
		}, nil

	case "container.id":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return (*Event)(ctx.Object).Container.ID },

			Field: field,
		}, nil

	case "event.retval":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Event.Retval) },

			Field: field,
		}, nil

	case "event.type":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Event.ResolveType((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.new_container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.ResolveNewContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.new_filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.ResolveNewInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.new_inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.NewInode) },

			Field: field,
		}, nil

	case "link.new_overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.NewOverlayNumLower) },

			Field: field,
		}, nil

	case "link.src_container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.ResolveSrcContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.src_filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Link.ResolveSrcInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "link.src_inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.SrcInode) },

			Field: field,
		}, nil

	case "link.src_overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Link.SrcOverlayNumLower) },

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

	case "mkdir.overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Mkdir.OverlayNumLower) },

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

	case "open.overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Open.OverlayNumLower) },

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

	case "process.name":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Process.ResolveComm((*Event)(ctx.Object).resolvers)
			},

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

	case "rename.new_filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.ResolveTargetInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.new_inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.TargetInode) },

			Field: field,
		}, nil

	case "rename.old_filename":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.ResolveSrcInode((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.old_inode":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.SrcInode) },

			Field: field,
		}, nil

	case "rename.src_container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.ResolveSrcContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.src_overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.SrcOverlayNumLower) },

			Field: field,
		}, nil

	case "rename.target_container_path":

		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				return (*Event)(ctx.Object).Rename.ResolveTargetContainerPath((*Event)(ctx.Object).resolvers)
			},

			Field: field,
		}, nil

	case "rename.target_overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rename.TargetOverlayNumLower) },

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

	case "rmdir.overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Rmdir.OverlayNumLower) },

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

	case "unlink.overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Unlink.OverlayNumLower) },

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

	case "utimes.overlay_num_lower":

		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int { return int((*Event)(ctx.Object).Utimes.OverlayNumLower) },

			Field: field,
		}, nil

	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {

	case "chmod.container_path":

		return e.Chmod.ResolveContainerPath(e.resolvers), nil

	case "chmod.filename":

		return e.Chmod.ResolveInode(e.resolvers), nil

	case "chmod.inode":

		return int(e.Chmod.Inode), nil

	case "chmod.mode":

		return int(e.Chmod.Mode), nil

	case "chmod.overlay_num_lower":

		return int(e.Chmod.OverlayNumLower), nil

	case "chown.container_path":

		return e.Chown.ResolveContainerPath(e.resolvers), nil

	case "chown.filename":

		return e.Chown.ResolveInode(e.resolvers), nil

	case "chown.gid":

		return int(e.Chown.GID), nil

	case "chown.inode":

		return int(e.Chown.Inode), nil

	case "chown.overlay_num_lower":

		return int(e.Chown.OverlayNumLower), nil

	case "chown.uid":

		return int(e.Chown.UID), nil

	case "container.id":

		return e.Container.ID, nil

	case "event.retval":

		return int(e.Event.Retval), nil

	case "event.type":

		return e.Event.ResolveType(e.resolvers), nil

	case "link.new_container_path":

		return e.Link.ResolveNewContainerPath(e.resolvers), nil

	case "link.new_filename":

		return e.Link.ResolveNewInode(e.resolvers), nil

	case "link.new_inode":

		return int(e.Link.NewInode), nil

	case "link.new_overlay_num_lower":

		return int(e.Link.NewOverlayNumLower), nil

	case "link.src_container_path":

		return e.Link.ResolveSrcContainerPath(e.resolvers), nil

	case "link.src_filename":

		return e.Link.ResolveSrcInode(e.resolvers), nil

	case "link.src_inode":

		return int(e.Link.SrcInode), nil

	case "link.src_overlay_num_lower":

		return int(e.Link.SrcOverlayNumLower), nil

	case "mkdir.container_path":

		return e.Mkdir.ResolveContainerPath(e.resolvers), nil

	case "mkdir.filename":

		return e.Mkdir.ResolveInode(e.resolvers), nil

	case "mkdir.inode":

		return int(e.Mkdir.Inode), nil

	case "mkdir.mode":

		return int(e.Mkdir.Mode), nil

	case "mkdir.overlay_num_lower":

		return int(e.Mkdir.OverlayNumLower), nil

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

	case "open.overlay_num_lower":

		return int(e.Open.OverlayNumLower), nil

	case "process.filename":

		return e.Process.ResolveInode(e.resolvers), nil

	case "process.gid":

		return int(e.Process.GID), nil

	case "process.group":

		return e.Process.ResolveGroup(e.resolvers), nil

	case "process.name":

		return e.Process.ResolveComm(e.resolvers), nil

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

	case "rename.new_filename":

		return e.Rename.ResolveTargetInode(e.resolvers), nil

	case "rename.new_inode":

		return int(e.Rename.TargetInode), nil

	case "rename.old_filename":

		return e.Rename.ResolveSrcInode(e.resolvers), nil

	case "rename.old_inode":

		return int(e.Rename.SrcInode), nil

	case "rename.src_container_path":

		return e.Rename.ResolveSrcContainerPath(e.resolvers), nil

	case "rename.src_overlay_num_lower":

		return int(e.Rename.SrcOverlayNumLower), nil

	case "rename.target_container_path":

		return e.Rename.ResolveTargetContainerPath(e.resolvers), nil

	case "rename.target_overlay_num_lower":

		return int(e.Rename.TargetOverlayNumLower), nil

	case "rmdir.container_path":

		return e.Rmdir.ResolveContainerPath(e.resolvers), nil

	case "rmdir.filename":

		return e.Rmdir.ResolveInode(e.resolvers), nil

	case "rmdir.inode":

		return int(e.Rmdir.Inode), nil

	case "rmdir.overlay_num_lower":

		return int(e.Rmdir.OverlayNumLower), nil

	case "unlink.container_path":

		return e.Unlink.ResolveContainerPath(e.resolvers), nil

	case "unlink.filename":

		return e.Unlink.ResolveInode(e.resolvers), nil

	case "unlink.flags":

		return int(e.Unlink.Flags), nil

	case "unlink.inode":

		return int(e.Unlink.Inode), nil

	case "unlink.overlay_num_lower":

		return int(e.Unlink.OverlayNumLower), nil

	case "utimes.container_path":

		return e.Utimes.ResolveContainerPath(e.resolvers), nil

	case "utimes.filename":

		return e.Utimes.ResolveInode(e.resolvers), nil

	case "utimes.inode":

		return int(e.Utimes.Inode), nil

	case "utimes.overlay_num_lower":

		return int(e.Utimes.OverlayNumLower), nil

	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {

	case "chmod.container_path":
		return "chmod", nil

	case "chmod.filename":
		return "chmod", nil

	case "chmod.inode":
		return "chmod", nil

	case "chmod.mode":
		return "chmod", nil

	case "chmod.overlay_num_lower":
		return "chmod", nil

	case "chown.container_path":
		return "chown", nil

	case "chown.filename":
		return "chown", nil

	case "chown.gid":
		return "chown", nil

	case "chown.inode":
		return "chown", nil

	case "chown.overlay_num_lower":
		return "chown", nil

	case "chown.uid":
		return "chown", nil

	case "container.id":
		return "container", nil

	case "event.retval":
		return "*", nil

	case "event.type":
		return "*", nil

	case "link.new_container_path":
		return "link", nil

	case "link.new_filename":
		return "link", nil

	case "link.new_inode":
		return "link", nil

	case "link.new_overlay_num_lower":
		return "link", nil

	case "link.src_container_path":
		return "link", nil

	case "link.src_filename":
		return "link", nil

	case "link.src_inode":
		return "link", nil

	case "link.src_overlay_num_lower":
		return "link", nil

	case "mkdir.container_path":
		return "mkdir", nil

	case "mkdir.filename":
		return "mkdir", nil

	case "mkdir.inode":
		return "mkdir", nil

	case "mkdir.mode":
		return "mkdir", nil

	case "mkdir.overlay_num_lower":
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

	case "open.overlay_num_lower":
		return "open", nil

	case "process.filename":
		return "*", nil

	case "process.gid":
		return "*", nil

	case "process.group":
		return "*", nil

	case "process.name":
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

	case "rename.new_filename":
		return "rename", nil

	case "rename.new_inode":
		return "rename", nil

	case "rename.old_filename":
		return "rename", nil

	case "rename.old_inode":
		return "rename", nil

	case "rename.src_container_path":
		return "rename", nil

	case "rename.src_overlay_num_lower":
		return "rename", nil

	case "rename.target_container_path":
		return "rename", nil

	case "rename.target_overlay_num_lower":
		return "rename", nil

	case "rmdir.container_path":
		return "rmdir", nil

	case "rmdir.filename":
		return "rmdir", nil

	case "rmdir.inode":
		return "rmdir", nil

	case "rmdir.overlay_num_lower":
		return "rmdir", nil

	case "unlink.container_path":
		return "unlink", nil

	case "unlink.filename":
		return "unlink", nil

	case "unlink.flags":
		return "unlink", nil

	case "unlink.inode":
		return "unlink", nil

	case "unlink.overlay_num_lower":
		return "unlink", nil

	case "utimes.container_path":
		return "utimes", nil

	case "utimes.filename":
		return "utimes", nil

	case "utimes.inode":
		return "utimes", nil

	case "utimes.overlay_num_lower":
		return "utimes", nil

	}

	return "", &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {

	case "chmod.container_path":

		return reflect.String, nil

	case "chmod.filename":

		return reflect.String, nil

	case "chmod.inode":

		return reflect.Int, nil

	case "chmod.mode":

		return reflect.Int, nil

	case "chmod.overlay_num_lower":

		return reflect.Int, nil

	case "chown.container_path":

		return reflect.String, nil

	case "chown.filename":

		return reflect.String, nil

	case "chown.gid":

		return reflect.Int, nil

	case "chown.inode":

		return reflect.Int, nil

	case "chown.overlay_num_lower":

		return reflect.Int, nil

	case "chown.uid":

		return reflect.Int, nil

	case "container.id":

		return reflect.String, nil

	case "event.retval":

		return reflect.Int, nil

	case "event.type":

		return reflect.String, nil

	case "link.new_container_path":

		return reflect.String, nil

	case "link.new_filename":

		return reflect.String, nil

	case "link.new_inode":

		return reflect.Int, nil

	case "link.new_overlay_num_lower":

		return reflect.Int, nil

	case "link.src_container_path":

		return reflect.String, nil

	case "link.src_filename":

		return reflect.String, nil

	case "link.src_inode":

		return reflect.Int, nil

	case "link.src_overlay_num_lower":

		return reflect.Int, nil

	case "mkdir.container_path":

		return reflect.String, nil

	case "mkdir.filename":

		return reflect.String, nil

	case "mkdir.inode":

		return reflect.Int, nil

	case "mkdir.mode":

		return reflect.Int, nil

	case "mkdir.overlay_num_lower":

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

	case "open.overlay_num_lower":

		return reflect.Int, nil

	case "process.filename":

		return reflect.String, nil

	case "process.gid":

		return reflect.Int, nil

	case "process.group":

		return reflect.String, nil

	case "process.name":

		return reflect.String, nil

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

	case "rename.new_filename":

		return reflect.String, nil

	case "rename.new_inode":

		return reflect.Int, nil

	case "rename.old_filename":

		return reflect.String, nil

	case "rename.old_inode":

		return reflect.Int, nil

	case "rename.src_container_path":

		return reflect.String, nil

	case "rename.src_overlay_num_lower":

		return reflect.Int, nil

	case "rename.target_container_path":

		return reflect.String, nil

	case "rename.target_overlay_num_lower":

		return reflect.Int, nil

	case "rmdir.container_path":

		return reflect.String, nil

	case "rmdir.filename":

		return reflect.String, nil

	case "rmdir.inode":

		return reflect.Int, nil

	case "rmdir.overlay_num_lower":

		return reflect.Int, nil

	case "unlink.container_path":

		return reflect.String, nil

	case "unlink.filename":

		return reflect.String, nil

	case "unlink.flags":

		return reflect.Int, nil

	case "unlink.inode":

		return reflect.Int, nil

	case "unlink.overlay_num_lower":

		return reflect.Int, nil

	case "utimes.container_path":

		return reflect.String, nil

	case "utimes.filename":

		return reflect.String, nil

	case "utimes.inode":

		return reflect.Int, nil

	case "utimes.overlay_num_lower":

		return reflect.Int, nil

	}

	return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) SetFieldValue(field eval.Field, value interface{}) error {
	var ok bool
	switch field {

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
		e.Chmod.Mode = int32(v)
		return nil

	case "chmod.overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chmod.OverlayNumLower"}
		}
		e.Chmod.OverlayNumLower = int32(v)
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

	case "chown.overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Chown.OverlayNumLower"}
		}
		e.Chown.OverlayNumLower = int32(v)
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

	case "event.retval":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Event.Retval"}
		}
		e.Event.Retval = int64(v)
		return nil

	case "event.type":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Event.Type"}
		}
		e.Event.Type = uint64(v)
		return nil

	case "link.new_container_path":

		if e.Link.NewContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.NewContainerPath"}
		}
		return nil

	case "link.new_filename":

		if e.Link.NewPathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.NewPathnameStr"}
		}
		return nil

	case "link.new_inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.NewInode"}
		}
		e.Link.NewInode = uint64(v)
		return nil

	case "link.new_overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.NewOverlayNumLower"}
		}
		e.Link.NewOverlayNumLower = int32(v)
		return nil

	case "link.src_container_path":

		if e.Link.SrcContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.SrcContainerPath"}
		}
		return nil

	case "link.src_filename":

		if e.Link.SrcPathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.SrcPathnameStr"}
		}
		return nil

	case "link.src_inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.SrcInode"}
		}
		e.Link.SrcInode = uint64(v)
		return nil

	case "link.src_overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Link.SrcOverlayNumLower"}
		}
		e.Link.SrcOverlayNumLower = int32(v)
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

	case "mkdir.overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Mkdir.OverlayNumLower"}
		}
		e.Mkdir.OverlayNumLower = int32(v)
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

	case "open.overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Open.OverlayNumLower"}
		}
		e.Open.OverlayNumLower = int32(v)
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

	case "process.name":

		if e.Process.Comm, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Process.Comm"}
		}
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

	case "rename.new_filename":

		if e.Rename.TargetPathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.TargetPathnameStr"}
		}
		return nil

	case "rename.new_inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.TargetInode"}
		}
		e.Rename.TargetInode = uint64(v)
		return nil

	case "rename.old_filename":

		if e.Rename.SrcPathnameStr, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.SrcPathnameStr"}
		}
		return nil

	case "rename.old_inode":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.SrcInode"}
		}
		e.Rename.SrcInode = uint64(v)
		return nil

	case "rename.src_container_path":

		if e.Rename.SrcContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.SrcContainerPath"}
		}
		return nil

	case "rename.src_overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.SrcOverlayNumLower"}
		}
		e.Rename.SrcOverlayNumLower = int32(v)
		return nil

	case "rename.target_container_path":

		if e.Rename.TargetContainerPath, ok = value.(string); !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.TargetContainerPath"}
		}
		return nil

	case "rename.target_overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rename.TargetOverlayNumLower"}
		}
		e.Rename.TargetOverlayNumLower = int32(v)
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

	case "rmdir.overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Rmdir.OverlayNumLower"}
		}
		e.Rmdir.OverlayNumLower = int32(v)
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

	case "unlink.overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Unlink.OverlayNumLower"}
		}
		e.Unlink.OverlayNumLower = int32(v)
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

	case "utimes.overlay_num_lower":

		v, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Utimes.OverlayNumLower"}
		}
		e.Utimes.OverlayNumLower = int32(v)
		return nil

	}

	return &eval.ErrFieldNotFound{Field: field}
}
