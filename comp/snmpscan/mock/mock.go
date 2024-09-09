// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the snmpscan component
package mock

import (
	"testing"

	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	"github.com/gosnmp/gosnmp"
)

type mock struct{}

type Provides struct {
	comp snmpscan.Component
}

// New returns a mock compressor
func New(*testing.T) Provides {
	return Provides{
		comp: mock{},
	}
}

func (m mock) RunDeviceScan(snmpConection *gosnmp.GoSNMP, deviceNamespace string) error {
	return nil
}
func (m mock) RunSnmpWalk(snmpConection *gosnmp.GoSNMP, firstOid string) error {
	return nil
}
