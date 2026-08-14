// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package com_datadoghq_remoteaction_internal hosts internal remote actions
// used to support per-task secret input encryption (see ADRAP-37). The
// directory is named `internalactions` rather than `internal` to avoid Go's
// reserved import path restriction on `internal/`.
package com_datadoghq_remoteaction_internal

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type InternalBundle struct {
	actions map[string]types.Action
}

func NewInternal(store *encryptioncontext.Store) types.Bundle {
	return &InternalBundle{
		actions: map[string]types.Action{
			"prepareEncryption": NewPrepareEncryptionHandler(store),
		},
	}
}

func (h *InternalBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
