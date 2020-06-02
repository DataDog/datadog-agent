//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/accessors -output model_accessors.go

package probe

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var NotEnoughData = errors.New("not enough data")

type Model struct {
	event *Event
}

func (m *Model) SetEvent(event interface{}) {
	m.event = event.(*Event)
}

func (m *Model) GetEvent() eval.Event {
	return m.event
}

type OpenEvent struct {
	Flags       uint32 `yaml:"flags" field:"flags" event:"open"`
	Mode        uint32 `yaml:"mode" field:"mode" event:"open"`
	Inode       uint64 `field:"inode" event:"open"`
	Dev         uint32 `field:"-"`
	PathnameStr string `field:"filename" handler:"ResolveInode,string" event:"open"`
	BasenameStr string `field:"basename" handler:"ResolveBasename,string" event:"open"`
}

func (e *OpenEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mode":%d,`, e.Mode)
	fmt.Fprintf(&buf, `"flags":%d`, e.Flags)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *OpenEvent) ResolveInode(resolvers *Resolvers) string {
	if len(e.PathnameStr) == 0 {
		e.PathnameStr = resolvers.DentryResolver.Resolve(e.Dev, e.Inode)
	}
	return e.PathnameStr
}

func (e *OpenEvent) ResolveBasename(resolvers *Resolvers) string {
	if len(e.BasenameStr) == 0 {
		e.BasenameStr = resolvers.DentryResolver.GetName(e.Dev, e.Inode)
	}
	return e.BasenameStr
}

func (e *OpenEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 20 {
		return 0, NotEnoughData
	}
	e.Flags = byteOrder.Uint32(data[0:4])
	e.Mode = byteOrder.Uint32(data[4:8])
	e.Inode = byteOrder.Uint64(data[8:16])
	e.Dev = byteOrder.Uint32(data[16:20])
	return 20, nil
}

type MkdirEvent struct {
	Mode        int32  `field:"mode" event:"mkdir"`
	Dev         uint32 `field:"-"`
	Inode       uint64 `field:"inode" event:"mkdir"`
	PathnameStr string `field:"filename" handler:"ResolveInode,string" event:"mkdir"`
}

func (e *MkdirEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mode":%d`, e.Mode)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *MkdirEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, NotEnoughData
	}
	e.Mode = int32(byteOrder.Uint32(data[0:4]))
	e.Dev = byteOrder.Uint32(data[4:8])
	e.Inode = byteOrder.Uint64(data[8:16])
	return 16, nil
}

func (e *MkdirEvent) ResolveInode(resolvers *Resolvers) string {
	if len(e.PathnameStr) == 0 {
		e.PathnameStr = resolvers.DentryResolver.Resolve(e.Dev, e.Inode)
	}
	return e.PathnameStr
}

type RmdirEvent struct {
	Dev         uint32 `field:"-"`
	Inode       uint64 `field:"inode" event:"rmdir"`
	PathnameStr string `field:"filename" handler:"ResolveInode,string" event:"rmdir"`
}

func (e *RmdirEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"inode":%d`, e.Inode)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *RmdirEvent) ResolveInode(resolvers *Resolvers) string {
	if len(e.PathnameStr) == 0 {
		e.PathnameStr = resolvers.DentryResolver.Resolve(e.Dev, e.Inode)
	}
	return e.PathnameStr
}

func (e *RmdirEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 12 {
		return 0, NotEnoughData
	}
	e.Inode = byteOrder.Uint64(data[0:8])
	e.Dev = byteOrder.Uint32(data[8:12])
	return 12, nil
}

type UnlinkEvent struct {
	Inode       uint64 `field:"inode" event:"unlink"`
	Dev         uint32 `field:"-"`
	PathnameStr string `field:"filename" handler:"ResolveInode,string" event:"unlink"`
}

func (e *UnlinkEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"inode":%d`, e.Inode)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *UnlinkEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 12 {
		return 0, NotEnoughData
	}
	e.Inode = byteOrder.Uint64(data[0:8])
	e.Dev = byteOrder.Uint32(data[8:12])
	return 12, nil
}

