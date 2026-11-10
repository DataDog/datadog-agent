// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

func TestEverySubscriptionCategoryUsesSameWireEncoder(t *testing.T) {
	for _, encoding := range []gnmipb.Encoding{gnmipb.Encoding_PROTO, gnmipb.Encoding_JSON_IETF} {
		t.Run(encoding.String(), func(t *testing.T) {
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
				Encoding:        config.Encoding(encoding),
			}}
			request, err := c.buildSubscribeRequest()
			require.NoError(t, err)
			list := request.GetSubscribe()
			require.Nil(t, list.Prefix, "origin belongs to individual paths, never both places")
			require.Equal(t, gnmipb.SubscriptionList_STREAM, list.Mode)
			require.Equal(t, encoding, list.Encoding)
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
				require.Equal(t, "openconfig", path.Origin)
				require.NotEqual(t, "openconfig", path.Elem[0].Name)
				roots[path.Elem[0].Name] = true
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

func TestNotificationOriginsSurviveCacheLookupAndDelete(t *testing.T) {
	for _, originOnPrefix := range []bool{true, false} {
		t.Run(fmt.Sprint("prefix=", originOnPrefix), func(t *testing.T) {
			c := &Client{cache: newCache(), opt: options{clock: clock.NewMock()}}
			keys := map[string]string{"name": "ethernet-1/1"}
			for i, origin := range []string{"", "openconfig", "native-a", "native-b"} {
				prefix := &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "interfaces"}, {Name: "interface", Key: keys}}}
				path := &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "state"}, {Name: "counters"}}}
				if originOnPrefix {
					prefix.Origin = origin
				} else {
					path.Origin = origin
				}
				c.applyUpdate(c.cache, prefix, &gnmipb.Update{
					Path: path,
					Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(fmt.Sprintf(`{"in-octets":%d}`, i+1))}},
				}, time.Unix(100, 0))
			}
			require.Len(t, c.Snapshot(), 8, "containers and leaves retain all origins")
			for i, origin := range []string{"", "openconfig", "native-a", "native-b"} {
				path := "/interfaces/interface/state/counters/in-octets"
				if origin != "" {
					path = origin + ":" + path
				}
				entry, ok := c.Get(path, keys)
				require.True(t, ok, path)
				require.EqualValues(t, i+1, entry.Value)
			}
			c.applyDelete(c.cache, nil, &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "interfaces"}}})
			require.Len(t, c.Snapshot(), 6, "originless delete must not erase explicit origins")
			c.applyDelete(c.cache, &gnmipb.Path{Origin: "native-a", Elem: []*gnmipb.PathElem{{Name: "interfaces"}}}, &gnmipb.Path{})
			require.Len(t, c.Snapshot(), 4)
			_, ok := c.Get("native-a:/interfaces/interface/state/counters/in-octets", keys)
			require.False(t, ok)
			entry, ok := c.Get("native-b:/interfaces/interface/state/counters/in-octets", keys)
			require.True(t, ok)
			require.EqualValues(t, 4, entry.Value)
		})
	}
}

func TestJSONModuleNamesAndNativeOriginsDoNotCollapse(t *testing.T) {
	c := &Client{cache: newCache(), opt: options{clock: clock.NewMock()}}
	for _, origin := range []string{"native-a", "native-b"} {
		c.applyUpdate(c.cache, &gnmipb.Path{Origin: origin}, &gnmipb.Update{
			Path: &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "vendor:system"}, {Name: "state"}}},
			Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"vendor-a:hostname":"a","vendor-b:hostname":"b"}`)}},
		}, time.Unix(100, 0))
	}
	require.Len(t, c.Snapshot(), 6)
	for _, origin := range []string{"native-a", "native-b"} {
		for _, module := range []string{"a", "b"} {
			entry, ok := c.Get(origin+":/vendor:system/state/vendor-"+module+":hostname", nil)
			require.True(t, ok)
			require.Equal(t, module, entry.Value)
		}
	}
	for _, path := range []string{"/system/state/hostname", "openconfig:/system/state/hostname", "native-a:/system/state/hostname"} {
		_, ok := c.Get(path, nil)
		require.False(t, ok, path)
	}
}

func TestOpenConfigCacheLookupCompatibility(t *testing.T) {
	for _, wire := range []*gnmipb.Path{
		{Origin: "openconfig", Elem: []*gnmipb.PathElem{{Name: "system"}, {Name: "state"}}},
		{Elem: []*gnmipb.PathElem{{Name: "system"}, {Name: "state"}}},
		{Elem: []*gnmipb.PathElem{{Name: "openconfig"}, {Name: "system"}, {Name: "state"}}},
		{Elem: []*gnmipb.PathElem{{Name: "openconfig-system:system"}, {Name: "state"}}},
	} {
		c := &Client{cache: newCache(), opt: options{clock: clock.NewMock()}}
		c.applyUpdate(c.cache, nil, &gnmipb.Update{Path: wire,
			Val: &gnmipb.TypedValue{Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"openconfig-system:hostname":"router"}`)}},
		}, time.Unix(100, 0))
		for _, selector := range []string{"openconfig:/system/state/hostname", "/openconfig/system/state/hostname", "/system/state/hostname"} {
			entry, ok := c.Get(selector, nil)
			require.True(t, ok, selector)
			require.Equal(t, "router", entry.Value)
		}
	}
}

