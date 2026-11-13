// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

const testClusterUID = "d3b07384-d9a7-4e1c-8b6f-6e2a1c7f0a11"

func TestPeerK8sNamespace(t *testing.T) {
	for _, tc := range []struct {
		host      string
		namespace string // "" means no tags
	}{
		// <service>.<namespace>.svc.<cluster domain>
		{host: "orders.shop.svc.cluster.local", namespace: "shop"},
		{host: "orders.shop.svc.cluster.local.", namespace: "shop"},
		{host: "Orders.Shop.SVC.Cluster.Local", namespace: "shop"},
		{host: "orders.shop.svc.example.org"},
		{host: "shop.svc.cluster.local"},
		// <service>.<namespace>.svc
		{host: "orders.shop.svc", namespace: "shop"},
		{host: "orders.svc"},
		// headless service pods
		{host: "pod-0.orders.shop.svc.cluster.local", namespace: "shop"},
		{host: "pod-0.orders.shop.svc.cluster.local.", namespace: "shop"},
		{host: "pod-0.orders.shop.svc", namespace: "shop"},
		{host: "x.pod-0.orders.shop.svc"},
		{host: "x.pod-0.orders.shop.svc.cluster.local"},
		{host: "pod_0.orders.shop.svc"},
		// ports
		{host: "orders.shop.svc:8080", namespace: "shop"},
		{host: "orders.shop.svc.cluster.local:5432", namespace: "shop"},
		{host: "orders.shop.svc:http"},
		{host: "orders.shop.svc:"},
		{host: "orders.shop.svc:80:80"},
		// schemes
		{host: "dns:///orders.shop.svc"},
		{host: "xds:///orders.shop.svc.cluster.local:443"},
		{host: "http://orders.shop.svc:80"},
		{host: "orders.shop.svc."},
		// short names: their namespace depends on the caller's DNS search path
		{host: "orders"},
		{host: "orders:8080"},
		{host: "orders.shop"},
		{host: "orders.shop:8080"},
		{host: "example.com"},
		{host: "localhost"},
		// IP addresses, raw and quantized
		{host: "10.1.2.3"},
		{host: "127.0.0.1"},
		{host: "::1"},
		{host: "2001:db8::1"},
		{host: "blocked-ip-address"},
		{host: "blocked-ip-address:5432"},
		{host: "blocked-ip-address.shop.svc"},
		{host: "blocked-ip-address.orders.shop.svc"},
		{host: "[::1]:8080"},
		{host: "10.1.2.3:8080"},
		// anything else
		{host: ""},
		{host: "."},
		{host: "db.us-east-1.rds.amazonaws.com"},
		{host: "orders_v2.shop.svc"},
		{host: "orders.shop_ns.svc"},
		{host: "-orders.shop.svc"},
		{host: "orders-.shop.svc"},
		{host: ".shop.svc"},
		{host: "orders..svc"},
		{host: "a,b.shop.svc"},
		{host: strings.Repeat("a", 64) + ".shop.svc"},
		{host: "orders." + strings.Repeat("a", 64) + ".svc"},
		{host: strings.Repeat("a", 63) + ".shop.svc", namespace: "shop"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			namespace, ok := peerK8sNamespace(tc.host)
			assert.Equal(t, tc.namespace, namespace)
			assert.Equal(t, tc.namespace != "", ok)
		})
	}
}

