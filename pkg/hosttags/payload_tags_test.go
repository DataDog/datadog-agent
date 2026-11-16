// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package hosttags

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/config/env"
)

const testTaskARNTag = "task_arn:arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"

type fakeGlobalTagger struct {
	tags        []string
	err         error
	cardinality types.TagCardinality
}

func (f *fakeGlobalTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	f.cardinality = cardinality
	return f.tags, f.err
}

func providerWithTags(tags ...string) *HostTagProvider {
	return &HostTagProvider{hostTags: tags}
}

func TestPayloadTagsOnECSFargate(t *testing.T) {
	env.SetFeatures(t, env.ECSFargate)
	tagger := &fakeGlobalTagger{tags: []string{"cluster_name:cluster", testTaskARNTag}}

	p := NewPayloadTags(providerWithTags("env:prod"), tagger)
	assert.Equal(t, []string{"env:prod", "cluster_name:cluster", testTaskARNTag}, p.Get())
	assert.Equal(t, types.OrchestratorCardinality, tagger.cardinality, "task_arn is an orchestrator cardinality tag")
}

func TestPayloadTagsGlobalTagsError(t *testing.T) {
	env.SetFeatures(t, env.ECSFargate)
	p := NewPayloadTags(providerWithTags("env:prod"), &fakeGlobalTagger{err: errors.New("not ready")})
	assert.Equal(t, []string{"env:prod"}, p.Get())
}

func TestPayloadTagsNotECSFargate(t *testing.T) {
	for _, feature := range []env.Feature{env.EKSFargate, env.ECSManagedInstances} {
		t.Run(string(feature), func(t *testing.T) {
			env.SetFeatures(t, feature)
			p := NewPayloadTags(providerWithTags("env:prod"), &fakeGlobalTagger{tags: []string{testTaskARNTag}})
			assert.Equal(t, []string{"env:prod"}, p.Get())
		})
	}
}

func TestPayloadTagsReturnsACopy(t *testing.T) {
	env.SetFeatures(t, env.ECSFargate)
	provider := &HostTagProvider{hostTags: make([]string, 1, 4)}
	provider.hostTags[0] = "env:prod"
	p := NewPayloadTags(provider, &fakeGlobalTagger{tags: []string{testTaskARNTag}})

	tags := p.Get()
	tags[0] = "changed"
	assert.Equal(t, []string{"env:prod"}, provider.hostTags)
	assert.Equal(t, []string{"env:prod", ""}, provider.hostTags[:2], "the provider's spare capacity must not be written")
}
