// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package metadata

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestGetMinInterval(t *testing.T) {
	config.Mock(t).Set("inventories_min_interval", 6)
	assert.EqualValues(t, 6*time.Second, getMinInterval())
}

func TestGetMinIntervalInvalid(t *testing.T) {
	mockConf := config.Mock(t)

	// an invalid integer results in a value of 0 from Viper (with a logged warning)
	mockConf.Set("inventories_min_interval", 0)
	assert.EqualValues(t, config.DefaultInventoriesMinInterval*time.Second, getMinInterval())

	// an invalid integer results in a value of 0 from Viper (with a logged warning)
	mockConf.Set("inventories_min_interval", -1)
	assert.EqualValues(t, config.DefaultInventoriesMinInterval*time.Second, getMinInterval())
}

func TestGetMaxInterval(t *testing.T) {
	config.Mock(t).Set("inventories_max_interval", 6)
	assert.EqualValues(t, 6*time.Second, getMaxInterval())
}

func TestGetMaxIntervalInvalid(t *testing.T) {
	mockConf := config.Mock(t)

	// an invalid integer results in a value of 0 from Viper (with a logged warning)
	mockConf.Set("inventories_max_interval", 0)
	assert.EqualValues(t, config.DefaultInventoriesMaxInterval*time.Second, getMaxInterval())
	mockConf.Set("inventories_max_interval", -1)
	assert.EqualValues(t, config.DefaultInventoriesMaxInterval*time.Second, getMaxInterval())
}
