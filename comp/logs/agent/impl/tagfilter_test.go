// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func tagFilterTestAgent(t *testing.T, global map[string]interface{}) *logAgent {
	t.Helper()
	t.Cleanup(func() { tagfilter.SetGlobal(nil) })
	cfg := configmock.New(t)
	if global != nil {
		cfg.SetInTest("logs_config.tag_filters", global)
	}
	a := &logAgent{
		log:     logmock.New(t),
		config:  cfg,
		sources: sources.NewLogSources(),
	}
	require.NoError(t, a.setupTagFilters())
	return a
}

func tagFilterTestSource(perSource *config.TagFilters) *sources.LogSource {
	return sources.NewLogSource("test-source", &config.LogsConfig{
		Type:       config.FileType,
		Path:       "/var/log/test.log",
		Tags:       []string{"dirname:/var/log", "kube_app_name:web", "team:logs"},
		TagFilters: perSource,
	})
}

func TestTagFilterWiringIsLive(t *testing.T) {
	a := tagFilterTestAgent(t, nil)
	source := tagFilterTestSource(&config.TagFilters{Exclude: []string{"dirname:*"}})
	a.sources.AddSource(source)

	origin := message.NewOrigin(source)

	assert.Equal(t, []string{"kube_app_name:web", "team:logs"}, origin.TransportTags())
	assert.Equal(t, "kube_app_name:web,team:logs", origin.TransportTagsToString())

	assert.Equal(t, []string{"dirname:/var/log", "kube_app_name:web", "team:logs"}, origin.Tags())
}

func TestGlobalTagFilterAppliesWithoutPerSourceBlock(t *testing.T) {
	a := tagFilterTestAgent(t, map[string]interface{}{"exclude": []string{"dirname:*"}})
	source := tagFilterTestSource(nil)
	a.sources.AddSource(source)

	origin := message.NewOrigin(source)
	assert.Equal(t, []string{"kube_app_name:web", "team:logs"}, origin.TransportTags())
	assert.Equal(t, []string{"dirname:/var/log", "kube_app_name:web", "team:logs"}, origin.Tags())
}

func TestPerSourceIncludeOverridesGlobalExclude(t *testing.T) {
	a := tagFilterTestAgent(t, map[string]interface{}{
		"exclude": []string{"dirname:*", "kube_app_*"},
	})
	source := tagFilterTestSource(&config.TagFilters{Include: []string{"dirname:*"}})
	a.sources.AddSource(source)

	origin := message.NewOrigin(source)
	assert.Equal(t, []string{"dirname:/var/log", "team:logs"}, origin.TransportTags())
	assert.Equal(t, []string{"dirname:/var/log", "kube_app_name:web", "team:logs"}, origin.Tags())
}

func TestGlobalExcludeAppliesWhereTheSourceIsSilent(t *testing.T) {
	a := tagFilterTestAgent(t, map[string]interface{}{
		"exclude": []string{"dirname:*", "team:*"},
	})
	source := tagFilterTestSource(&config.TagFilters{Include: []string{"dirname:*"}})
	a.sources.AddSource(source)

	origin := message.NewOrigin(source)
	assert.Equal(t, []string{"dirname:/var/log", "kube_app_name:web"}, origin.TransportTags())
}

func TestUnconfiguredSourceGetsNoFilter(t *testing.T) {
	a := tagFilterTestAgent(t, nil)
	source := tagFilterTestSource(nil)
	a.sources.AddSource(source)

	assert.Nil(t, source.TagFilters())
	assert.Nil(t, message.NewOrigin(source).TagFilters())
	assert.Empty(t, source.GetInfoStatus(true)["Tag Filters"], "an unfiltered source should not clutter the status page")
}

func TestTagFilterRulesAreReportedToStatus(t *testing.T) {
	a := tagFilterTestAgent(t, map[string]interface{}{"exclude": []string{"team:*"}})
	source := tagFilterTestSource(&config.TagFilters{Exclude: []string{"dirname:*"}})
	a.sources.AddSource(source)

	origin := message.NewOrigin(source)
	require.Equal(t, []string{"kube_app_name:web"}, origin.TransportTags())

	assert.Equal(t, []string{
		"global exclude: team:*",
		"source exclude: dirname:*",
	}, source.GetInfoStatus(true)["Tag Filters"])
}

