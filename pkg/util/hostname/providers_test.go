// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func dummyProvider(ctx context.Context, options map[string]interface{}) (string, error) {
	return "dummy-hostname", nil
}

func dummyErrorProvider(ctx context.Context, options map[string]interface{}) (string, error) {
	return "", fmt.Errorf("Some error")
}

func dummyInvalidProvider(ctx context.Context, options map[string]interface{}) (string, error) {
	return "some invalid hostname", nil
}

func TestRegisterHostnameProvider(t *testing.T) {
	RegisterHostnameProvider("dummy", dummyProvider)
	assert.Contains(t, providerCatalog, "dummy")
	delete(providerCatalog, "dummy")
}

func TestGetProvider(t *testing.T) {
	RegisterHostnameProvider("dummy", dummyProvider)
	defer delete(providerCatalog, "dummy")
	assert.NotNil(t, GetProvider("dummy"))
	assert.Nil(t, GetProvider("does not exists"))
}

func TestGetHostname(t *testing.T) {
	RegisterHostnameProvider("dummy", dummyProvider)
	defer delete(providerCatalog, "dummy")

	name, err := GetHostname(context.Background(), "dummy", nil)
	assert.NoError(t, err)
	assert.Equal(t, "dummy-hostname", name)
}

func TestGetHostnameUnknown(t *testing.T) {
	_, err := GetHostname(context.Background(), "dummy", nil)
	assert.Error(t, err)
}

func TestGetHostnameError(t *testing.T) {
	RegisterHostnameProvider("dummy", dummyErrorProvider)
	defer delete(providerCatalog, "dummy")

	_, err := GetHostname(context.Background(), "dummy", nil)
	assert.Error(t, err)
}

func TestGetHostnameInvalid(t *testing.T) {
	RegisterHostnameProvider("dummy", dummyInvalidProvider)
	defer delete(providerCatalog, "dummy")

	_, err := GetHostname(context.Background(), "dummy", nil)
	assert.Error(t, err)
}
