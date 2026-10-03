// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock process submitter for testing.
package mock

import (
	"testing"

	submitter "github.com/DataDog/datadog-agent/comp/process/submitter/def"
	submitterimpl "github.com/DataDog/datadog-agent/comp/process/submitter/impl"
)

// New returns a testify mock submitter with optional Start, Stop, and Submit expectations.
func New(t testing.TB) submitter.Component {
	return submitterimpl.NewMock(t)
}
