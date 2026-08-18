// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package darwin

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// supportedFieldPrefixes are the SECL field namespaces darwin populates.
// Anything else is rejected at policy-load time, which is the point: a policy
// referencing a field this platform never fills would otherwise load cleanly and
// then silently never match.
var supportedFieldPrefixes = []string{
	"exec.",
	"exit.",
	"process.",
	"open.",
	"rename.",
	"unlink.",
	"event.",
}

// NewDarwinModel returns the SECL model for darwin, rejecting fields this
// platform does not populate.
func NewDarwinModel() *model.Model {
	return &model.Model{
		ExtraValidateFieldFnc: func(field eval.Field, _ eval.FieldValue) error {
			for _, prefix := range supportedFieldPrefixes {
				if strings.HasPrefix(field, prefix) {
					return nil
				}
			}
			return rules.ErrEventTypeNotEnabled
		},
	}
}

// SupportedEventTypes returns the event types darwin can emit.
//
// CreateNewFileEventType is deliberately absent even though it is reachable from
// darwin (secl/model/events.go carries no build tag): it lives in the Windows
// event-type range (FirstWindowsEventType == CreateNewFileEventType), so using it
// here would mislabel darwin events for the backend. File creation is expressed
// through open with create flags instead.
func SupportedEventTypes() map[eval.EventType]bool {
	return map[eval.EventType]bool{
		model.ExecEventType.String():       true,
		model.ForkEventType.String():       true,
		model.ExitEventType.String():       true,
		model.FileOpenEventType.String():   true,
		model.FileRenameEventType.String(): true,
		model.FileUnlinkEventType.String(): true,
	}
}
