// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

// Package connections provides a check that collects TCP connections from the discovery module
// and sends them to the NPM backend for service dependency mapping.
package connections

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "disco_connections"
)

// Factory returns an empty option on non-Linux platforms as this check is not supported.
func Factory(_ tagger.Component, _ connectionsforwarder.Component) option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
