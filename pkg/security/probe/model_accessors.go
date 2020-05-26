// Code generated - DO NOT EDIT.

package probe

import (
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

var (
	ErrFieldNotFound  = errors.New("field not found")
	ErrWrongValueType = errors.New("wrong value type")
)

func (m *Model) GetEvaluator(key string) (interface{}, error) {
	switch key {

	case "container.id":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Container.ID },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Container.ID },

			Field: key,
		}, nil

	case "event.retval":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Event.Retval) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Event.Retval) },

			Field: key,
		}, nil

	case "event.type":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Event.ResolveType(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Event.ResolveType(m.event.resolvers) },

			Field: key,
		}, nil

	case "mkdir.filename":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Mkdir.ResolveInode(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Mkdir.ResolveInode(m.event.resolvers) },

			Field: key,
		}, nil

	case "mkdir.inode":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Mkdir.Inode) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Mkdir.Inode) },

			Field: key,
		}, nil

	case "mkdir.mode":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Mkdir.Mode) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Mkdir.Mode) },

			Field: key,
		}, nil

	case "open.basename":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Open.ResolveBasename(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Open.ResolveBasename(m.event.resolvers) },

			Field: key,
		}, nil

	case "open.filename":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Open.ResolveInode(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Open.ResolveInode(m.event.resolvers) },

			Field: key,
		}, nil

	case "open.flags":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Open.Flags) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Open.Flags) },

			Field: key,
		}, nil

	case "open.inode":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Open.Inode) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Open.Inode) },

			Field: key,
		}, nil

	case "open.mode":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Open.Mode) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Open.Mode) },

			Field: key,
		}, nil

	case "process.gid":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Process.GID) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Process.GID) },

			Field: key,
		}, nil

	case "process.name":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Process.ResolveComm(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Process.ResolveComm(m.event.resolvers) },

			Field: key,
		}, nil

	case "process.pid":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Process.Pid) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Process.Pid) },

			Field: key,
		}, nil

	case "process.pidns":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Process.Pidns) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Process.Pidns) },

			Field: key,
		}, nil

	case "process.tid":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Process.Tid) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Process.Tid) },

			Field: key,
		}, nil

	case "process.tty_name":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Process.TTYName },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Process.TTYName },

			Field: key,
		}, nil

	case "process.uid":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Process.UID) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Process.UID) },

			Field: key,
		}, nil

	case "rename.new_filename":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Rename.ResolveTargetInode(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Rename.ResolveTargetInode(m.event.resolvers) },

			Field: key,
		}, nil

	case "rename.new_inode":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Rename.TargetInode) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Rename.TargetInode) },

			Field: key,
		}, nil

	case "rename.old_filename":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Rename.ResolveSrcInode(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Rename.ResolveSrcInode(m.event.resolvers) },

			Field: key,
		}, nil

	case "rename.old_inode":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Rename.SrcInode) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Rename.SrcInode) },

			Field: key,
		}, nil

	case "rmdir.filename":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Rmdir.ResolveInode(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Rmdir.ResolveInode(m.event.resolvers) },

			Field: key,
		}, nil

	case "rmdir.inode":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Rmdir.Inode) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Rmdir.Inode) },

			Field: key,
		}, nil

	case "unlink.filename":

		return &eval.StringEvaluator{
			EvalFnc:      func(ctx *eval.Context) string { return m.event.Unlink.ResolveInode(m.event.resolvers) },
			DebugEvalFnc: func(ctx *eval.Context) string { return m.event.Unlink.ResolveInode(m.event.resolvers) },

			Field: key,
		}, nil

	case "unlink.inode":

		return &eval.IntEvaluator{
			EvalFnc:      func(ctx *eval.Context) int { return int(m.event.Unlink.Inode) },
			DebugEvalFnc: func(ctx *eval.Context) int { return int(m.event.Unlink.Inode) },

			Field: key,
		}, nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}

