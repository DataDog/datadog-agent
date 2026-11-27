// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortImageIsLearnedOnce(t *testing.T) {
	p := New()
	assert.Empty(t, p.ShortImage())

	p.PrepareNewVersion("v1", []string{"image_name:gcr.io/org/app"}, 4, 1)
	assert.Empty(t, p.ShortImage())

	p.PrepareNewVersion("v1", []string{"image_tag:v1", "short_image:app"}, 4, 2)
	assert.Equal(t, "app", p.ShortImage())

	// The cached value stays even if the version tags are later rewritten.
	for _, ctx := range p.versionContexts {
		ctx.Tags = []string{"short_image:other"}
	}
	assert.Equal(t, "app", p.ShortImage())

	p.PrepareNewVersion("v2", []string{"short_image:other"}, 4, 3)
	assert.Equal(t, "app", p.ShortImage())
}

func TestShortImageLearnedFromAddedVersionContext(t *testing.T) {
	p := New()
	p.AddVersionContext("v1", &VersionContext{Tags: []string{"short_image:redis"}})
	assert.Equal(t, "redis", p.ShortImage())

	p.AddVersionContext("v2", &VersionContext{Tags: []string{"short_image:other"}})
	assert.Equal(t, "redis", p.ShortImage())
}

func TestShortImageRestoredFromProtobuf(t *testing.T) {
	p := newProfileWithSelector(t)
	p.PrepareNewVersion("1.2.3", []string{"short_image:nginx", "image_tag:1.2.3"}, 4, 1)

	buf, err := p.EncodeSecurityProfileProtobuf()
	require.NoError(t, err)

	loaded := New()
	require.NoError(t, loaded.DecodeSecurityProfileProtobuf(buf))
	assert.Equal(t, "nginx", loaded.ShortImage())
}
