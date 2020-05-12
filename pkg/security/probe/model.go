//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/accessors -output model_accessors.go

package probe

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

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

type OpenEvent struct {
	Flags       uint32 `yaml:"flags" field:"flags" event:"open"`
	Mode        uint32 `yaml:"mode" field:"mode" event:"open"`
	Inode       uint32 `field:"inode" event:"open"`
	PathnameKey uint32 `field:"filename" handler:"HandlePathnameKey,string" event:"open"`
	MountID     int32  `field:"mount_id" event:"open"`

	pathnameStr string `field:"-"`
}

func (e *OpenEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.HandlePathnameKey(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"mode":%d,`, e.Mode)
	fmt.Fprintf(&buf, `"flags":%d`, e.Flags)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *OpenEvent) HandlePathnameKey(resolvers *Resolvers) string {
	if len(e.pathnameStr) == 0 {
		e.pathnameStr = resolvers.DentryResolver.Resolve(e.PathnameKey)
	}
	return e.pathnameStr
}

func (e *OpenEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 20 {
		return 0, NotEnoughData
	}
	e.Flags = byteOrder.Uint32(data[0:4])
	e.Mode = byteOrder.Uint32(data[4:8])
	e.Inode = byteOrder.Uint32(data[8:12])
	e.PathnameKey = byteOrder.Uint32(data[12:16])
	e.MountID = int32(byteOrder.Uint32(data[16:20]))
	return 20, nil
}

type MkdirEvent struct {
	Inode       uint32 `field:"inode" event:"mkdir"`
	PathnameKey uint32 `field:"filename" handler:"HandlePathnameKey,string" event:"mkdir"`
	MountID     int32  `field:"mount_id" event:"mkdir"`
	Mode        int32  `field:"mode" event:"mkdir"`

	pathnameStr string `field:"-"`
}

func (e *MkdirEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.HandlePathnameKey(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"mode":%d`, e.Mode)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *MkdirEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, NotEnoughData
	}
	e.Inode = byteOrder.Uint32(data[0:4])
	e.PathnameKey = byteOrder.Uint32(data[4:8])
	e.MountID = int32(byteOrder.Uint32(data[8:12]))
	e.Mode = int32(byteOrder.Uint32(data[12:16]))
	return 16, nil
}

func (e *MkdirEvent) HandlePathnameKey(resolvers *Resolvers) string {
	if len(e.pathnameStr) == 0 {
		e.pathnameStr = resolvers.DentryResolver.Resolve(e.PathnameKey)
	}
	return e.pathnameStr
}

type RmdirEvent struct {
	Inode       uint32 `field:"inode" event:"rmdir"`
	PathnameKey uint32 `handler:"HandlePathnameKey,string" event:"rmdir"`
	MountID     int32  `field:"mount_id" event:"rmdir"`

	pathnameStr string `field:"-"`
}

func (e *RmdirEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.HandlePathnameKey(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d`, e.MountID)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *RmdirEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 12 {
		return 0, NotEnoughData
	}
	e.Inode = byteOrder.Uint32(data[0:4])
	e.PathnameKey = byteOrder.Uint32(data[4:8])
	e.MountID = int32(byteOrder.Uint32(data[8:12]))
	return 12, nil
}

func (e *RmdirEvent) HandlePathnameKey(resolvers *Resolvers) string {
	if len(e.pathnameStr) == 0 {
		e.pathnameStr = resolvers.DentryResolver.Resolve(e.PathnameKey)
	}
	return e.pathnameStr
}

type UnlinkEvent struct {
	Inode       uint32 `field:"inode" event:"unlink"`
	PathnameKey uint32 `field:"filename" handler:"HandlePathnameKey,string" event:"unlink"`
	MountID     int32  `field:"mount_id" event:"unlink"`

	pathnameStr string `json:"filename" field:"-"`
}

func (e *UnlinkEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.HandlePathnameKey(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d`, e.MountID)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *UnlinkEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 12 {
		return 0, NotEnoughData
	}
	e.Inode = byteOrder.Uint32(data[0:4])
	e.PathnameKey = byteOrder.Uint32(data[4:8])
	e.MountID = int32(byteOrder.Uint32(data[8:12]))
	return 12, nil
}

