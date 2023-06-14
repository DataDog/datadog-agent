// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin

package v5

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metadata/internal/gohai"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	CommonPayload
	HostPayload
	ResourcesPayload
	GohaiPayload
}

// GohaiPayload wraps Payload from the gohai package
// As weird as it sounds, in the v5 payload the value of the "gohai" field
// is a JSON-formatted string. So this struct contains a MarshalledGohaiPayload
// which will be marshalled as a JSON-formatted string.
type GohaiPayload struct {
	Marshalled MarshalledGohaiPayload `json:"gohai"`
}

// MarshalledGohaiPayload contains the marshalled payload
type MarshalledGohaiPayload struct {
	gohai gohai.Payload
}

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	// Metadata payloads are analyzed as a whole, so they cannot be split
	return nil, fmt.Errorf("V5 Payload splitting is not implemented")
}

// MarshalJSON implements the json.Marshaler interface.
// It marshals the gohai struct twice (to a string) to comply with
// the v5 payload format
func (m MarshalledGohaiPayload) MarshalJSON() ([]byte, error) {
	marshalledPayload, err := json.Marshal(m.gohai.Gohai)
	if err != nil {
		return []byte(""), err
	}
	doubleMarshalledPayload, err := json.Marshal(string(marshalledPayload))
	if err != nil {
		return []byte(""), err
	}
	return doubleMarshalledPayload, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// Unmarshals the passed bytes twice (first to a string, then to gohai.Gohai)
func (m *MarshalledGohaiPayload) UnmarshalJSON(bytes []byte) error {
	firstUnmarshall := ""
	err := json.Unmarshal(bytes, &firstUnmarshall)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(firstUnmarshall), &(m.gohai.Gohai))
	return err
}
