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
			Eval:      func(ctx *eval.Context) string { return m.event.Mkdir.Resolve(m) },
			DebugEval: func(ctx *eval.Context) string { return m.event.Mkdir.Resolve(m) },

			Field: key,
		}, nil

	case "mkdir.flags":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.Flags) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.Flags) },

			Field: key,
		}, nil

	case "mkdir.mode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.Mode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.Mode) },

			Field: key,
		}, nil

	case "mkdir.source_inode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.SrcInode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.SrcInode) },

			Field: key,
		}, nil

	case "mkdir.source_mount_id":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.SrcMountID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.SrcMountID) },

			Field: key,
		}, nil

	case "mkdir.target_inode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.TargetInode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.TargetInode) },

			Field: key,
		}, nil

	case "mkdir.target_mount_id":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Mkdir.TargetMountID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Mkdir.TargetMountID) },

			Field: key,
		}, nil

	case "open.filename":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Open.Filename },
			DebugEval: func(ctx *eval.Context) string { return m.event.Open.Filename },

			Field: key,
		}, nil

	case "open.flags":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Open.Flags) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Open.Flags) },

			Field: key,
		}, nil

	case "open.mode":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Open.Mode) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Open.Mode) },

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
			Eval:      func(ctx *eval.Context) string { return m.event.Process.GetComm() },
			DebugEval: func(ctx *eval.Context) string { return m.event.Process.GetComm() },

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
			Eval:      func(ctx *eval.Context) string { return m.event.Process.GetTTY() },
			DebugEval: func(ctx *eval.Context) string { return m.event.Process.GetTTY() },

			Field: key,
		}, nil

	case "process.uid":

		return &eval.IntEvaluator{
			Eval:      func(ctx *eval.Context) int { return int(m.event.Process.UID) },
			DebugEval: func(ctx *eval.Context) int { return int(m.event.Process.UID) },

			Field: key,
		}, nil

	case "rename.newname":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Rename.NewName },
			DebugEval: func(ctx *eval.Context) string { return m.event.Rename.NewName },

			Field: key,
		}, nil

	case "rename.oldname":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Rename.OldName },
			DebugEval: func(ctx *eval.Context) string { return m.event.Rename.OldName },

			Field: key,
		}, nil

	case "unlink.filename":

		return &eval.StringEvaluator{
			Eval:      func(ctx *eval.Context) string { return m.event.Unlink.Filename },
			DebugEval: func(ctx *eval.Context) string { return m.event.Unlink.Filename },

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

	case "mkdir.flags":
		return []string{"fs"}, nil

	case "mkdir.mode":
		return []string{"fs"}, nil

	case "mkdir.source_inode":
		return []string{"fs"}, nil

	case "mkdir.source_mount_id":
		return []string{"fs"}, nil

	case "mkdir.target_inode":
		return []string{"fs"}, nil

	case "mkdir.target_mount_id":
		return []string{"fs"}, nil

	case "open.filename":
		return []string{"fs"}, nil

	case "open.flags":
		return []string{"fs"}, nil

	case "open.mode":
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

	case "rename.newname":
		return []string{"fs"}, nil

	case "rename.oldname":
		return []string{"fs"}, nil

	case "unlink.filename":
		return []string{"fs"}, nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}
