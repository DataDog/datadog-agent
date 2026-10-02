// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
)

func setupEUDMTest(t *testing.T, eudm bool, sysInfo *systeminfo.SystemInfo, sysInfoErr error) {
	t.Cleanup(func() {
		systeminfoCollect = systeminfo.Collect
	})

	cfg := configmock.New(t)
	if eudm {
		cfg.SetInTest("infrastructure_mode", "end_user_device")
	}

	systeminfoCollect = func() (*systeminfo.SystemInfo, error) {
		return sysInfo, sysInfoErr
	}
}

func TestFromEUDMSerialNumberGateOff(t *testing.T) {
	setupEUDMTest(t, false, &systeminfo.SystemInfo{SerialNumber: "ABC123"}, nil)

	hostname, err := fromEUDMSerialNumber(context.Background(), "")
	assert.Error(t, err)
	assert.Empty(t, hostname)
}

func TestFromEUDMSerialNumberCollectionError(t *testing.T) {
	setupEUDMTest(t, true, nil, errors.New("collection failed"))

	hostname, err := fromEUDMSerialNumber(context.Background(), "")
	assert.Error(t, err)
	assert.Empty(t, hostname)
}

func TestFromEUDMSerialNumberEmptySerial(t *testing.T) {
	setupEUDMTest(t, true, &systeminfo.SystemInfo{SerialNumber: ""}, nil)

	hostname, err := fromEUDMSerialNumber(context.Background(), "")
	assert.Error(t, err)
	assert.Empty(t, hostname)
}

func TestFromEUDMSerialNumberValidSerial(t *testing.T) {
	setupEUDMTest(t, true, &systeminfo.SystemInfo{SerialNumber: "C02ABCDE1234"}, nil)

	hostname, err := fromEUDMSerialNumber(context.Background(), "")
	assert.NoError(t, err)
	assert.Equal(t, "C02ABCDE1234", hostname)
}

func TestFromEUDMSerialNumberSanitization(t *testing.T) {
	setupEUDMTest(t, true, &systeminfo.SystemInfo{SerialNumber: "C02 ABC_DE 1234!"}, nil)

	hostname, err := fromEUDMSerialNumber(context.Background(), "")
	assert.NoError(t, err)
	assert.Equal(t, "C02ABCDE1234", hostname)
}

func TestSanitizeEUDMSerialNumber(t *testing.T) {
	assert.Equal(t, "ABC123", sanitizeEUDMSerialNumber("ABC 123"))
	assert.Equal(t, "ABC-123.foo", sanitizeEUDMSerialNumber(" ABC-123.foo "))
	assert.Equal(t, "", sanitizeEUDMSerialNumber("   "))
	assert.Equal(t, "ABC123", sanitizeEUDMSerialNumber("ABC_123"))
}
