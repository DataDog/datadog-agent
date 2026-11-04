// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gce

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
)

func TestIsProductNameGCE(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("gce_use_dmi", true)

	dmi.SetupMockProductName(t, "")
	assert.False(t, isProductNameGCE())

	dmi.SetupMockProductName(t, DMIProductName)
	assert.True(t, isProductNameGCE())

	dmi.SetupMockProductName(t, "VMware Virtual Platform")
	assert.False(t, isProductNameGCE())

	cfg = configmock.New(t)
	cfg.SetInTest("gce_use_dmi", false)
	dmi.SetupMockProductName(t, DMIProductName)
	assert.False(t, isProductNameGCE())
}

func TestIsRunningOnDMI(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("gce_use_dmi", true)

	dmi.SetupMockProductName(t, "")
	assert.False(t, IsRunningOnDMI())

	dmi.SetupMockProductName(t, DMIProductName)
	assert.True(t, IsRunningOnDMI())
}
