// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestConfigSetHandler(t *testing.T) {
	coreconfig.SetupLogger("", seelog.InfoStr, "", "", false, true, false)
	var b bytes.Buffer
	f := bufio.NewWriter(&b)
	l, _ := seelog.LoggerFromWriterWithMinLevel(f, seelog.InfoLvl)
	log.RegisterAdditionalLogger("buffer", l)

	t.Run("warn", func(t *testing.T) {
		h := config.SetHandler()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", fmt.Sprintf("/?log_level=%s", seelog.WarnStr), nil))
		log.Warn("should be logged")
		f.Flush()

		currentLvl, err := log.GetLogLevel()
		assert.Nil(t, err)
		assert.Equal(t, seelog.WarnStr, coreconfig.Datadog.Get("log_level"))
		assert.Equal(t, seelog.WarnStr, currentLvl.String())
		assert.NotContains(t, b.String(), fmt.Sprintf("Switched log level to %s", seelog.WarnStr))
		assert.Contains(t, b.String(), "should be logged")
		assert.Equal(t, 200, rec.Code)
	})

	t.Run("debug", func(t *testing.T) {
		b.Reset()
		h := config.SetHandler()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", fmt.Sprintf("/?log_level=%s", seelog.DebugStr), nil))
		f.Flush()

		currentLvl, err := log.GetLogLevel()
		assert.Nil(t, err)
		assert.Equal(t, seelog.DebugStr, coreconfig.Datadog.Get("log_level"))
		assert.Equal(t, seelog.DebugStr, currentLvl.String())
		assert.Contains(t, b.String(), fmt.Sprintf("Switched log level to %s", seelog.DebugStr))
		assert.Equal(t, 200, rec.Code)
	})
}
