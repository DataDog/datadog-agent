// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"bytes"
	"fmt"
	"io"

	adprotov1 "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"google.golang.org/protobuf/encoding/protojson"
)

// EncodeJSON encodes an activity dump in the ProtoJSON format
func (p *Profile) EncodeJSON(indent string) (*bytes.Buffer, error) {
	p.m.Lock()
	defer p.m.Unlock()

	pad := profileToSecDumpProto(p)
	defer pad.ReturnToVTPool()

	opts := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
		Indent:          indent,
	}

	raw, err := opts.Marshal(pad)
	if err != nil {
		return nil, fmt.Errorf("couldn't encode in json: %v", err)
	}
	return bytes.NewBuffer(raw), nil
}

// DecodeJSON decodes JSON to an activity dump
func (p *Profile) DecodeJSON(reader io.Reader) error {
	p.m.Lock()
	defer p.m.Unlock()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("couldn't open security profile file: %w", err)
	}

	opts := protojson.UnmarshalOptions{
		AllowPartial:   true,
		DiscardUnknown: true,
	}
	inter := &adprotov1.SecDump{}
	if err = opts.Unmarshal(raw, inter); err != nil {
		return fmt.Errorf("couldn't decode json file: %w", err)
	}

	secDumpProtoToProfile(p, inter)

	return nil
}
