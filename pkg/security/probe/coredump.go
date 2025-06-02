// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"bytes"
	"compress/gzip"
	json "encoding/json"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// CoreDump defines an internal core dump
type CoreDump struct {
	resolvers  *resolvers.EBPFResolvers
	definition *rules.CoreDumpDefinition
	event      events.EventMarshaler
}

// NewCoreDump returns a new core dump
func NewCoreDump(def *rules.CoreDumpDefinition, resolvers *resolvers.EBPFResolvers, event events.EventMarshaler) *CoreDump {
	return &CoreDump{
		resolvers:  resolvers,
		definition: def,
		event:      event,
	}
}

// ToJSON return json version of the dump
func (cd *CoreDump) ToJSON() ([]byte, error) {
	data, err := cd.event.ToJSON()
	if err != nil {
		return nil, err
	}

	content := struct {
		Event   json.RawMessage
		Process json.RawMessage
		Mount   json.RawMessage
		Dentry  json.RawMessage
	}{
		Event: data,
	}

	if cd.definition.Process {
		data, _ := cd.resolvers.ProcessResolver.ToJSON(false)
		content.Process = data
	}

	if cd.definition.Mount {
		data, _ := cd.resolvers.MountResolver.ToJSON()
		content.Mount = data
	}

	if cd.definition.Dentry {
		data, _ := cd.resolvers.DentryResolver.ToJSON()
		content.Dentry = data
	}

	data, err = json.Marshal(content)
	if err != nil {
		return nil, err
	}

	if cd.definition.NoCompression {
		return data, nil
	}

	buf := &bytes.Buffer{}
	gzWriter := gzip.NewWriter(buf)
	if _, err = gzWriter.Write(data); err != nil {
		gzWriter.Close()
		return nil, err
	}

	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	dump := struct {
		Data []byte
	}{
		Data: buf.Bytes(),
	}

	return json.Marshal(dump)
}
