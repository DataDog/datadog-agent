// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// Discarder represents a discarder which is basically the field that we know for sure
// that the value will be always rejected by the rules
type Discarder struct {
	Field eval.Field
}

// ErrDiscarderNotSupported is returned when trying to discover a discarder on a field that doesn't support them
type ErrDiscarderNotSupported struct {
	Field string
}

func (e ErrDiscarderNotSupported) Error() string {
	return fmt.Sprintf("discarder not supported for `%s`", e.Field)
}

var (
	// SupportedDiscarders lists all field which supports discarders
	SupportedDiscarders = make(map[eval.Field]bool)
	// SupportedMultiDiscarder lists all supported multi discarders
	SupportedMultiDiscarder []*rules.MultiDiscarder
)
