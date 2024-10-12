// // Unless explicitly stated otherwise all files in this repository are licensed
// // under the Apache License Version 2.0.
// // This product includes software developed at Datadog (https://www.datadoghq.com/).
// // Copyright 2016-present Datadog, Inc.

// //go:build !windows

// //nolint:revive // TODO(PLINT) Fix revive linter
package wlan_test

// import (
// 	"testing"

// 	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
// 	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
// 	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/wlan"
// )

// func TestWLANOK(t *testing.T) {
// 	// wlanQuery = testWLANQuery
// 	// defer func() { wlanQuery = wlan.QueryWithOptions }()

// 	wlanCheck := new(wlan.WLANCheck)

// 	senderManager := mocksender.CreateDefaultDemultiplexer()
// 	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

// 	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)

// 	mockSender.On("Gauge", "wlan.rssi", float64(21), "", []string(nil)).Return().Times(1)
// 	wlanCheck.Run()

// 	mockSender.AssertExpectations(t)
// 	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
// 	mockSender.AssertNumberOfCalls(t, "Commit", 1)
// }
