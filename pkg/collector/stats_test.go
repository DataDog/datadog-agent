// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCollectorErrors(t *testing.T) {
	ce := newCollectorErrors()
	assert.Len(t, ce.loader, 0)
	assert.Len(t, ce.run, 0)
}
func TestSetLoaderError(t *testing.T) {
	ce := newCollectorErrors()
	ce.setLoaderError("aCheck", "aLoader", "anError")
	ce.setLoaderError("anotherCheck", "aLoader", "anError")

	assert.Len(t, ce.loader, 2) // 2 checks for this loader
	assert.Len(t, ce.loader["aCheck"], 1)
	assert.Len(t, ce.loader["anotherCheck"], 1)
}

func TestRemoveLoaderErrors(t *testing.T) {
	ce := newCollectorErrors()
	ce.setLoaderError("aCheck", "aLoader", "anError")
	ce.removeLoaderErrors("aCheck")

	assert.Len(t, ce.loader, 0)
}

func TestGetLoaderErrors(t *testing.T) {
	ce := newCollectorErrors()
	ce.setLoaderError("aCheck", "aLoader", "anError")
	errs := ce.getLoaderErrors()
	assert.Len(t, errs, 1)
}
