// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
)

func TestIsChassisAssetTagAzure(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("azure_use_dmi", true)

	dmi.SetupMockChassisAssetTag(t, "")
	assert.False(t, isChassisAssetTagAzure())

	dmi.SetupMockChassisAssetTag(t, DMIChassisAssetTag)
	assert.True(t, isChassisAssetTagAzure())

	dmi.SetupMockChassisAssetTag(t, "some-other-tag")
	assert.False(t, isChassisAssetTagAzure())

	cfg = configmock.New(t)
	cfg.SetInTest("azure_use_dmi", false)
	dmi.SetupMockChassisAssetTag(t, DMIChassisAssetTag)
	assert.False(t, isChassisAssetTagAzure())
}

func TestIsRunningOnDMI(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("azure_use_dmi", true)

	dmi.SetupMockChassisAssetTag(t, "")
	assert.False(t, IsRunningOnDMI())

	dmi.SetupMockChassisAssetTag(t, DMIChassisAssetTag)
	assert.True(t, IsRunningOnDMI())
}
