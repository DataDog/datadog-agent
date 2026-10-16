package msrpc

import (
	"errors"
	"fmt"
	"math"
)

// NetShareEnumAllRequest represents an MS-SRVS NetrShareEnum request (Opnum 15).
type NetShareEnumAllRequest struct {
	CallId     uint32
	ServerName string
	Level      uint32
}

func (r *NetShareEnumAllRequest) Size() int {
	// PDU Header: 24 bytes
	// ServerName: 4 (ptr) + 12 (str header) + 2 * (len+1) + align
	// InfoStruct: 4 (Level) + 4 (switch Level) + 4 (ctr ptr) + 4 (EntriesRead) + 4 (Buffer ptr) + 4 (max buffer)
	// ResumeHandle: 4 (null ptr)
	enc := NewEncoder()
	r.encodeStub(enc)
	return HeaderSize + enc.Len()
}

func (r *NetShareEnumAllRequest) encodeStub(enc *Encoder) {
	// ServerName: [in, string, unique] SRVSVC_HANDLE ServerName
	enc.WriteUint32(0x00020000) // Referent ID
	enc.WriteConformantVaryingString(r.ServerName)

	// InfoStruct: [in, out] LPSHARE_ENUM_STRUCT InfoStruct
	// SHARE_ENUM_STRUCT: Level + switch_is(Level) SHARE_ENUM_UNION
	enc.WriteUint32(r.Level) // Level
	// Union: discriminant + arm
	enc.WriteUint32(r.Level)    // discriminant
	enc.WriteUint32(0x00020004) // Referent ID to SHARE_INFO_X_CONTAINER
	// Container: EntriesRead = 0, Buffer = NULL
	enc.WriteUint32(0) // EntriesRead
	enc.WriteUint32(0) // Buffer (NULL pointer)

	// PreferedMaximumLength: [in] DWORD PreferedMaximumLength (MAX_PREFERRED_LENGTH = 0xFFFFFFFF)
	enc.WriteUint32(0xFFFFFFFF)

	// ResumeHandle: [in, out, unique] DWORD* ResumeHandle (NULL pointer = 0)
	enc.WriteUint32(0)
}

// Validate rejects requests that cannot fit the RPC fragment length field.
func (r *NetShareEnumAllRequest) Validate() error {
	if r == nil || r.Size() > math.MaxUint16 {
		return fmt.Errorf("server name exceeds max MSRPC fragment size: %w", errInvalidArgument)
	}
	return nil
}

func (r *NetShareEnumAllRequest) Encode(b []byte) {
	enc := NewEncoder()
	r.encodeStub(enc)
	stub := enc.Bytes()

	totalLen := HeaderSize + len(stub)
	if totalLen > math.MaxUint16 {
		panic("msrpc: request fragment length exceeds uint16")
	}

	// Common header (16 bytes)
	encodeCommonHeader(b, RPC_TYPE_REQUEST, RPC_PACKET_FLAG_FIRST|RPC_PACKET_FLAG_LAST, uint16(totalLen), 0, r.CallId)

	// Request header (8 bytes)
	le.PutUint32(b[16:20], uint32(len(stub))) // alloc_hint
	le.PutUint16(b[20:22], 0)                 // context_id
	le.PutUint16(b[22:24], OP_NET_SHARE_ENUM) // opnum

	// Stub data
	copy(b[HeaderSize:], stub)
}

// ShareInfo represents information about a shared resource.
type ShareInfo struct {
	Name    string
	Type    uint32
	Comment string
}

// DecodeNetShareEnumAllResponse decodes a complete NetrShareEnum response stub.
// The stub contains only NDR parameters; validate each RPC fragment first.
func DecodeNetShareEnumAllResponse(stub []byte) ([]ShareInfo, error) {
	dec := NewDecoder(stub)

	level, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if level != 0 && level != 1 {
		return nil, errors.New("msrpc: unsupported share info level")
	}

	// switch_is(Level)
	switchLevel, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if switchLevel != level {
		return nil, errors.New("msrpc: share info level discriminant mismatch")
	}

	// container pointer
	containerPtr, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if containerPtr == 0 {
		return nil, errors.New("msrpc: nil share info container")
	}

	// EntriesRead
	entriesRead, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if entriesRead > 65536 {
		return nil, errInvalidCount
	}

	// buffer pointer
	bufferPtr, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if entriesRead != 0 && bufferPtr == 0 {
		return nil, errors.New("msrpc: nil share info buffer")
	}

	count := int(entriesRead)
	if bufferPtr == 0 {
		return readShareEnumTail(dec, entriesRead, nil)
	}

	// array max count
	arrayMaxCount, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if arrayMaxCount != entriesRead {
		return nil, errors.New("msrpc: share info array count mismatch")
	}

	infos := make([]ShareInfo, count)

	switch level {
	case 0:
		namePtrs := make([]uint32, count)
		for i := range count {
			namePtr, err := dec.ReadUint32()
			if err != nil {
				return nil, err
			}
			namePtrs[i] = namePtr
		}

		for i := range count {
			if namePtrs[i] == 0 {
				continue
			}
			name, err := dec.ReadConformantVaryingString()
			if err != nil {
				return nil, err
			}
			infos[i].Name = name
		}
	case 1:
		type inline1 struct {
			namePtr   uint32
			typ       uint32
			remarkPtr uint32
		}
		items := make([]inline1, count)
		for i := range count {
			namePtr, err := dec.ReadUint32()
			if err != nil {
				return nil, err
			}
			typ, err := dec.ReadUint32()
			if err != nil {
				return nil, err
			}
			remarkPtr, err := dec.ReadUint32()
			if err != nil {
				return nil, err
			}
			items[i] = inline1{namePtr: namePtr, typ: typ, remarkPtr: remarkPtr}
		}

		for i := range count {
			if items[i].namePtr != 0 {
				name, err := dec.ReadConformantVaryingString()
				if err != nil {
					return nil, err
				}
				infos[i].Name = name
			}
			infos[i].Type = items[i].typ

			if items[i].remarkPtr != 0 {
				remark, err := dec.ReadConformantVaryingString()
				if err != nil {
					return nil, err
				}
				infos[i].Comment = remark
			}
		}
	default:
		return nil, errors.New("msrpc: unsupported share info level")
	}

	return readShareEnumTail(dec, entriesRead, infos)
}

func readShareEnumTail(dec *Decoder, entriesRead uint32, infos []ShareInfo) ([]ShareInfo, error) {
	totalEntries, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if totalEntries < entriesRead {
		return nil, errors.New("msrpc: total entries is less than entries read")
	}

	// ResumeHandle is a unique pointer. A NULL pointer has no referent value.
	resumePtr, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if resumePtr != 0 {
		if _, err := dec.ReadUint32(); err != nil {
			return nil, err
		}
	}

	status, err := dec.ReadUint32()
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, errors.New("msrpc: NetrShareEnum returned failure status")
	}

	if infos == nil {
		return []ShareInfo{}, nil
	}
	return infos, nil
}

// DecodeNetShareEnumAllShareNames returns the list of share names.
func DecodeNetShareEnumAllShareNames(stub []byte) ([]string, error) {
	infos, err := DecodeNetShareEnumAllResponse(stub)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = info.Name
	}
	return names, nil
}
