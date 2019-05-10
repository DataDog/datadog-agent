package model

// message.go is a stripped down version of the backend message processing
// with support for the same MessageVersion and MessageEncoding but with
// only a limited set of message types.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"

	"github.com/DataDog/zstd"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
)

// MessageEncoding represents how messages will be encoded or decoded for
// over-the-wire transfer. Protobuf should be used for server-side messages
// (e.g. from collector <-> server) and JSON should be used for client-side.
type MessageEncoding uint8

// Message encoding constants.
const (
	MessageEncodingProtobuf MessageEncoding = 0
	MessageEncodingJSON     MessageEncoding = 1
	MessageEncodingZstdPB   MessageEncoding = 2
)

// MessageVersion is the version of the message. It should always be the first
// byte in the encoded version.
type MessageVersion uint8

// Message versioning constants.
const (
	MessageV1 MessageVersion = 1
	MessageV2                = 2
	MessageV3                = 3
)

const headerLength = 1 + 1 + 1 + 1 + 4

// MessageHeader is attached to all messages at the head of the message. Some
// fields are added in later versions so make sure you're only using fields that
// are available in the defined Version.
type MessageHeader struct {
	Version        MessageVersion
	Encoding       MessageEncoding
	Type           MessageType
	SubscriptionID uint8 // Unused in Agent
	OrgID          int32 // Unused in Agent
	Timestamp      int64
}

func unmarshal(enc MessageEncoding, body []byte, m proto.Message) error {
	switch enc {
	case MessageEncodingProtobuf:
		return proto.Unmarshal(body, m)
	case MessageEncodingJSON:
		return jsonpb.Unmarshal(bytes.NewReader(body), m)
	case MessageEncodingZstdPB:
		d, err := zstd.Decompress(nil, body)
		if err != nil {
			return err
		}
		return proto.Unmarshal(d, m)
	}
	return fmt.Errorf("unknown message encoding: %d", enc)
}

// MessageType is a string representing the type of a message.
type MessageType uint8

// Message type constants for MessageType.
// Note: Ordering my seem unusual, this is just to match the backend where there
// are additional types that aren't covered here.
const (
	TypeCollectorProc              = 12
	TypeCollectorConnections       = 22
	TypeResCollector               = 23
	TypeCollectorRealTime          = 27
	TypeCollectorContainer         = 39
	TypeCollectorContainerRealTime = 40
)

// Message is a generic type for all messages with a Header and Body.
type Message struct {
	Header MessageHeader
	Body   MessageBody
}

// MessageBody is a common interface used by all message types.
type MessageBody interface {
	ProtoMessage()
	Reset()
	String() string
	Size() int
}

// DecodeMessage decodes raw message bytes into a specific type that satisfies
// the Message interface. If we can't decode, an error is returned.
func DecodeMessage(data []byte) (Message, error) {
	header, offset, err := ReadHeader(data)
	if err != nil {
		return Message{}, err
	}
	body := data[offset:]
	var m MessageBody
	switch header.Type {
	case TypeCollectorProc:
		m = &CollectorProc{}
	case TypeCollectorConnections:
		m = &CollectorConnections{}
	case TypeCollectorRealTime:
		m = &CollectorRealTime{}
	case TypeResCollector:
		m = &ResCollector{}
	case TypeCollectorContainer:
		m = &CollectorContainer{}
	case TypeCollectorContainerRealTime:
		m = &CollectorContainerRealTime{}
	default:
		return Message{}, fmt.Errorf("unhandled message type: %d", header.Type)
	}
	if err = unmarshal(header.Encoding, body, m); err != nil {
		return Message{}, err
	}
	return Message{header, m}, nil
}

// DetectMessageType returns the message type for the given MessageBody
func DetectMessageType(b MessageBody) (MessageType, error) {
	var t MessageType
	switch b.(type) {
	case *CollectorProc:
		t = TypeCollectorProc
	case *CollectorConnections:
		t = TypeCollectorConnections
	case *CollectorRealTime:
		t = TypeCollectorRealTime
	case *ResCollector:
		t = TypeResCollector
	case *CollectorContainer:
		t = TypeCollectorContainer
	case *CollectorContainerRealTime:
		t = TypeCollectorContainerRealTime
	default:
		return 0, fmt.Errorf("unknown message body type: %s", reflect.TypeOf(b))
	}
	return t, nil
}

// EncodeMessage encodes a message object into bytes with protobuf. A type
// header is added for ease of decoding.
func EncodeMessage(m Message) ([]byte, error) {
	hb, err := encodeHeader(m.Header)
	if err != nil {
		return nil, fmt.Errorf("could not encode header: %s", err)
	}

	b := new(bytes.Buffer)
	if _, err := b.Write(hb); err != nil {
		return nil, err
	}

	var p []byte
	switch m.Header.Encoding {
	case MessageEncodingProtobuf:
		p, err = proto.Marshal(m.Body)
		if err != nil {
			return nil, err
		}
	case MessageEncodingJSON:
		marshaler := jsonpb.Marshaler{EmitDefaults: true}
		s, err := marshaler.MarshalToString(m.Body)
		if err != nil {
			return nil, err
		}
		p = []byte(s)
	case MessageEncodingZstdPB:
		pb, err := proto.Marshal(m.Body)
		if err != nil {
			return nil, err
		}
		p, err = zstd.Compress(nil, pb)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown message encoding: %d", m.Header.Encoding)
	}
	_, err = b.Write(p)
	return b.Bytes(), err
}

