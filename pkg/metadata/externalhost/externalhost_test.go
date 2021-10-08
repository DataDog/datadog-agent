// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package externalhost

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {
	// empty cache, empty payload
	p := *GetPayload()
	assert.Len(t, p, 0)

	host := "localhost"
	sourceType := "vsphere"
	tags := []string{"foo", "bar"}
	eTags := ExternalTags{sourceType: tags}

	// add one tag to the cache
	SetExternalTags(host, sourceType, tags)
	p = *GetPayload()
	assert.Len(t, p, 1)
	hTags := p[0]
	assert.Contains(t, hTags, host)
	assert.Contains(t, hTags, eTags)

	// GetPayload is supposed to empty the cache
	assert.Len(t, externalHostCache, 0)
}
