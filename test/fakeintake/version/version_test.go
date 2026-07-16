// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImageURL_OverrideUnset(t *testing.T) {
	t.Setenv("FAKEINTAKE_IMAGE_OVERRIDE", "")

	assert.Equal(t, "public.ecr.aws/datadog/fakeintake:"+Tag, ImageURL("public.ecr.aws/datadog/fakeintake"))
}

func TestImageURL_OverrideSet(t *testing.T) {
	t.Setenv("FAKEINTAKE_IMAGE_OVERRIDE", "public.ecr.aws/datadog/fakeintake:v1234abcd")

	assert.Equal(t, "public.ecr.aws/datadog/fakeintake:v1234abcd", ImageURL("public.ecr.aws/datadog/fakeintake"))
}

func TestImageURL_ComposesRegistryAndTag(t *testing.T) {
	t.Setenv("FAKEINTAKE_IMAGE_OVERRIDE", "")

	assert.Equal(t, "registry.datadoghq.com/fakeintake:"+Tag, ImageURL("registry.datadoghq.com/fakeintake"))
}

func TestTag_IsTrimmed(t *testing.T) {
	assert.NotContains(t, Tag, "\n")
	assert.NotContains(t, Tag, " ")
	assert.NotEmpty(t, Tag)
}