func (e *UnlinkEvent) ResolveInode(resolvers *Resolvers) string {
	if len(e.PathnameStr) == 0 {
		e.PathnameStr = resolvers.DentryResolver.Resolve(e.Dev, e.Inode)
	}
	return e.PathnameStr
}

type RenameEvent struct {
	Dev               uint32 `field:"-"`
	SrcInode          uint64 `json:"oldinode,omitempty" field:"oldinode" event:"rename"`
	SrcPathnameStr    string `json:"-" field:"oldfilename" handler:"ResolveSrcInode,string" event:"rename"`
	TargetInode       uint64 `json:"newinode,omitempty" field:"newinode" event:"rename"`
	TargetPathnameStr string `json:"-" field:"newfilename" handler:"ResolveTargetInode,string" event:"rename"`
}

func (e *RenameEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.SrcInode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"old_inode":%d,`, e.SrcInode)
	fmt.Fprintf(&buf, `"old_filename":"%s",`, e.ResolveSrcInode(resolvers))
	fmt.Fprintf(&buf, `"new_inode":%d,`, e.TargetInode)
	fmt.Fprintf(&buf, `"new_filename":"%s"`, e.ResolveTargetInode(resolvers))
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, NotEnoughData
	}
	e.Dev = byteOrder.Uint32(data[0:4])
	// padding
	e.SrcInode = byteOrder.Uint64(data[8:16])
	e.TargetInode = byteOrder.Uint64(data[16:24])
	return 24, nil
}

func (e *RenameEvent) ResolveSrcInode(resolvers *Resolvers) string {
	if len(e.SrcPathnameStr) == 0 {
		e.SrcPathnameStr = resolvers.DentryResolver.Resolve(0xffffffff, e.SrcInode)
	}
	return e.SrcPathnameStr
}

func (e *RenameEvent) ResolveTargetInode(resolvers *Resolvers) string {
	if len(e.TargetPathnameStr) == 0 {
		e.TargetPathnameStr = resolvers.DentryResolver.Resolve(e.Dev, e.TargetInode)
	}
	return e.TargetPathnameStr
}

type ContainerEvent struct {
	ID     string   `yaml:"id" field:"id" event:"container"`
	Labels []string `yaml:"labels" field:"labels" event:"container"`
}

type KernelEvent struct {
	Type      uint64 `field:"type" handler:"ResolveType,string"`
	Timestamp uint64 `field:"-"`
	Retval    int64  `field:"retval"`
}

func (k *KernelEvent) ResolveType(resolvers *Resolvers) string {
	return ProbeEventType(k.Type).String()
}

func (k *KernelEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if k.Type == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"type":%d,`, k.Type) // TODO(sbaubeau): use resolved type
	fmt.Fprintf(&buf, `"timestamp":%d,`, k.Timestamp)
	fmt.Fprintf(&buf, `"retval":%d`, k.Retval)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (k *KernelEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, NotEnoughData
	}
	k.Type = byteOrder.Uint64(data[0:8])
	k.Timestamp = byteOrder.Uint64(data[8:16])
	k.Retval = int64(byteOrder.Uint64(data[16:24]))
	return 24, nil
}

type ProcessEvent struct {
	Pidns   uint64 `field:"pidns" event:"*"`
	Comm    string `field:"name" handler:"ResolveComm,string" event:"*"`
	TTYName string `field:"tty_name" handler:ResolveTTY",string" event:"*"`
	Pid     uint32 `field:"pid" event:"*"`
	Tid     uint32 `field:"tid" event:"*"`
	UID     uint32 `field:"uid" event:"*"`
	GID     uint32 `field:"gid" event:"*"`

	CommRaw    [16]byte `field:"-"`
	TTYNameRaw [64]byte `field:"-"`
}

