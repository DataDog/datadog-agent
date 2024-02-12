// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// NewEBPFLessModel returns a new model with some extra field validation
func NewEBPFLessModel() *model.Model {
	return &model.Model{
		ExtraValidateFieldFnc: func(field eval.Field, fieldValue eval.FieldValue) error {
			// TODO(safchain) remove this check when multiple model per platform will be supported in the SECL package
			if !strings.HasPrefix(field, "exec.") &&
				!strings.HasPrefix(field, "exit.") &&
				!strings.HasPrefix(field, "open.") &&
				!strings.HasPrefix(field, "process.") &&
				!strings.HasPrefix(field, "setuid.") &&
				!strings.HasPrefix(field, "setgid.") &&
				!strings.HasPrefix(field, "setreuid.") &&
				!strings.HasPrefix(field, "setregid.") &&
				!strings.HasPrefix(field, "setfsuid.") &&
				!strings.HasPrefix(field, "setfsgid.") &&
				!strings.HasPrefix(field, "capset.") &&
				!strings.HasPrefix(field, "rmdir.") &&
				!strings.HasPrefix(field, "unlink.") &&
				!strings.HasPrefix(field, "rename.") &&
				!strings.HasPrefix(field, "mkdir.") &&
				!strings.HasPrefix(field, "utimes.") &&
				!strings.HasPrefix(field, "link.") &&
				!strings.HasPrefix(field, "chmod.") &&
				!strings.HasPrefix(field, "chown.") &&
				!strings.HasPrefix(field, "load_module.") &&
				!strings.HasPrefix(field, "unload_module.") &&
				!strings.HasPrefix(field, "container.") &&
				!strings.HasPrefix(field, "event.") {
				return rules.ErrEventTypeNotEnabled
			}
			return nil
		},
	}
}

// NewEBPFLessEvent returns a new event
func NewEBPFLessEvent(fh *EBPFLessFieldHandlers) *model.Event {
	event := model.NewDefaultEvent()
	event.FieldHandlers = fh
	return event
}
