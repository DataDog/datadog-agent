// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package servicediscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
)

func TestFixup(t *testing.T) {
	meta := fixupMetadata(usm.ServiceMetadata{Name: "Foo", DDService: "Bar"}, language.Go)
	assert.Equal(t, meta.Name, "foo")
	assert.Equal(t, meta.DDService, "bar")

	meta = fixupMetadata(usm.ServiceMetadata{Name: ""}, language.Go)
	assert.Equal(t, meta.Name, "unnamed-go-service")
	assert.Equal(t, meta.DDService, "")

	meta = fixupMetadata(usm.ServiceMetadata{Name: ""}, language.Unknown)
	assert.Equal(t, meta.Name, "unnamed-service")
	assert.Equal(t, meta.DDService, "")

	meta = fixupMetadata(usm.ServiceMetadata{Name: "foo", AdditionalNames: []string{"bar", "baz"}}, language.Go)
	assert.Equal(t, meta.Name, "foo-bar-baz")
}
