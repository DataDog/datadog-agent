// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

const testTaskARNTag = "task_arn:arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"

type fakeGlobalTagger struct {
	tags        []string
	err         error
	called      bool
	cardinality types.TagCardinality
}

func (f *fakeGlobalTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	f.called = true
	f.cardinality = cardinality
	return f.tags, f.err
}

func TestECSTaskTagsOnECSFargate(t *testing.T) {
	tagger := &fakeGlobalTagger{tags: []string{"cluster_name:cluster", testTaskARNTag}}

	taskTags, ok := ecsTaskTags(ECS, tagger)
	assert.True(t, ok)
	assert.Equal(t, []string{"cluster_name:cluster", testTaskARNTag}, taskTags)
	assert.Equal(t, types.OrchestratorCardinality, tagger.cardinality, "task_arn is an orchestrator cardinality tag")
}

func TestECSTaskTagsTaggerError(t *testing.T) {
	taskTags, ok := ecsTaskTags(ECS, &fakeGlobalTagger{err: errors.New("not ready")})
	assert.True(t, ok)
	assert.Empty(t, taskTags)
}

func TestECSTaskTagsElsewhere(t *testing.T) {
	for _, orchestrator := range []OrchestratorName{EKS, ECSManagedInstances, Unknown} {
		t.Run(string(orchestrator), func(t *testing.T) {
			tagger := &fakeGlobalTagger{tags: []string{testTaskARNTag}}
			taskTags, ok := ecsTaskTags(orchestrator, tagger)
			assert.False(t, ok)
			assert.Nil(t, taskTags)
			assert.False(t, tagger.called, "the tagger isn't queried")
		})
	}
}

func TestHasTaskARN(t *testing.T) {
	assert.True(t, HasTaskARN([]string{"cluster_name:cluster", testTaskARNTag}))
	assert.False(t, HasTaskARN([]string{"cluster_name:cluster", "task_family:web"}))
	assert.False(t, HasTaskARN(nil))
}
