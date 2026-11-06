// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ottl

import (
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestSourceAdmissionCompilesBeforePublication(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		collection := sources.NewLogSources(PrepareSource)
		done := make(chan struct{})
		defer close(done)
		added, _ := collection.SubscribeForType(config.FileType, done, done)
		source := sources.NewLogSource("valid", &config.LogsConfig{Type: config.FileType, Path: "app.log", ProcessingRules: []*config.ProcessingRule{
			{Name: "keep", Type: config.IncludeAtMatchOTTL, Pattern: `attributes["env"] == "prod"`},
			{Name: "mask", Type: config.MaskOTTLTransform, Pattern: `set(attributes["password"], "***")`},
		}})
		go collection.AddSource(source)
		require.Same(t, source, <-added)
		first, second := source.Config.ProcessingRules[0].OTTL, source.Config.ProcessingRules[1].OTTL
		require.NotNil(t, first)
		require.NotNil(t, second)
		c := first.NewContext([]byte(`{"env":"prod","password":"secret"}`))
		defer c.Release()
		match, err := first.Evaluate(c)
		require.NoError(t, err)
		require.True(t, match)
		changed, err := second.Evaluate(c)
		require.NoError(t, err)
		require.True(t, changed)
		require.JSONEq(t, `{"env":"prod","password":"***"}`, string(c.Content()))
		require.NoError(t, PrepareSource(source))
		require.Same(t, first, source.Config.ProcessingRules[0].OTTL, "compiled rules are shared across pipelines and inherited sources")
		require.Same(t, second, source.Config.ProcessingRules[1].OTTL)
	})
}

func TestInvalidOTTLSourceNeverReachesLaunchers(t *testing.T) {
	for _, kind := range []string{config.IncludeAtMatchOTTL, config.ExcludeAtMatchOTTL, config.MaskOTTLTransform} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				collection := sources.NewLogSources(PrepareSource)
				invalid := sources.NewLogSource("invalid", &config.LogsConfig{Type: config.FileType, Path: "app.log", ProcessingRules: []*config.ProcessingRule{{Name: "bad_rule", Type: kind, Pattern: "invalid OTTL syntax"}}})
				collection.AddSource(invalid)
				require.ErrorContains(t, ValidateLogsConfig(invalid.Config), "bad_rule")
				require.Contains(t, invalid.Status().GetError(), "bad_rule")
				require.Contains(t, collection.GetSources(), invalid, "rejected sources remain visible in status")
				valid := sources.NewLogSource("valid", &config.LogsConfig{Type: config.FileType, Path: "other.log"})
				collection.AddSource(valid)
				done := make(chan struct{})
				defer close(done)
				all, _ := collection.SubscribeAll(done, done)
				typed, _ := collection.SubscribeForType(config.FileType, done, done)
				added := collection.GetAddedForType(config.FileType, done)
				for _, ch := range []chan *sources.LogSource{all, typed, added} {
					require.Same(t, valid, <-ch, "backfill must exclude rejected sources")
				}
				// Rejection with active subscribers must neither block nor notify them.
				collection.AddSource(invalid)
				collection.RemoveSource(invalid)
				synctest.Wait()
				for _, ch := range []chan *sources.LogSource{all, typed, added} {
					select {
					case unexpected := <-ch:
						t.Fatalf("rejected source was published: %s", unexpected.Name)
					default:
					}
				}
			})
		})
	}
}
