// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"testing"

	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTagsSetup(t *testing.T) {
	// TODO: Fix and re-enable flaky test
	t.Skip()

	config.Mock(t)

	ddTagsEnv := "key1:value1 key2:value2 key3:value3:4"
	ddExtraTagsEnv := "key22:value22 key23:value23"
	t.Setenv("DD_TAGS", ddTagsEnv)
	t.Setenv("DD_EXTRA_TAGS", ddExtraTagsEnv)
	ddTags := cast.ToStringSlice(ddTagsEnv)
	ddExtraTags := cast.ToStringSlice(ddExtraTagsEnv)

	allTags := append(ddTags, ddExtraTags...)

	_, _, traceAgent, metricAgent, _ := setup()
	defer traceAgent.Stop()
	defer metricAgent.Stop()
	assert.Subset(t, metricAgent.GetExtraTags(), allTags)
	assert.Subset(t, logs.GetLogsTags(), allTags)
}

func TestFxApp(t *testing.T) {
	fxutil.TestOneShot(t, main)
}
