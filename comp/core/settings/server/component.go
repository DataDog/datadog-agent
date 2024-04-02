// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package server defines the interface for the settings server
package server

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	GetFullConfig(cfg config.Config, namespaces ...string) http.HandlerFunc
	GetValue(w http.ResponseWriter, r *http.Request)
	SetValue(w http.ResponseWriter, r *http.Request)
	ListConfigurable(w http.ResponseWriter, r *http.Request)
}

// RuntimeSettingResponse is used to communicate settings config
type RuntimeSettingResponse struct {
	Description string
	Hidden      bool
}