func (e *Event) GetFieldValue(key string) (interface{}, error) {
	switch key {

	case "container.id":

		return e.Container.ID, nil

	case "event.retval":

		return int(e.Event.Retval), nil

	case "event.type":

		return e.Event.ResolveType(e.resolvers), nil

	case "mkdir.filename":

		return e.Mkdir.ResolveInode(e.resolvers), nil

	case "mkdir.inode":

		return int(e.Mkdir.Inode), nil

	case "mkdir.mode":

		return int(e.Mkdir.Mode), nil

	case "open.filename":

		return e.Open.ResolveInode(e.resolvers), nil

	case "open.flags":

		return int(e.Open.Flags), nil

	case "open.inode":

		return int(e.Open.Inode), nil

	case "open.mode":

		return int(e.Open.Mode), nil

	case "process.gid":

		return int(e.Process.GID), nil

	case "process.name":

		return e.Process.ResolveComm(e.resolvers), nil

	case "process.pid":

		return int(e.Process.Pid), nil

	case "process.pidns":

		return int(e.Process.Pidns), nil

	case "process.tid":

		return int(e.Process.Tid), nil

	case "process.tty_name":

		return e.Process.TTYName, nil

	case "process.uid":

		return int(e.Process.UID), nil

	case "rename.newfilename":

		return e.Rename.ResolveTargetInode(e.resolvers), nil

	case "rename.newinode":

		return int(e.Rename.TargetInode), nil

	case "rename.oldfilename":

		return e.Rename.ResolveSrcInode(e.resolvers), nil

	case "rename.oldinode":

		return int(e.Rename.SrcInode), nil

	case "rmdir.filename":

		return e.Rmdir.ResolveInode(e.resolvers), nil

	case "rmdir.inode":

		return int(e.Rmdir.Inode), nil

	case "unlink.filename":

		return e.Unlink.ResolveInode(e.resolvers), nil

	case "unlink.inode":

		return int(e.Unlink.Inode), nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}

func (e *Event) GetFieldTags(key string) ([]string, error) {
	switch key {

	case "container.id":
		return []string{}, nil

	case "event.retval":
		return []string{}, nil

	case "event.type":
		return []string{}, nil

	case "mkdir.filename":
		return []string{}, nil

	case "mkdir.inode":
		return []string{}, nil

	case "mkdir.mode":
		return []string{}, nil

	case "open.basename":
		return []string{}, nil

	case "open.filename":
		return []string{}, nil

	case "open.flags":
		return []string{}, nil

	case "open.inode":
		return []string{}, nil

	case "open.mode":
		return []string{}, nil

	case "process.gid":
		return []string{}, nil

	case "process.name":
		return []string{}, nil

	case "process.pid":
		return []string{}, nil

	case "process.pidns":
		return []string{}, nil

	case "process.tid":
		return []string{}, nil

	case "process.tty_name":
		return []string{}, nil

	case "process.uid":
		return []string{}, nil

	case "rename.new_filename":
		return []string{}, nil

	case "rename.new_inode":
		return []string{}, nil

	case "rename.old_filename":
		return []string{}, nil

	case "rename.old_inode":
		return []string{}, nil

	case "rmdir.filename":
		return []string{}, nil

	case "rmdir.inode":
		return []string{}, nil

	case "unlink.filename":
		return []string{}, nil

	case "unlink.inode":
		return []string{}, nil

	}

	return nil, errors.Wrap(ErrFieldNotFound, key)
}

func (e *Event) GetFieldEventType(key string) (string, error) {
	switch key {

	case "container.id":
		return "container", nil

	case "event.retval":
		return "", nil

	case "event.type":
		return "", nil

	case "mkdir.filename":
		return "mkdir", nil

	case "mkdir.inode":
		return "mkdir", nil

	case "mkdir.mode":
		return "mkdir", nil

	case "open.basename":
		return "open", nil

	case "open.filename":
		return "open", nil

	case "open.flags":
		return "open", nil

	case "open.inode":
		return "open", nil

	case "open.mode":
		return "open", nil

	case "process.gid":
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
		return "", nil

	case "process.uid":
		return "*", nil

	case "rename.new_filename":
		return "rename", nil

	case "rename.new_inode":
		return "rename", nil

	case "rename.old_filename":
		return "rename", nil

	case "rename.old_inode":
		return "rename", nil

	case "rmdir.filename":
		return "rmdir", nil

	case "rmdir.inode":
		return "rmdir", nil

	case "unlink.filename":
		return "unlink", nil

	case "unlink.inode":
		return "unlink", nil

	}

	return "", errors.Wrap(ErrFieldNotFound, key)
}

func (e *Event) SetFieldValue(key string, value interface{}) error {
	var ok bool
	switch key {

	case "container.id":

		if e.Container.ID, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "event.retval":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Event.Retval = int64(v)
		return nil

	case "event.type":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Event.Type = uint64(v)
		return nil

	case "mkdir.filename":

		if e.Mkdir.PathnameStr, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "mkdir.inode":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Mkdir.Inode = uint64(v)
		return nil

	case "mkdir.mode":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Mkdir.Mode = int32(v)
		return nil

	case "open.basename":

		if m.event.Open.BasenameStr, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "open.filename":

		if e.Open.PathnameStr, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "open.flags":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Open.Flags = uint32(v)
		return nil

	case "open.inode":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Open.Inode = uint64(v)
		return nil

	case "open.mode":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Open.Mode = uint32(v)
		return nil

	case "process.gid":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Process.GID = uint32(v)
		return nil

	case "process.name":

		if e.Process.Comm, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "process.pid":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Process.Pid = uint32(v)
		return nil

	case "process.pidns":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Process.Pidns = uint64(v)
		return nil

	case "process.tid":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Process.Tid = uint32(v)
		return nil

	case "process.tty_name":

		if e.Process.TTYName, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "process.uid":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Process.UID = uint32(v)
		return nil

	case "rename.newfilename":

		if e.Rename.TargetPathnameStr, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "rename.newinode":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Rename.TargetInode = uint64(v)
		return nil

	case "rename.oldfilename":

		if e.Rename.SrcPathnameStr, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "rename.oldinode":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Rename.SrcInode = uint64(v)
		return nil

	case "rmdir.filename":

		if e.Rmdir.PathnameStr, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "rmdir.inode":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Rmdir.Inode = uint64(v)
		return nil

	case "unlink.filename":

		if e.Unlink.PathnameStr, ok = value.(string); !ok {
			return ErrWrongValueType
		}
		return nil

	case "unlink.inode":

		v, ok := value.(int)
		if !ok {
			return ErrWrongValueType
		}
		e.Unlink.Inode = uint64(v)
		return nil

	}

	return errors.Wrap(ErrFieldNotFound, key)
}
