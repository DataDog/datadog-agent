// Code generated - DO NOT EDIT.

package probe

import (
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

var (
	ErrFieldNotFound = errors.New("field not found")
)

func (m *Model) GetEvaluator(key string) (interface{}, error) {
	switch key {

	case "container.id":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Container.ID },
			DebugEval: func(ctx *eval.Context) string { return m.event.Container.ID },

			Field: key,
		}, nil

	case "event.retval":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Event.Retval) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Event.Retval) },

			Field: key,
		}, nil

	case "event.type":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Event.Type) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Event.Type) },

			Field: key,
		}, nil

	case "mkdir.filename":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Mkdir.HandlePathnameKey(m.event.resolvers) },
			DebugEval: func(ctx *eval.Context) string { return m.event.Mkdir.HandlePathnameKey(m.event.resolvers) },

			Field: key,
		}, nil

	case "mkdir.inode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.Inode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.Inode) },

			Field: key,
		}, nil

	case "mkdir.mode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.Mode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.Mode) },

			Field: key,
		}, nil

	case "mkdir.mount_id":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.MountID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.MountID) },

			Field: key,
		}, nil

	case "open.filename":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Open.ResolvePathnameKey(m.event.resolvers) },
			DebugEval: func(ctx *eval.Context) string { return m.event.Open.ResolvePathnameKey(m.event.resolvers) },

			Field: key,
		}, nil

	case "open.flags":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Open.Flags) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Open.Flags) },

			Field: key,
		}, nil

	case "open.inode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Open.Inode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Open.Inode) },

			Field: key,
		}, nil

	case "open.mode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Open.Mode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Open.Mode) },

			Field: key,
		}, nil

	case "open.mount_id":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Open.MountID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Open.MountID) },

			Field: key,
		}, nil

	case "process.gid":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Process.GID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Process.GID) },

			Field: key,
		}, nil

	case "process.name":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Process.HandleComm(m.event.resolvers) },
			DebugEval: func(ctx *eval.Context) string { return m.event.Process.HandleComm(m.event.resolvers) },

			Field: key,
		}, nil

	case "process.pid":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Process.Pid) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Process.Pid) },

			Field: key,
		}, nil

	case "process.pidns":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Process.Pidns) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Process.Pidns) },

			Field: key,
		}, nil

	case "process.tid":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Process.Tid) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Process.Tid) },

			Field: key,
		}, nil

	case "process.tty_name":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Process.HandleTTY(m.event.resolvers) },
			DebugEval: func(ctx *eval.Context) string { return m.event.Process.HandleTTY(m.event.resolvers) },

			Field: key,
		}, nil

	case "process.uid":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Process.UID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Process.UID) },

			Field: key,
		}, nil

	case "rename.newfilename":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rename.TargetPathnameKey) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rename.TargetPathnameKey) },

			Field: key,
		}, nil

	case "rename.newinode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rename.TargetInode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rename.TargetInode) },

			Field: key,
		}, nil

	case "rename.newmountid":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rename.TargetMountID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rename.TargetMountID) },

			Field: key,
		}, nil

	case "rename.oldfilename":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rename.SrcPathnameKey) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rename.SrcPathnameKey) },

			Field: key,
		}, nil

	case "rename.oldinode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rename.SrcInode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rename.SrcInode) },

			Field: key,
		}, nil

	case "rename.oldmountid":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rename.SrcMountID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rename.SrcMountID) },

			Field: key,
		}, nil

	case "rmdir.filename":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rmdir.PathnameKey) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rmdir.PathnameKey) },

			Field: key,
		}, nil

	case "rmdir.inode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rmdir.Inode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rmdir.Inode) },

			Field: key,
		}, nil

	case "rmdir.mount_id":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Rmdir.MountID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Rmdir.MountID) },

			Field: key,
		}, nil

	case "unlink.filename":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Unlink.PathnameKey) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Unlink.PathnameKey) },

			Field: key,
		}, nil

	case "unlink.inode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Unlink.Inode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Unlink.Inode) },

			Field: key,
		}, nil

	case "unlink.mount_id":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Unlink.MountID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Unlink.MountID) },

			Field: key,
		}, nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}

func (m *Model) GetTags(key string) ([]string, error) {
	switch key {

	case "container.id":
		return []string{"container"}, nil

	case "event.retval":
		return []string{}, nil

	case "event.type":
		return []string{}, nil

	case "mkdir.filename":
		return []string{"fs"}, nil

	case "mkdir.inode":
		return []string{"fs"}, nil

	case "mkdir.mode":
		return []string{"fs"}, nil

	case "mkdir.mount_id":
		return []string{"fs"}, nil

	case "open.filename":
		return []string{"fs"}, nil

	case "open.flags":
		return []string{"fs"}, nil

	case "open.inode":
		return []string{"fs"}, nil

	case "open.mode":
		return []string{"fs"}, nil

	case "open.mount_id":
		return []string{"fs"}, nil

	case "process.gid":
		return []string{"process"}, nil

	case "process.name":
		return []string{"process"}, nil

	case "process.pid":
		return []string{"process"}, nil

	case "process.pidns":
		return []string{"process"}, nil

	case "process.tid":
		return []string{"process"}, nil

	case "process.tty_name":
		return []string{"process"}, nil

	case "process.uid":
		return []string{"process"}, nil

	case "rename.newfilename":
		return []string{"fs"}, nil

	case "rename.newinode":
		return []string{"fs"}, nil

	case "rename.newmountid":
		return []string{"fs"}, nil

	case "rename.oldfilename":
		return []string{"fs"}, nil

	case "rename.oldinode":
		return []string{"fs"}, nil

	case "rename.oldmountid":
		return []string{"fs"}, nil

	case "rmdir.filename":
		return []string{"fs"}, nil

	case "rmdir.inode":
		return []string{"fs"}, nil

	case "rmdir.mount_id":
		return []string{"fs"}, nil

	case "unlink.filename":
		return []string{"fs"}, nil

	case "unlink.inode":
		return []string{"fs"}, nil

	case "unlink.mount_id":
		return []string{"fs"}, nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}
