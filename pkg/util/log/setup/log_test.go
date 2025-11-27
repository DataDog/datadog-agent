// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"

	seelogCfg "github.com/DataDog/datadog-agent/pkg/util/log/setup/internal/seelog"
)

func TestSeelogConfig(t *testing.T) {
	cfg := seelogCfg.NewSeelogConfig("TEST", "off", "common", "", "", false, nil, nil)
	cfg.EnableConsoleLog(true)
	cfg.EnableFileLogging("/dev/null", 123, 456)

	seelogConfigStr, err := cfg.Render()
	assert.Nil(t, err)

	logger, err := seelog.LoggerFromConfigAsString(seelogConfigStr)
	assert.Nil(t, err)
	assert.NotNil(t, logger)
}
