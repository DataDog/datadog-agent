// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package helper

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
)

func TestGetGUIAddress(t *testing.T) {
	for _, tt := range []struct {
		host string
		port string
		want string
	}{
		{host: "localhost", port: "5002", want: "127.0.0.1:5002"},
		{host: "127.0.0.1", port: "5002", want: "127.0.0.1:5002"},
		{host: "127.0.0.2", port: "5003", want: "127.0.0.2:5003"},
		{host: "::1", port: "5002", want: "[::1]:5002"},
		{host: "::ffff:127.0.0.1", port: "5003", want: "[::ffff:127.0.0.1]:5003"},
		{host: "0.0.0.0", port: "5002"},
		{host: "::", port: "5002"},
		{host: "192.0.2.1", port: "5002"},
		{host: "2001:db8::1", port: "5002"},
		{host: "example.com", port: "5002"},
		{host: "", port: "5002"},
	} {
		t.Run(tt.host, func(t *testing.T) {
			cfg := nodetreemodel.NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // isolated config for testing
			cfg.SetDefault("GUI_host", tt.host)
			cfg.SetDefault("GUI_port", tt.port)
			cfg.BuildSchema()

			address, err := GetGUIAddress(cfg)
			if tt.want == "" {
				require.Error(t, err)
				require.Empty(t, address)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, address)
			host, _, err := net.SplitHostPort(address)
			require.NoError(t, err)
			require.True(t, net.ParseIP(host).IsLoopback())
		})
	}
}
