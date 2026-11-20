// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestSupressedWarningRegex(t *testing.T) {
	fixtures := map[string]bool{
		"v1 ComponentStatus is deprecated in v1.19+":                                                                                                  true,
		"batch/v1beta1 CronJob is deprecated in v1.21+, unavailable in v1.25+; use batch/v1 CronJob":                                                  true,
		"autoscaling/v2beta1 HorizontalPodAutoscaler is deprecated in v1.22+, unavailable in v1.25+; use autoscaling/v2beta2 HorizontalPodAutoscaler": true,
		"keep me":    false,
		"deprecated": false,
	}

	for tc, expected := range fixtures {
		require.Equal(t, expected, supressedWarning.MatchString(tc))
	}
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func TestCustomWarningLoggerDeduplicates(t *testing.T) {
	var logs bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&logs, log.DebugLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "debug")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "debug") })

	clock := &fakeClock{now: time.Now()}
	l := &CustomWarningLogger{logged: cache.NewLRUExpireCacheWithClock(warningCacheSize, clock)}
	read := func() string {
		log.Flush()
		s := logs.String()
		logs.Reset()
		return s
	}

	const msg = "[.resources.requests.cpu] do not equal [.resources.limits.cpu] in container=app"
	l.HandleWarningHeader(299, "", msg)
	assert.Equal(t, "[INFO] Kubernetes API server warning: "+msg+"\n", read())

	l.HandleWarningHeader(299, "", msg)
	assert.Equal(t, "[DEBUG] Kubernetes API server warning: "+msg+"\n", read())

	l.HandleWarningHeader(299, "", "another warning")
	assert.Equal(t, "[INFO] Kubernetes API server warning: another warning\n", read())

	clock.now = clock.now.Add(warningLogInterval + time.Second)
	l.HandleWarningHeader(299, "", msg)
	assert.Equal(t, "[INFO] Kubernetes API server warning: "+msg+"\n", read())

	l.HandleWarningHeader(200, "", msg)
	l.HandleWarningHeader(299, "", "")
	l.HandleWarningHeader(299, "", "v1 ComponentStatus is deprecated in v1.19+")
	assert.Empty(t, read())
}
