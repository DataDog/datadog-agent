// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package snmpscanimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

type deps struct {
	fx.In
	Demultiplexer demultiplexer.Mock
}

func TestSnmpScanComp(t *testing.T) {
	testDeps := fxutil.Test[deps](t, demultiplexerimpl.MockModule(), defaultforwarder.MockModule(), core.MockBundle())
	deps := Requires{
		Logger:        logmock.New(t),
		Demultiplexer: testDeps.Demultiplexer,
	}
	snmpScanner, err := NewComponent(deps)
	assert.NoError(t, err)

	snmpConnection := gosnmp.Default
	snmpConnection.LocalAddr = "127.0.0.1"
	snmpConnection.Port = 0

	err = snmpScanner.Comp.RunDeviceScan(snmpConnection, "default", "127.0.0.1")
	assert.ErrorContains(t, err, "&GoSNMP.Conn is missing. Provide a connection or use Connect()")

	err = snmpScanner.Comp.RunDeviceScan(snmpConnection, "default", "127.0.0.1")
	assert.ErrorContains(t, err, "&GoSNMP.Conn is missing. Provide a connection or use Connect()")
}
