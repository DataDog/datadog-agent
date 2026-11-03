// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"testing"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

func TestEverySubscriptionCategoryUsesSameWireEncoder(t *testing.T) {
	for _, encoding := range []config.PathEncoding{config.PathEncodingOrigin, config.PathEncodingLiteralOpenConfig} {
		t.Run(string(encoding), func(t *testing.T) {
			c := &Client{cfg: Config{
				Profile: config.ProfileDefinition{
					Metrics: []config.MetricConfig{
						{Path: "/openconfig/interfaces/interface/state/counters/in-octets", Tags: map[string]string{"interface": "name"}},
						{Path: "rfc7951:/Cisco-IOS-XE-fib-oper:fib-oper-data/fib-ni-entry[instance-name=IPv4:Default]"},
					},
					Topology: config.DefaultOpenConfigLLDP(),
				},
				CollectTopology: true,
				SampleInterval:  10 * time.Second,
				Encoding:        config.Encoding(gnmipb.Encoding_PROTO),
				PathEncoding:    encoding,
			}}
			request, err := c.buildSubscribeRequest()
			require.NoError(t, err)
			list := request.GetSubscribe()
			require.Nil(t, list.Prefix, "origin belongs to individual paths, never both places")
			require.Equal(t, gnmipb.SubscriptionList_STREAM, list.Mode)
			require.Equal(t, gnmipb.Encoding_PROTO, list.Encoding)
			roots := make(map[string]bool)
			for _, sub := range list.Subscription {
				require.Equal(t, gnmipb.SubscriptionMode_SAMPLE, sub.Mode)
				require.Equal(t, uint64((10 * time.Second).Nanoseconds()), sub.SampleInterval)
				path := sub.Path
				if path.Origin == "rfc7951" {
					require.Equal(t, "Cisco-IOS-XE-fib-oper:fib-oper-data", path.Elem[0].Name)
					require.Equal(t, map[string]string{"instance-name": "IPv4:Default"}, path.Elem[1].Key)
					roots["native"] = true
					continue
				}
				if encoding == config.PathEncodingLiteralOpenConfig {
					require.Empty(t, path.Origin)
					require.Equal(t, "openconfig", path.Elem[0].Name)
					roots[path.Elem[1].Name] = true
				} else {
					require.Equal(t, "openconfig", path.Origin)
					require.NotEqual(t, "openconfig", path.Elem[0].Name)
					roots[path.Elem[0].Name] = true
				}
			}
			for _, root := range []string{"interfaces", "system", "components", "lldp", "native"} {
				require.True(t, roots[root], "missing subscription category/root %s", root)
			}
		})
	}
}

func TestNewRejectsBadSelectorBeforeConnecting(t *testing.T) {
	_, err := New(Config{
		Address: "127.0.0.1", Port: 57400, Username: "test", Password: "test",
		Profile: config.ProfileDefinition{Metrics: []config.MetricConfig{{Path: "/openconfig/interfaces//state"}}},
	})
	require.ErrorContains(t, err, "empty elements")
}
