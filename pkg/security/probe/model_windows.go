// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NewWindowsModel returns a new model with some extra field validation
func NewWindowsModel(_ *WindowsProbe) *model.Model {
	return &model.Model{
		ExtraValidateFieldFnc: func(field eval.Field, fieldValue eval.FieldValue) error {
			// TODO(safchain) remove this check when multiple model per platform will be supported in the SECL package
			if !strings.HasPrefix(field, "exec.") &&
				!strings.HasPrefix(field, "exit.") &&
				!strings.HasPrefix(field, "create.") &&
				!strings.HasPrefix(field, "open.") &&
				!strings.HasPrefix(field, "rename.") &&
				!strings.HasPrefix(field, "set.") &&
				!strings.HasPrefix(field, "delete.") &&
				!strings.HasPrefix(field, "write.") &&
				!strings.HasPrefix(field, "process.") &&
				!strings.HasPrefix(field, "change_permission") {
				return fmt.Errorf("%s is not available with the Windows version", field)
			}
			return nil
		},
	}
}

// NewWindowsEvent returns a new event
func NewWindowsEvent(fh *FieldHandlers) *model.Event {
	event := model.NewFakeEvent()
	event.FieldHandlers = fh
	return event
}
