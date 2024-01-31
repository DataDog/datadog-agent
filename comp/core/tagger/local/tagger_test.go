// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"context"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTagBuilder(t *testing.T) {

	store := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	tagger := NewTagger(store, tagstore.NewTagStore())
	tagger.Start(context.Background())
	defer tagger.Stop()

	tagger.tagStore.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:       "entity_name",
			Source:       "stream",
			LowCardTags:  []string{"low1"},
			HighCardTags: []string{"high"},
		},
		{
			Entity:      "entity_name",
			Source:      "pull",
			LowCardTags: []string{"low2"},
		},
	})

	tb := tagset.NewHashlessTagsAccumulator()
	err := tagger.AccumulateTagsFor("entity_name", collectors.HighCardinality, tb)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"high", "low1", "low2"}, tb.Get())
}

func TestInjectHostTags(t *testing.T) {

	overrides := map[string]interface{}{
		"tags":                   []string{"tag1:value1", "tag2", "tag3"},
		"expected_tags_duration": "10m",
	}

	store := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		fx.Replace(config.MockParams{Overrides: overrides}),
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))

	testClock := clock.NewMock()
	testClock.Set(time.Now()) // Set the mock to now since the tag expire date is based off of the real clock time.

	tagStore := tagstore.NewTagStoreWithClock(testClock)
	tagger := NewTagger(store, tagStore)
	tagger.Start(context.Background())
	defer tagger.Stop()

	tb := tagset.NewHashlessTagsAccumulator()

	tagger.AccumulateTagsFor(collectors.HostEntityID, collectors.LowCardinality, tb)
	assert.ElementsMatch(t, []string{"tag1:value1", "tag2", "tag3"}, tb.Get())

	tb.Reset()
	// Advance the clock by 11 minutes so prune will expire the tags.
	testClock.Add(11 * time.Minute)
	// Force a prune to remove the expired tags (this usually happens on a long timer).
	tagStore.Prune()

	tagger.AccumulateTagsFor(collectors.HostEntityID, collectors.LowCardinality, tb)
	assert.Empty(t, tb.Get())
}
