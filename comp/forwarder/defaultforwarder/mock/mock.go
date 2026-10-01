// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package defaultforwardermock provides a mock forwarder component for testing.
package defaultforwardermock

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	impl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
)

// New creates a forwarder with mock dependencies for testing.
func New(t testing.TB) defaultforwarder.Component {
	cfg := configmock.NewMock(t)
	log := logmock.New(t)
	sec := secretsmock.New(t)
	opts, _ := impl.NewOptions(cfg, log, nil)
	if opts == nil {
		opts = &impl.Options{}
	}
	opts.Secrets = sec
	return impl.NewDefaultForwarder(cfg, log, opts)
}
