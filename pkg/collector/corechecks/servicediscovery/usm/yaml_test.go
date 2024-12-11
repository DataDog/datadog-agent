// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewYamlSource(t *testing.T) {
	reader := strings.NewReader(`
logging:
  level:
    org.springframework: ERROR
    com.mkyong: DEBUG

spring:
  profiles:
    active: dev
  main:
    banner-mode: off

email: yaml@mkyong.com
thread-pool: 10

wordpress:
  menus:
    - title: Home
      name: Home
      path: /
    - title: About
      name: About
      path: /about
  themes:
    default-folder: /wp-content/themes/mkyong
  servers:
    - ip: 127.0.0.1
      path: /dev1
    - ip: 127.0.0.2
      path: /dev2
    - ip: 127.0.0.3
      path: /dev3
`)
	source, err := newYamlSource(reader)
	require.NoError(t, err)
	val, ok := source.Get("logging.level.org.springframework")
	require.True(t, ok)
	require.Equal(t, val, "ERROR")
}
