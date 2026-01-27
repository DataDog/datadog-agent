// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tmplvar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFallbackHost(t *testing.T) {
	ip, err := getFallbackHost(map[string]string{"bridge": "172.17.0.1"})
	assert.Equal(t, "172.17.0.1", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1"})
	assert.Equal(t, "172.17.0.1", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1", "bridge": "172.17.0.2"})
	assert.Equal(t, "172.17.0.2", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1", "bar": "172.17.0.2"})
	assert.Equal(t, "", ip)
	assert.NotNil(t, err)
}
