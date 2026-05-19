// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectContentType(t *testing.T) {
	cases := []struct {
		name        string
		integration string
		path        string
		want        string
	}{
		{"redis conf canonical", "redis", "/etc/redis/redis.conf", "redis_conf"},
		{"redis conf uppercased", "redis", "/etc/REDIS/Redis.Conf", "redis_conf"},
		{"redis other .conf", "redis", "/etc/redis/extra.conf", "redis_conf"},
		{"redis yaml integration", "redis", "/etc/datadog-agent/conf.d/redis.d/conf.yaml", "yaml"},
		{"yml extension", "redis", "/tmp/x.yml", "yaml"},
		{"json extension", "anything", "/tmp/x.json", "json"},

		{"nginx conf rejected", "nginx", "/etc/nginx/nginx.conf", ""},
		{"postgres conf rejected", "postgres", "/etc/postgresql/postgresql.conf", ""},
		{"empty integration with conf rejected", "", "/etc/redis/redis.conf", ""},

		{"no extension", "redis", "/etc/redis/redis", ""},
		{"unknown ext", "redis", "/etc/redis/redis.ini", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, detectContentType(tc.integration, tc.path))
		})
	}
}