func TestJSONRootUpdatesRetainOrigins(t *testing.T) {
	c := &Client{cache: newCache(), opt: options{clock: clock.NewMock()}}
	for _, origin := range []string{"native-a", "native-b"} {
		c.applyUpdate(c.cache, &gnmipb.Path{Origin: origin}, &gnmipb.Update{
			Path: &gnmipb.Path{},
			Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"vendor:system":{"state":{"vendor:hostname":"router"}}}`)}},
		}, time.Unix(100, 0))
	}
	require.Len(t, c.Snapshot(), 4)
	seen := make(map[string]bool)
	for _, cached := range c.Snapshot() {
		seen[cached.Key.Path] = true
	}
	for _, origin := range []string{"native-a", "native-b"} {
		require.True(t, seen[origin+":/"])
		entry, ok := c.Get(origin+":/vendor:system/state/vendor:hostname", nil)
		require.True(t, ok)
		require.Equal(t, "router", entry.Value)
	}
	c.applyDelete(c.cache, &gnmipb.Path{Origin: "native-a"}, &gnmipb.Path{})
	require.Len(t, c.Snapshot(), 2)
	_, ok := c.Get("native-b:/vendor:system/state/vendor:hostname", nil)
	require.True(t, ok)
}

func TestRepeatedSegmentKeysRetainParentIdentity(t *testing.T) {
	c := &Client{cache: newCache(), opt: options{clock: clock.NewMock()}}
	for i, selector := range []string{"native:/entry[name=A]/entry[name=B]/counter", "native:/entry[name=C]/entry[name=B]/counter"} {
		path, err := config.CompilePath(selector, nil)
		require.NoError(t, err)
		c.applyUpdate(c.cache, nil, &gnmipb.Update{Path: path, Val: &gnmipb.TypedValue{Value: &gnmipb.TypedValue_IntVal{IntVal: int64(i + 1)}}}, time.Unix(100, 0))
	}
	require.Len(t, c.Snapshot(), 2)
	for i, selector := range []string{"native:/entry[name=A]/entry[name=B]/counter", "native:/entry[name=C]/entry[name=B]/counter"} {
		entry, ok := c.Get(selector, nil)
		require.True(t, ok)
		require.EqualValues(t, i+1, entry.Value)
	}
	_, ok := c.Get("native:/entry[name=missing]/entry[name=B]/counter", nil)
	require.False(t, ok)
}

func TestOpenConfigQualifiedLeafDelete(t *testing.T) {
	for _, origin := range []string{"openconfig", "", "native"} {
		t.Run(origin, func(t *testing.T) {
			c := &Client{cache: newCache(), opt: options{clock: clock.NewMock()}}
			c.applyUpdate(c.cache, &gnmipb.Path{Origin: origin}, &gnmipb.Update{
				Path: &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "system"}, {Name: "state"}}},
				Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"openconfig-system:hostname":"router"}`)}},
			}, time.Unix(100, 0))
			selector := "/system/state/openconfig-system:hostname"
			if origin != "" {
				selector = origin + ":" + selector
			}
			_, ok := c.Get(selector, nil)
			require.True(t, ok)
			c.applyDelete(c.cache, &gnmipb.Path{Origin: origin}, &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "system"}, {Name: "state"}, {Name: "hostname"}}})
			_, ok = c.Get(selector, nil)
			require.Equal(t, origin == "native", ok, "only bounded OpenConfig names are equivalent")
		})
	}
}

func TestScopedSnapshotKeysAreCloned(t *testing.T) {
	c := &Client{cache: newCache(), opt: options{clock: clock.NewMock()}}
	selector := "native:/entry[name=A]/entry[name=B]/counter"
	path, err := config.CompilePath(selector, nil)
	require.NoError(t, err)
	c.applyUpdate(c.cache, nil, &gnmipb.Update{Path: path, Val: &gnmipb.TypedValue{Value: &gnmipb.TypedValue_IntVal{IntVal: 1}}}, time.Unix(100, 0))
	snapshot := c.Snapshot()
	require.Len(t, snapshot, 1)
	snapshot[0].Key.ElementKeys[0]["name"] = "changed"
	_, ok := c.Get(selector, nil)
	require.True(t, ok)
}

func TestOpenConfigEquivalentUpdatesShareIdentity(t *testing.T) {
	for _, origin := range []string{"", "openconfig", "native"} {
		c := &Client{cache: newCache(), opt: options{clock: clock.NewMock()}}
		for i, root := range []string{"openconfig-system:system", "system"} {
			c.applyUpdate(c.cache, &gnmipb.Path{Origin: origin}, &gnmipb.Update{
				Path: &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: root}, {Name: "state"}, {Name: "hostname"}}},
				Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_IntVal{IntVal: int64(i)}},
			}, time.Unix(100, 0))
		}
		if origin == "native" {
			require.Len(t, c.Snapshot(), 2, "native qualified schema names remain distinct")
		} else {
			require.Len(t, c.Snapshot(), 1)
			entry, ok := c.Get("openconfig:/system/state/hostname", nil)
			require.True(t, ok)
			require.EqualValues(t, 1, entry.Value)
			c.applyDelete(c.cache, &gnmipb.Path{Origin: origin}, &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "openconfig-system:system"}, {Name: "state"}}})
			require.Empty(t, c.Snapshot())
		}
	}
}
