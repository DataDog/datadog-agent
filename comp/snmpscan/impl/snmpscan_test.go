// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package snmpscanimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	logscomp "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

type deps struct {
	fx.In
	EventPlatform eventplatform.Component
}

func TestSnmpScanComp(t *testing.T) {
	testDeps := fxutil.Test[deps](t, eventplatformimpl.MockModule(), logscomp.MockModule(), core.MockBundle(), hostnameimpl.MockModule())
	deps := Requires{
		Logger:        logmock.New(t),
		EventPlatform: testDeps.EventPlatform,
	}
	snmpScanner, err := NewComponent(deps)
	assert.NoError(t, err)

	snmpConnection := gosnmp.Default
	snmpConnection.LocalAddr = "127.0.0.1"
	snmpConnection.Port = 0

	err = snmpScanner.Comp.RunSnmpWalk(snmpConnection, "1.0")
	assert.ErrorContains(t, err, "&GoSNMP.Conn is missing. Provide a connection or use Connect()")

	err = snmpScanner.Comp.RunSnmpWalk(snmpConnection, "1.0")
	assert.ErrorContains(t, err, "&GoSNMP.Conn is missing. Provide a connection or use Connect()")
}
