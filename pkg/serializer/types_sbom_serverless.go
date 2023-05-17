// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build serverless

package serializer

import "google.golang.org/protobuf/reflect/protoreflect"

// SBOMMessage is a type alias for SBOM proto payload and is not needed in
// serverless mode
type SBOMMessage struct {
	Version  int
	Host     string
	Source   *string
	Entities []interface{}
}

var _ protoreflect.ProtoMessage = (*SBOMMessage)(nil)

// ProtoReflect allows SBOMMessage to implement protoreflect.ProtoMessage
func (s *SBOMMessage) ProtoReflect() protoreflect.Message { return nil }