// ReadHeader reads the header off raw message bytes.
func ReadHeader(data []byte) (MessageHeader, int, error) {
	if len(data) <= 4 {
		return MessageHeader{}, 0, fmt.Errorf("invalid message length: %d", len(data))
	}
	switch MessageVersion(uint8(data[0])) {
	case MessageV1:
		return readHeaderV1(data)
	case MessageV2:
		return readHeaderV2(data)
	default:
		return MessageHeader{}, 0, fmt.Errorf("invalid message version: %d", uint8(data[0]))
	}
}

func readHeaderV1(data []byte) (MessageHeader, int, error) {
	b := bytes.NewBuffer(data[1:])
	var msgEnc uint8
	if err := binary.Read(b, binary.LittleEndian, &msgEnc); err != nil {
		return MessageHeader{}, 0, err
	}
	var msgType uint8
	if err := binary.Read(b, binary.LittleEndian, &msgType); err != nil {
		return MessageHeader{}, 0, err
	}
	var subID uint8
	if err := binary.Read(b, binary.LittleEndian, &subID); err != nil {
		return MessageHeader{}, 0, err
	}
	return MessageHeader{
		Version:        MessageV1,
		Encoding:       MessageEncoding(msgEnc),
		Type:           MessageType(msgType),
		SubscriptionID: subID,
		OrgID:          0,
	}, 4, nil
}

func readHeaderV2(data []byte) (MessageHeader, int, error) {
	b := bytes.NewBuffer(data[1:])
	var msgEnc uint8
	if err := binary.Read(b, binary.LittleEndian, &msgEnc); err != nil {
		return MessageHeader{}, 0, err
	}
	var msgType uint8
	if err := binary.Read(b, binary.LittleEndian, &msgType); err != nil {
		return MessageHeader{}, 0, err
	}
	var subID uint8
	if err := binary.Read(b, binary.LittleEndian, &subID); err != nil {
		return MessageHeader{}, 0, err
	}
	var orgID int32
	if err := binary.Read(b, binary.LittleEndian, &orgID); err != nil {
		return MessageHeader{}, 0, err
	}
	return MessageHeader{
		Version:        MessageV2,
		Encoding:       MessageEncoding(msgEnc),
		Type:           MessageType(msgType),
		SubscriptionID: subID,
		OrgID:          orgID,
	}, 8, nil
}

func readHeaderV3(data []byte) (MessageHeader, int, error) {
	b := bytes.NewBuffer(data[1:])
	var msgEnc uint8
	if err := binary.Read(b, binary.LittleEndian, &msgEnc); err != nil {
		return MessageHeader{}, 0, err
	}
	var msgType uint8
	if err := binary.Read(b, binary.LittleEndian, &msgType); err != nil {
		return MessageHeader{}, 0, err
	}
	var subID uint8
	if err := binary.Read(b, binary.LittleEndian, &subID); err != nil {
		return MessageHeader{}, 0, err
	}
	var orgID int32
	if err := binary.Read(b, binary.LittleEndian, &orgID); err != nil {
		return MessageHeader{}, 0, err
	}
	var timestamp int64
	if err := binary.Read(b, binary.LittleEndian, &timestamp); err != nil {
		return MessageHeader{}, 0, err
	}
	return MessageHeader{
		Version:        MessageV3,
		Encoding:       MessageEncoding(msgEnc),
		Type:           MessageType(msgType),
		SubscriptionID: subID,
		OrgID:          orgID,
		Timestamp:      timestamp,
	}, 16, nil
}

func encodeHeader(h MessageHeader) ([]byte, error) {
	switch h.Version {
	case MessageV3:
		return encodeHeaderV3(h)
	default:
		return nil, fmt.Errorf("invalid message version: %d", h.Version)
	}
}

func encodeHeaderV3(h MessageHeader) ([]byte, error) {
	b := new(bytes.Buffer)
	err := binary.Write(b, binary.LittleEndian, uint8(h.Version))
	if err != nil {
		return nil, err
	}
	err = binary.Write(b, binary.LittleEndian, uint8(h.Encoding))
	if err != nil {
		return nil, err
	}
	err = binary.Write(b, binary.LittleEndian, uint8(h.Type))
	if err != nil {
		return nil, err
	}
	err = binary.Write(b, binary.LittleEndian, uint8(h.SubscriptionID))
	if err != nil {
		return nil, err
	}
	err = binary.Write(b, binary.LittleEndian, h.OrgID)
	if err != nil {
		return nil, err
	}
	err = binary.Write(b, binary.LittleEndian, h.Timestamp)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
