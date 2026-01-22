// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_http

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type HttpBundle struct {
	actions map[string]types.Action
}

func NewHttpBundle(cfg *config.Config) types.Bundle {
	return &HttpBundle{
		actions: map[string]types.Action{
			"request":            NewHttpRequestAction(cfg),
			"getChecksumFromURL": NewGetChecksumFromURLHandler(cfg),
		},
	}
}

func (h *HttpBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