func TestTagFilterRulesReportedAreStableUnderTraffic(t *testing.T) {
	a := tagFilterTestAgent(t, map[string]interface{}{"exclude": []string{"team:*"}})
	source := tagFilterTestSource(&config.TagFilters{Exclude: []string{"dirname:*"}})
	a.sources.AddSource(source)

	before := source.GetInfoStatus(true)["Tag Filters"]
	require.NotEmpty(t, before)

	origin := message.NewOrigin(source)
	for i := 0; i < 10; i++ {
		require.Equal(t, []string{"kube_app_name:web"}, origin.TransportTags())
	}

	assert.Equal(t, before, source.GetInfoStatus(true)["Tag Filters"])
}

func TestTagFilterRulesAreVerboseOnly(t *testing.T) {
	a := tagFilterTestAgent(t, map[string]interface{}{"exclude": []string{"team:*"}})
	source := tagFilterTestSource(nil)
	a.sources.AddSource(source)

	require.NotEmpty(t, source.GetInfoStatus(true)["Tag Filters"],
		"the section must be there in the verbose view")
	assert.Empty(t, source.GetInfoStatus(false)["Tag Filters"],
		"the section must be hidden from the default view")
}

func TestTagFilterIncludeRulesAreReportedToStatus(t *testing.T) {
	a := tagFilterTestAgent(t, nil)
	source := tagFilterTestSource(&config.TagFilters{
		Include: []string{"kube_app_name:*", "team:*"},
		Exclude: []string{"kube_*", "team:*", "dirname:*"},
	})
	a.sources.AddSource(source)

	origin := message.NewOrigin(source)
	require.Equal(t, []string{"kube_app_name:web", "team:logs"}, origin.TransportTags())

	assert.Equal(t, []string{
		"source include: kube_app_name:*, team:*",
		"source exclude: kube_*, team:*, dirname:*",
	}, source.GetInfoStatus(true)["Tag Filters"])
}

// tagFilterWarningAgent builds an agent whose warnings land in the returned buffer.
func tagFilterWarningAgent(t *testing.T, global map[string]interface{}) (*logAgent, *bytes.Buffer) {
	t.Helper()
	t.Cleanup(func() { tagfilter.SetGlobal(nil) })

	var output bytes.Buffer
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndFullFormat(&output, pkglog.WarnLvl)
	require.NoError(t, err)
	t.Cleanup(func() {
		pkglog.SetupLogger(pkglog.Default(), pkglog.InfoStr)
		logger.Close()
	})
	pkglog.SetupLogger(logger, pkglog.WarnStr)

	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.tag_filters", global)
	return &logAgent{log: pkglog.NewWrapper(2), config: cfg, sources: sources.NewLogSources()}, &output
}

func TestGlobalIncludeOnlyWarns(t *testing.T) {
	a, output := tagFilterWarningAgent(t, map[string]interface{}{"include": []string{"env", "service"}})

	require.NoError(t, a.setupTagFilters())
	pkglog.Flush()

	assert.Contains(t, output.String(), "logs_config.tag_filters: include is set but exclude is empty")
	assert.Contains(t, output.String(), "include is not an allowlist")
}

func TestGlobalIncludeWithExcludeDoesNotWarn(t *testing.T) {
	a, output := tagFilterWarningAgent(t, map[string]interface{}{
		"include": []string{"env", "service"},
		"exclude": []string{"dirname:*"},
	})

	require.NoError(t, a.setupTagFilters())
	pkglog.Flush()

	assert.NotContains(t, output.String(), "include is not an allowlist")
}

func TestMatchEverythingPatternFailsStartup(t *testing.T) {
	t.Cleanup(func() { tagfilter.SetGlobal(nil) })
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.tag_filters", map[string]interface{}{"exclude": []string{"*"}})
	a := &logAgent{log: logmock.New(t), config: cfg, sources: sources.NewLogSources()}

	err := a.setupTagFilters()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "matches every tag")
}
