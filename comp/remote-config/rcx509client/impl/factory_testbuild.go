// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package rcx509clientimpl

import (
	"errors"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
)

// NewClientFactory returns a fail-closed factory for test builds. Component
// tests inject a fake factory and do not need to compile or link the native
// libdd-rc client.
func NewClientFactory(_ log.Component) rcx509client.ClientFactory {
	return func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		return nil, errors.New("remote config x509 client is unavailable in test builds")
	}
}
