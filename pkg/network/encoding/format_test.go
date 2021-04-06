package encoding

import (
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatRouteIdx(t *testing.T) {

	tests := []struct {
		desc                string
		via                 *network.Via
		routesIn, routesOut map[string]RouteIdx
		idx                 int32
	}{
		{
			desc: "nil via",
			via:  nil,
			idx:  -1,
		},
		{
			desc: "empty via",
			via:  &network.Via{},
			idx:  -1,
		},
		{
			desc:     "empty routes, non-nil via",
			via:      &network.Via{Subnet: network.Subnet{Alias: "foo"}},
			idx:      0,
			routesIn: map[string]RouteIdx{},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
		},
		{
			desc: "non-empty routes, non-nil via with existing alias",
			via:  &network.Via{Subnet: network.Subnet{Alias: "foo"}},
			idx:  0,
			routesIn: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
		},
		{
			desc: "non-empty routes, non-nil via with non-existing alias",
			via:  &network.Via{Subnet: network.Subnet{Alias: "bar"}},
			idx:  1,
			routesIn: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
				"bar": {Idx: 1, Route: model.Route{Subnet: &model.Subnet{Alias: "bar"}}},
			},
		},
	}

	for _, te := range tests {
		t.Run(te.desc, func(t *testing.T) {
			idx := formatRouteIdx(te.via, te.routesIn)
			require.Equal(t, te.idx, idx)
			require.Len(t, te.routesIn, len(te.routesIn), "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
			for k, v := range te.routesOut {
				otherv, ok := te.routesIn[k]
				require.True(t, ok, "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
				require.Equal(t, v, otherv, "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
			}
		})
	}
}

func TestFormatHTTPStats(t *testing.T) {
	key1 := http.NewKey(
		util.AddressFromString("10.1.1.1"),
		util.AddressFromString("10.2.2.2"),
		1000,
		9000,
		"/path-1",
	)

	key2 := http.NewKey(
		util.AddressFromString("10.1.1.1"),
		util.AddressFromString("10.2.2.2"),
		1000,
		9000,
		"/path-2",
	)

	stats1 := http.RequestStats{
		{Count: 1},
		{Count: 2},
		{Count: 3},
		{Count: 4},
		{Count: 5},
	}

	stats2 := http.RequestStats{
		{Count: 6},
		{Count: 7},
		{Count: 8},
		{Count: 9},
		{Count: 10},
	}

	httpData := map[http.Key]http.RequestStats{
		key1: stats1,
		key2: stats2,
	}

	formatted := FormatHTTPStats(httpData)
	aggregatedKey := http.NewKey(
		util.AddressFromString("10.1.1.1"),
		util.AddressFromString("10.2.2.2"),
		1000,
		9000,
		"",
	)

	assert.Len(t, formatted, 1)
	assert.Contains(t, formatted, aggregatedKey)
	statsByPath := formatted[aggregatedKey].ByPath

	for key, stats := range httpData {
		path := key.Path
		assert.Contains(t, statsByPath, path)
		statsByCode := statsByPath[path].StatsByResponseStatus
		require.Len(t, statsByCode, http.NumStatusClasses)
		for i := 0; i < http.NumStatusClasses; i++ {
			assert.Equal(t, stats[i].Count, int(statsByCode[i].Count))
		}
	}
}
