// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	// ProtobufVersion defines the protobuf version in use
	ProtobufVersion = "v1"
)

// ActivityDumpHeader holds the header of an activity dump
type ActivityDumpHeader struct {
	// standard attributes used by the intake
	Host    string `json:"host,omitempty"`
	Service string `json:"service,omitempty"`
	Source  string `json:"ddsource,omitempty"`

	DDTags string `json:"ddtags,omitempty"`

	// Used to store the global list of DNS names contained in this dump
	// this is a hack used to provide this global list to the backend in the JSON header
	// instead of in the protobuf payload.
	DNSNames *utils.StringKeys `json:"dns_names"`
}
