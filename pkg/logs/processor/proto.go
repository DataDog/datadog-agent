// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// ProtoEncoder is a shared proto encoder.
var ProtoEncoder Encoder = &protoEncoder{}

// protoEncoder transforms a message into a protobuf byte array.
type protoEncoder struct{}

type Log struct {
	Message   string   `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	Status    string   `protobuf:"bytes,2,opt,name=status,proto3" json:"status,omitempty"`
	Timestamp int64    `protobuf:"varint,3,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	Hostname  string   `protobuf:"bytes,4,opt,name=hostname,proto3" json:"hostname,omitempty"`
	Service   string   `protobuf:"bytes,5,opt,name=service,proto3" json:"service,omitempty"`
	Source    string   `protobuf:"bytes,6,opt,name=source,proto3" json:"source,omitempty"`
	Tags      []string `protobuf:"bytes,7,rep,name=tags,proto3" json:"tags,omitempty"`
}

// Marshal encodes the Log struct to protobuf wire format using google.golang.org/protobuf.
func (l *Log) Marshal() ([]byte, error) {
	var buf []byte
	
	// Field 1: Message (string)
	if len(l.Message) > 0 {
		buf = protowire.AppendTag(buf, 1, protowire.BytesType)
		buf = protowire.AppendString(buf, l.Message)
	}
	
	// Field 2: Status (string)
	if len(l.Status) > 0 {
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendString(buf, l.Status)
	}
	
	// Field 3: Timestamp (int64)
	if l.Timestamp != 0 {
		buf = protowire.AppendTag(buf, 3, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(l.Timestamp))
	}
	
	// Field 4: Hostname (string)
	if len(l.Hostname) > 0 {
		buf = protowire.AppendTag(buf, 4, protowire.BytesType)
		buf = protowire.AppendString(buf, l.Hostname)
	}
	
	// Field 5: Service (string)
	if len(l.Service) > 0 {
		buf = protowire.AppendTag(buf, 5, protowire.BytesType)
		buf = protowire.AppendString(buf, l.Service)
	}
	
	// Field 6: Source (string)
	if len(l.Source) > 0 {
		buf = protowire.AppendTag(buf, 6, protowire.BytesType)
		buf = protowire.AppendString(buf, l.Source)
	}
	
	// Field 7: Tags (repeated string)
	for _, tag := range l.Tags {
		buf = protowire.AppendTag(buf, 7, protowire.BytesType)
		buf = protowire.AppendString(buf, tag)
	}
	
	return buf, nil
}

// Encode encodes a message into a protobuf byte array.
func (p *protoEncoder) Encode(msg *message.Message, hostname string) error {
	if msg.State != message.StateRendered {
		return errors.New("message passed to encoder isn't rendered")
	}

	log := &Log{
		Message:   toValidUtf8(msg.GetContent()),
		Status:    msg.GetStatus(),
		Timestamp: time.Now().UTC().UnixNano(),
		Hostname:  hostname,
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      msg.Tags(),
	}
	encoded, err := log.Marshal()

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}
