//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/accessors -output model_accessors.go

package probe

import (
	"bytes"
	"encoding/binary"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var NotEnoughData = errors.New("not enough data")

type Model struct {
	event *Event
}

func (m *Model) SetData(data interface{}) {
	m.event = data.(*Event)
}

type OpenEvent struct {
	Flags       uint32 `yaml:"flags" field:"flags" tags:"fs"`
	Mode        uint32 `yaml:"mode" field:"mode" tags:"fs"`
	Inode       uint32 `json:"inode,omitempty" field:"inode" tags:"fs"`
	PathnameKey uint32 `json:"-" field:"filename" handler:"ResolvePathnameKey,string" tags:"fs"`
	PathnameStr string `json:"filename" field:"-"`
	MountID     int32  `json:"mount_id,omitempty" field:"mount_id" tags:"fs"`
}

func (e *OpenEvent) ResolvePathnameKey(resolvers *Resolvers) string {
	if len(e.PathnameStr) == 0 {
		e.PathnameStr = resolvers.DentryResolver.Resolve(e.PathnameKey)
	}
	return e.PathnameStr
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
	Inode       uint32 `json:"inode,omitempty" field:"inode" tags:"fs"`
	PathnameKey uint32 `json:"-" field:"filename" handler:"HandlePathnameKey,string" tags:"fs"`
	PathnameStr string `json:"filename" field:"-"`
	MountID     int32  `json:"mount_id,omitempty" field:"mount_id" tags:"fs"`
	Mode        int32  `json:"mode,omitempty" field:"mode" tags:"fs"`
}

/*func (e *MkdirEvent) MarshalJSON() ([]byte, error) {
	if e.Inode == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename": %d,`, e.)
	fmt.Fprintf(&buf, `"inode": %d`, e.Inode)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}*/

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
	if len(e.PathnameStr) == 0 {
		e.PathnameStr = resolvers.DentryResolver.Resolve(e.PathnameKey)
	}
	return e.PathnameStr
}

type RmdirEvent struct {
	Inode       uint32 `json:"inode,omitempty" field:"inode" tags:"fs"`
	PathnameKey uint32 `json:"-" field:"filename,string,m.dentryResolver.Resolve({{.FieldPrefix}}{{.Field}})" tags:"fs"`
	MountID     int32  `json:"mount_id,omitempty" field:"mount_id" tags:"fs"`
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

type UnlinkEvent struct {
	Inode       uint32 `json:"inode,omitempty" field:"inode" tags:"fs"`
	PathnameKey uint32 `json:"-" field:"filename,string,m.dentryResolver.Resolve({{.FieldPrefix}}{{.Field}})" tags:"fs"`
	MountID     int32  `json:"mount_id,omitempty" field:"mount_id" tags:"fs"`
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

type RenameEvent struct {
	SrcInode          uint32 `json:"oldinode,omitempty" field:"oldinode" tags:"fs"`
	SrcPathnameKey    uint32 `json:"-" field:"oldfilename,string,m.dentryResolver.Resolve({{.FieldPrefix}}{{.Field}})" tags:"fs"`
	SrcMountID        int32  `json:"oldmountid,omitempty" field:"oldmountid" tags:"fs"`
	TargetInode       uint32 `json:"newinode,omitempty" field:"newinode" tags:"fs"`
	TargetPathnameKey uint32 `json:"-" field:"newfilename,string,m.dentryResolver.Resolve({{.FieldPrefix}}{{.Field}})" tags:"fs"`
	TargetMountID     int32  `json:"newmountid,omitempty" field:"newmountid" tags:"fs"`
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

type ContainerEvent struct {
	ID     string   `yaml:"id" field:"id" tags:"container"`
	Labels []string `yaml:"labels" field:"labels" tags:"container"`
}

type KernelEvent struct {
	Type      uint64 `json:"retval" field:"type"`
	Timestamp uint64 `json:"-" field:"-"`
	Retval    int64  `json:"retval" field:"retval"`
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
	Pidns      uint64   `json:"pidns" field:"pidns" tags:"process"`
	Comm       [16]byte `json:"-" field:"name" handler:"HandleComm,string" tags:"process"`
	CommStr    string   `json:"" field:"-"`
	TTYName    [64]byte `json:"-" field:"tty_name" handler:"HandleTTY,string" tags:"process"`
	TTYNameStr string   `json:"tty" field:"-"`
	Pid        uint32   `json:"pid" field:"pid" tags:"process"`
	Tid        uint32   `json:"tid" field:"tid" tags:"process"`
	UID        uint32   `json:"uid" field:"uid" tags:"process"`
	GID        uint32   `json:"gid" field:"gid" tags:"process"`
}

func (p *ProcessEvent) HandleTTY(resolvers *Resolvers) string {
	return p.GetTTY()
}

func (p *ProcessEvent) GetTTY() string {
	if len(p.TTYNameStr) == 0 {
		p.TTYNameStr = string(bytes.Trim(p.TTYName[:], "\x00"))
	}
	return p.TTYNameStr
}

func (p *ProcessEvent) HandleComm(resolvers *Resolvers) string {
	return p.GetComm()
}

func (p *ProcessEvent) GetComm() string {
	if len(p.CommStr) == 0 {
		p.CommStr = string(bytes.Trim(p.Comm[:], "\x00"))
	}
	return p.CommStr
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
	ID        string         `json:"id" yaml:"id" field:"-"`
	Event     KernelEvent    `json:"event" yaml:"event" field:"event"`
	Process   ProcessEvent   `json:"process" yaml:"process" field:"process"`
	Open      OpenEvent      `json:"open" yaml:"open" field:"open"`
	Mkdir     MkdirEvent     `json:"mkdir" yaml:"mkdir" field:"mkdir"`
	Rmdir     RmdirEvent     `json:"rmdir" yaml:"rmdir" field:"rmdir"`
	Unlink    UnlinkEvent    `json:"unlink" yaml:"unlink" field:"unlink"`
	Rename    RenameEvent    `json:"rename" yaml:"rename" field:"rename"`
	Container ContainerEvent `json:"container" yaml:"container" field:"container"`

	resolvers *Resolvers `field:"-"`
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