func (p *ProcessEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if p.Pid == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"pidns":%d,`, p.Pidns)
	fmt.Fprintf(&buf, `"name":"%s",`, p.GetComm())
	if tty := p.GetTTY(); tty != "" {
		fmt.Fprintf(&buf, `"tty_name":"%s",`, tty)
	}
	fmt.Fprintf(&buf, `"pid":%d,`, p.Pid)
	fmt.Fprintf(&buf, `"tid":%d,`, p.Tid)
	fmt.Fprintf(&buf, `"uid":%d,`, p.UID)
	fmt.Fprintf(&buf, `"gid":%d`, p.GID)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (p *ProcessEvent) ResolveTTY(resolvers *Resolvers) string {
	return p.GetTTY()
}

func (p *ProcessEvent) GetTTY() string {
	if len(p.TTYName) == 0 {
		p.TTYName = string(bytes.Trim(p.TTYNameRaw[:], "\x00"))
	}
	return p.TTYName
}

func (p *ProcessEvent) ResolveComm(resolvers *Resolvers) string {
	return p.GetComm()
}

func (p *ProcessEvent) GetComm() string {
	if len(p.Comm) == 0 {
		p.Comm = string(bytes.Trim(p.CommRaw[:], "\x00"))
	}
	return p.Comm
}

func (p *ProcessEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 104 {
		return 0, NotEnoughData
	}
	p.Pidns = byteOrder.Uint64(data[0:8])
	binary.Read(bytes.NewBuffer(data[8:24]), byteOrder, &p.Comm)
	binary.Read(bytes.NewBuffer(data[24:88]), byteOrder, &p.TTYName)
	p.Pid = byteOrder.Uint32(data[88:92])
	p.Tid = byteOrder.Uint32(data[92:96])
	p.UID = byteOrder.Uint32(data[96:100])
	p.GID = byteOrder.Uint32(data[100:104])
	return 104, nil
}

// genaccessors
type Event struct {
	ID        string         `yaml:"id" field:"-"`
	Event     KernelEvent    `yaml:"event" field:"event"`
	Process   ProcessEvent   `yaml:"process" field:"process"`
	Open      OpenEvent      `yaml:"open" field:"open"`
	Mkdir     MkdirEvent     `yaml:"mkdir" field:"mkdir"`
	Rmdir     RmdirEvent     `yaml:"rmdir" field:"rmdir"`
	Unlink    UnlinkEvent    `yaml:"unlink" field:"unlink"`
	Rename    RenameEvent    `yaml:"rename" field:"rename"`
	Container ContainerEvent `yaml:"container" field:"container"`

	resolvers *Resolvers `field:"-"`
}

func (e *Event) String() string {
	d, _ := json.Marshal(e)
	return string(d)
}

func (e *Event) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"id":"%s",`, e.ID)

	entries := []struct {
		field      string
		marshalFnc func(resolvers *Resolvers) ([]byte, error)
	}{
		{
			field:      "event",
			marshalFnc: e.Event.marshalJSON,
		},
		{
			field:      "process",
			marshalFnc: e.Process.marshalJSON,
		},
		{
			field:      "open",
			marshalFnc: e.Open.marshalJSON,
		},
		{
			field:      "mkdir",
			marshalFnc: e.Mkdir.marshalJSON,
		},
		{
			field:      "rmdir",
			marshalFnc: e.Rmdir.marshalJSON,
		},
		{
			field:      "unlink",
			marshalFnc: e.Unlink.marshalJSON,
		},
		{
			field:      "rename",
			marshalFnc: e.Rename.marshalJSON,
		},
	}

	var prev bool
	for _, entry := range entries {
		d, err := entry.marshalFnc(e.resolvers)
		if err != nil {
			return nil, err
		}
		if d != nil {
			if prev {
				buf.WriteRune(',')
			}
			buf.WriteString(`"` + entry.field + `":`)
			buf.Write(d)
			prev = true
		}
	}
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *Event) GetType() string {
	return ProbeEventType(e.Event.Type).String()
}

func (e *Event) GetID() string {
	return e.ID
}

func (e *Event) UnmarshalBinary(data []byte) (int, error) {
	offset, err := e.Process.UnmarshalBinary(data)
	if err != nil {
		return offset, err
	}

	return offset, nil
}

func NewEvent(resolvers *Resolvers) *Event {
	id, _ := uuid.NewRandom()
	return &Event{
		ID:        id.String(),
		resolvers: resolvers,
	}
}
