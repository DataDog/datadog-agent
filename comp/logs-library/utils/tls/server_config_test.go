// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tlsutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	t.Run("valid with cert and key", func(t *testing.T) {
		c := &ServerConfig{CertFile: "/cert", KeyFile: "/key"}
		assert.NoError(t, c.Validate())
	})

	t.Run("missing key_file", func(t *testing.T) {
		c := &ServerConfig{CertFile: "/cert"}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cert_file and key_file")
	})

	t.Run("missing cert_file", func(t *testing.T) {
		c := &ServerConfig{KeyFile: "/key"}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cert_file and key_file")
	})
}
