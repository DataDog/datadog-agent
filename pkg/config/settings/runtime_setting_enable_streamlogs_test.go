// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
)

var (
	setting    *EnableStreamLogsRuntimeSetting
	mockConfig config.Mock
)

func setup(t *testing.T) {
	setting = NewEnableStreamLogsRuntimeSetting()
	mockConfig = config.NewMock(t)

}

func TestMain(m *testing.M) {
	m.Run()
}

func TestEnableStreamLogsRuntimeSetting_Description(t *testing.T) {
	expected := StreamLogRuntimeDescription
	assert.Equal(t, expected, setting.Description())
}

func TestEnableStreamLogsRuntimeSetting_Hidden(t *testing.T) {
	assert.False(t, setting.Hidden())
}

func TestEnableStreamLogsRuntimeSetting_Name(t *testing.T) {
	assert.Equal(t, "enable_streamlogs", setting.Name())
}
func TestEnableStreamLogsRuntimeSetting_Get(t *testing.T) {
	setup(t)

	mockConfig.SetWithoutSource(StreamLogsConfigKey, true)
	value, err := setting.Get(mockConfig)
	assert.NoError(t, err)
	assert.Equal(t, true, value)

	mockConfig.SetWithoutSource(StreamLogsConfigKey, false)
	value, err = setting.Get(mockConfig)
	assert.NoError(t, err)
	assert.Equal(t, false, value)
}

func TestEnableStreamLogsRuntimeSetting_Set(t *testing.T) {
	setup(t)
	allSources := mockConfig.GetAllSources(StreamLogsConfigKey)
	assert.NotNil(t, allSources)

	// Test enabling and disabling the setting for each source
	for _, source := range allSources {
		// Test enabling the runtime setting
		err := setting.Set(mockConfig, true, source.Source)
		assert.NoErrorf(t, err, "source:%s yields error: %v", source, err)
		value, err := setting.Get(mockConfig)
		assert.NoErrorf(t, err, "source:%s yields error: %v", source, err)
		assert.Equalf(t, true, value, "source:%s expected true but got %v", source, value)

		// Test disabling the runtime setting
		err = setting.Set(mockConfig, false, source.Source)
		assert.NoErrorf(t, err, "source:%s yields error: %v", source, err)
		value, err = setting.Get(mockConfig)
		assert.NoErrorf(t, err, "source:%s yields error: %v", source, err)
		assert.Equalf(t, false, value, "source:%s expected false but got %v", source, value)
	}
}