func TestPeerK8sScopeAppendTags(t *testing.T) {
	uidTag := peerK8sClusterUIDTagPrefix + testClusterUID
	scope := newPeerK8sScope(uidTag)

	t.Run("cluster-local hostname", func(t *testing.T) {
		in := []string{"peer.service:orders", "peer.hostname:orders.shop.svc", "db.system:postgres"}
		out := scope.appendTagsCopy(in)
		assert.Equal(t, []string{
			"db.system:postgres",
			"peer.hostname:orders.shop.svc",
			"peer.k8s.cluster.uid:" + testClusterUID,
			"peer.k8s.namespace:shop",
			"peer.service:orders",
		}, out)
		assert.Equal(t, []string{"peer.service:orders", "peer.hostname:orders.shop.svc", "db.system:postgres"}, in, "appendTagsCopy must not modify its input")
		assert.Equal(t, out, scope.appendTags(in))
	})
	t.Run("in place", func(t *testing.T) {
		in := make([]string, 1, 3)
		in[0] = "peer.hostname:orders.shop.svc"
		out := scope.appendTags(in)
		assert.Equal(t, []string{"peer.hostname:orders.shop.svc", uidTag, "peer.k8s.namespace:shop"}, out)
		assert.Same(t, &in[0], &out[0], "appendTags reuses the backing array")
	})
	t.Run("hostname sources by precedence", func(t *testing.T) {
		for _, tc := range []struct {
			in        []string
			namespace string // "" means unchanged
		}{
			{in: []string{"out.host:orders.shop.svc"}, namespace: "shop"},
			{in: []string{"network.destination.name:orders.shop.svc"}, namespace: "shop"},
			{in: []string{"server.address:orders.shop.svc.cluster.local"}, namespace: "shop"},
			{in: []string{"grpc.host:orders.shop.svc:50051"}, namespace: "shop"},
			{in: []string{"db.hostname:orders.shop.svc"}, namespace: "shop"},
			{in: []string{"network.destination.ip:blocked-ip-address"}},
			// the highest precedence source decides, even when it is not in-cluster
			{in: []string{"out.host:orders.shop.svc", "server.address:api.example.org"}},
			{in: []string{"peer.hostname:api.example.org", "out.host:orders.shop.svc"}},
			{in: []string{"peer.hostname:orders.shop", "out.host:orders.shop.svc"}},
			{in: []string{"out.host:orders.shop.svc", "dns.hostname:api.example.org"}, namespace: "shop"},
			{in: []string{"peer.hostname:orders.shop.svc", "out.host:other.ns.svc"}, namespace: "shop"},
			// empty values are skipped, like the backend does
			{in: []string{"peer.hostname:", "out.host:orders.shop.svc"}, namespace: "shop"},
			{in: []string{"peer.hostname:"}},
		} {
			out := scope.appendTagsCopy(tc.in)
			if tc.namespace == "" {
				assert.Equal(t, tc.in, out)
				continue
			}
			assert.Contains(t, out, uidTag, tc.in)
			assert.Contains(t, out, "peer.k8s.namespace:"+tc.namespace, tc.in)
			assert.Len(t, out, len(tc.in)+2)
		}
	})
	t.Run("unchanged", func(t *testing.T) {
		for _, in := range [][]string{
			nil,
			{"peer.service:orders"},
			{"peer.hostname:orders"},
			{"peer.hostname:orders.shop"},
			{"peer.hostname:blocked-ip-address"},
			{"peer.hostname:db.example.org"},
			{"peer.hostname:dns:///orders.shop.svc"},
			{"peer.hostname:orders.shop.svc", "peer.k8s.namespace:other"},
		} {
			out := scope.appendTags(in)
			assert.Equal(t, in, out)
			if len(in) > 0 {
				assert.Same(t, &in[0], &out[0])
			}
		}
	})
	t.Run("inactive", func(t *testing.T) {
		in := []string{"peer.hostname:orders.shop.svc"}
		assert.Equal(t, in, newPeerK8sScope("").appendTags(in))
	})
	t.Run("no allocation when not applicable", func(t *testing.T) {
		in := []string{"peer.hostname:db.example.org", "peer.service:db"}
		assert.Zero(t, testing.AllocsPerRun(100, func() { scope.appendTags(in) }))
		assert.Zero(t, testing.AllocsPerRun(100, func() { newPeerK8sScope(uidTag) }))
	})
}

func TestPeerHostnamePrefixesFollowRegistry(t *testing.T) {
	original, err := semantics.NewEmbeddedRegistry()
	require.NoError(t, err)
	t.Cleanup(func() { semantics.UpdateRegistry(original) })

	prefixes := peerHostnamePrefixes()
	require.NotEmpty(t, prefixes)
	assert.Equal(t, "peer.hostname:", prefixes[0])
	assert.Contains(t, prefixes, "out.host:")
	assert.Contains(t, prefixes, "server.address:")

	custom, err := semantics.NewRegistryFromJSON([]byte(`{"version":"test","metadata":{"content_hash":"h"},"concepts":{"peer.hostname":{"fallbacks":[{"name":"x.host","provider":"datadog","type":"string"},{"name":"out.host","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	semantics.UpdateRegistry(custom)
	assert.Equal(t, []string{"x.host:", "out.host:"}, peerHostnamePrefixes())

	scope := newPeerK8sScope(peerK8sClusterUIDTagPrefix + testClusterUID)
	assert.Contains(t, scope.appendTags([]string{"x.host:orders.shop.svc", "peer.hostname:api.example.org"}), "peer.k8s.namespace:shop")
}

func TestPeerK8sClusterUIDTag(t *testing.T) {
	uid := testClusterUID
	conf := &config.AgentConfig{
		PeerTagsAggregation: true,
		PeerTagsK8sScope:    true,
		KubeClusterUID:      func() string { return uid },
	}
	var c peerK8sClusterUIDTag
	assert.Equal(t, "peer.k8s.cluster.uid:"+testClusterUID, c.get(conf))
	assert.Zero(t, testing.AllocsPerRun(100, func() { c.get(conf) }))

	uid = "a2b07384-d9a7-4e1c-8b6f-6e2a1c7f0a11"
	assert.Equal(t, "peer.k8s.cluster.uid:"+uid, c.get(conf))

	uid = ""
	assert.Equal(t, "", c.get(conf), "unknown cluster UID")

	uid = testClusterUID
	conf.PeerTagsK8sScope = false
	assert.Equal(t, "", c.get(conf), "opted out")

	conf.PeerTagsK8sScope = true
	conf.PeerTagsAggregation = false
	assert.Equal(t, "", c.get(conf), "peer tags aggregation disabled")

	conf.PeerTagsAggregation = true
	conf.KubeClusterUID = nil
	assert.Equal(t, "", c.get(conf), "not in Kubernetes")
}
