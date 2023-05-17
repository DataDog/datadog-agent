// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !orchestrator

package serializer

// ProcessMessageBody is a type alias for processes proto message body
// this type alias allows to avoid importing the process agent payload proto
// in case it's not needed (dogstastd)
type ProcessMessageBody = stubMessageBody

type stubMessageBody struct{}

func (stubMessageBody) ProtoMessage()  {}
func (stubMessageBody) Reset()         {}
func (stubMessageBody) String() string { return "" }
func (stubMessageBody) Size() int      { return 0 }

// processPayloadEncoder is a dummy ProcessMessageBody to avoid importing
// the process agent payload proto in case it's not needed (dogstastd)
var processPayloadEncoder = func(m ProcessMessageBody) ([]byte, error) {
	return []byte{}, nil
}
