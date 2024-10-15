// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the snmpscan component
package mock

import (
	"testing"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	"github.com/gosnmp/gosnmp"
)

type mock struct {
	Logger log.Component
}

// Provides that defines the output of mocked snmpscan component
type Provides struct {
	comp snmpscan.Component
}

// New returns a mock snmpscanner
func New(*testing.T) Provides {
	return Provides{
		comp: mock{},
	}
}

func (m mock) RunDeviceScan(_ *gosnmp.GoSNMP, _ string, _ string) error {
	return nil
}
func (m mock) RunSnmpWalk(_ *gosnmp.GoSNMP, _ string) error {
	return nil
}
