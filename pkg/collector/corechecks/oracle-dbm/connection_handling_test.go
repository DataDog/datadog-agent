// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestFailingConnection(t *testing.T) {
	senderManager := mocksender.CreateDefaultDemultiplexer()
	chk, _ := initCheck(t, senderManager, "localhost", 1523, "a", "a", "a")
	chk.Run()
}
