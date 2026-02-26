// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"bytes"
	"encoding/binary"
	"io"

	model "github.com/DataDog/agent-payload/v5/process"
)

func encodeHeaderV3(b io.Writer, h model.MessageHeader) error {
	err := binary.Write(b, binary.LittleEndian, uint8(h.Version))
	if err != nil {
		return err
	}
	err = binary.Write(b, binary.LittleEndian, uint8(h.Encoding))
	if err != nil {
		return err
	}
	err = binary.Write(b, binary.LittleEndian, uint8(h.Type))
	if err != nil {
		return err
	}
	err = binary.Write(b, binary.LittleEndian, h.SubscriptionID)
	if err != nil {
		return err
	}
	err = binary.Write(b, binary.LittleEndian, h.OrgID)
	if err != nil {
		return err
	}
	err = binary.Write(b, binary.LittleEndian, h.Timestamp)
	if err != nil {
		return err
	}
	return nil
}

func (d *directSender) encodeHeader() error {
	var buf bytes.Buffer
	err := encodeHeaderV3(&buf, model.MessageHeader{
		Version:  model.MessageV3,
		Encoding: model.MessageEncodingZstd1xPB,
		Type:     model.TypeCollectorConnections,
	})
	if err != nil {
		return err
	}
	d.staticEncodedHeader = buf.Bytes()
	return nil
}
