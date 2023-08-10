// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build !linux && !windows
// +build !linux,!windows

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors -tags '!linux,!windows' -types-file model.go -output accessors_other.go -field-handlers field_handlers_other.go

package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

// ValidateField validates the value of a field
func (m *Model) ValidateField(field eval.Field, fieldValue eval.FieldValue) error {
	return nil
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct{}

// Process represents a process
type Process struct{}

// PIDContext holds the process context of an kernel event
type PIDContext struct{}

type NetworkDeviceContext struct{}
