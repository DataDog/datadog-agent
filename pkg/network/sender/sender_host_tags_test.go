// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && (linux || (windows && npm))

package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/hosttags"
)

func TestDirectSenderHostTags(t *testing.T) {
	const taskARNTag = "task_arn:arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetGlobalTags([]string{"cluster_name:cluster"}, []string{taskARNTag}, nil, nil)

	t.Run("sidecar adds the global tags", func(t *testing.T) {
		d := &directSender{hostTagProvider: hosttags.NewHostTagProvider(), tagger: fakeTagger, sidecar: true}
		assert.ElementsMatch(t, []string{"cluster_name:cluster", taskARNTag}, d.hostTags())
	})

	t.Run("not a sidecar", func(t *testing.T) {
		d := &directSender{hostTagProvider: hosttags.NewHostTagProvider(), tagger: fakeTagger}
		assert.Empty(t, d.hostTags())
	})
}