func (e *UnlinkEvent) HandlePathnameKey(resolvers *Resolvers) string {
	if len(e.pathnameStr) == 0 {
		e.pathnameStr = resolvers.DentryResolver.Resolve(e.PathnameKey)
	}
	return e.pathnameStr
}

type RenameEvent struct {
	SrcInode          uint32 `json:"oldinode,omitempty" field:"oldinode" event:"rename"`
	SrcPathnameKey    uint32 `json:"-" field:"oldfilename" handler:"HandleSrcPathnameKey,string" event:"rename"`
	SrcMountID        int32  `json:"oldmountid,omitempty" field:"oldmountid" event:"rename"`
	TargetInode       uint32 `json:"newinode,omitempty" field:"newinode" event:"rename"`
	TargetPathnameKey uint32 `json:"-" field:"newfilename" handler:"HandleTargetPathnameKey,string" event:"rename"`
	TargetMountID     int32  `json:"newmountid,omitempty" field:"newmountid" event:"rename"`

	srcPathnameStr    string `json:"oldfilename" field:"-"`
	targetPathnameStr string `json:"newfilename" field:"-"`
}

func (e *RenameEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if e.SrcInode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"old_inode":%d,`, e.SrcInode)
	fmt.Fprintf(&buf, `"old_filename":"%s",`, e.HandleSrcPathnameKey(resolvers))
	fmt.Fprintf(&buf, `"old_mount_id":%d,`, e.SrcMountID)
	fmt.Fprintf(&buf, `"new_inode":%d,`, e.TargetInode)
	fmt.Fprintf(&buf, `"new_filename":"%s",`, e.HandleTargetPathnameKey(resolvers))
	fmt.Fprintf(&buf, `"new_mount_id":%d`, e.TargetMountID)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, NotEnoughData
	}
	e.SrcInode = byteOrder.Uint32(data[0:4])
	e.SrcPathnameKey = byteOrder.Uint32(data[4:8])
	e.SrcMountID = int32(byteOrder.Uint32(data[8:12]))
	e.TargetInode = byteOrder.Uint32(data[12:16])
	e.TargetPathnameKey = byteOrder.Uint32(data[16:20])
	e.TargetMountID = int32(byteOrder.Uint32(data[20:24]))
	return 24, nil
}

func (e *RenameEvent) HandleSrcPathnameKey(resolvers *Resolvers) string {
	if len(e.srcPathnameStr) == 0 {
		e.srcPathnameStr = resolvers.DentryResolver.Resolve(e.SrcPathnameKey)
	}
	return e.srcPathnameStr
}

func (e *RenameEvent) HandleTargetPathnameKey(resolvers *Resolvers) string {
	if len(e.targetPathnameStr) == 0 {
		e.targetPathnameStr = resolvers.DentryResolver.Resolve(e.TargetPathnameKey)
	}
	return e.targetPathnameStr
}

type ContainerEvent struct {
	ID     string   `yaml:"id" field:"id" event:"container"`
	Labels []string `yaml:"labels" field:"labels" event:"container"`
}

type KernelEvent struct {
	Type      uint64 `field:"type"`
	Timestamp uint64 `field:"-"`
	Retval    int64  `field:"retval"`
}

func (k *KernelEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if k.Type == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"type":%d,`, k.Type)
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
	Pidns   uint64   `field:"pidns"`
	Comm    [16]byte `field:"name" handler:"HandleComm,string"`
	TTYName [64]byte `field:"tty_name" handler:"HandleTTY,string"`
	Pid     uint32   `field:"pid"`
	Tid     uint32   `field:"tid"`
	UID     uint32   `field:"uid"`
	GID     uint32   `field:"gid"`

	commStr    string `json:"" field:"-"`
	ttyNameStr string `json:"tty" field:"-"`
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

func (p *ProcessEvent) HandleTTY(resolvers *Resolvers) string {
	return p.GetTTY()
}

func (p *ProcessEvent) GetTTY() string {
	if len(p.ttyNameStr) == 0 {
		p.ttyNameStr = string(bytes.Trim(p.TTYName[:], "\x00"))
	}
	return p.ttyNameStr
}

func (p *ProcessEvent) HandleComm(resolvers *Resolvers) string {
	return p.GetComm()
}

func (p *ProcessEvent) GetComm() string {
	if len(p.commStr) == 0 {
		p.commStr = string(bytes.Trim(p.Comm[:], "\x00"))
	}
	return p.commStr
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
