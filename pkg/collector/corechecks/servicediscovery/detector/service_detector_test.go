// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package detector

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
)

func TestFixup(t *testing.T) {
	// Test that generated name gets normalized
	meta := fixupMetadata(usm.ServiceMetadata{Name: "fOo"}, language.Go)
	assert.Equal(t, meta.Name, "foo")

	meta = fixupMetadata(usm.ServiceMetadata{Name: ""}, language.Go)
	assert.Equal(t, meta.Name, "unnamed-go-service")

	meta = fixupMetadata(usm.ServiceMetadata{Name: ""}, language.Unknown)
	assert.Equal(t, meta.Name, "unnamed-service")

	meta = fixupMetadata(usm.ServiceMetadata{Name: "foo", AdditionalNames: []string{"bAr", "  ", "*", "baz", "a"}}, language.Go)
	assert.Equal(t, "foo", meta.Name)
	assert.Equal(t, []string{"a", "bar", "baz"}, meta.AdditionalNames)
}
