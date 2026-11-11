// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

const testClusterUID = "d3b07384-d9a7-4e1c-8b6f-6e2a1c7f0a11"

func TestPeerK8sNamespace(t *testing.T) {
	for _, tc := range []struct {
		host      string
		caller    string
		namespace string // "" means no tags
	}{
		// <service>.<namespace>.svc.<cluster domain>
		{host: "orders.shop.svc.cluster.local", caller: "caller", namespace: "shop"},
		{host: "orders.shop.svc.cluster.local.", caller: "caller", namespace: "shop"},
		{host: "Orders.Shop.SVC.Cluster.Local", caller: "caller", namespace: "shop"},
		{host: "orders.shop.svc.example.org"},
		{host: "pod-0.orders.shop.svc.cluster.local"},
		{host: "shop.svc.cluster.local"},
		// <service>.<namespace>.svc
		{host: "orders.shop.svc", caller: "caller", namespace: "shop"},
		{host: "orders.shop.svc."},
		{host: "pod-0.orders.shop.svc"},
		// <service>.<namespace>
		{host: "orders.shop", caller: "caller", namespace: "shop"},
		{host: "orders.shop", namespace: "shop"},
		{host: "orders.shop."},
		{host: "orders.svc", namespace: "svc"},
		{host: "example.com", namespace: "com"}, // indistinguishable from a service in namespace com
		// <service>
		{host: "orders", caller: "caller", namespace: "caller"},
		{host: "ORDERS", caller: "caller", namespace: "caller"},
		{host: "orders"},
		{host: "orders."},
		{host: "orders", caller: "Bad_Namespace"},
		{host: "localhost", caller: "caller"},
		// IP addresses, raw and quantized
		{host: "10.1.2.3", caller: "caller"},
		{host: "127.0.0.1", caller: "caller"},
		{host: "::1", caller: "caller"},
		{host: "2001:db8::1", caller: "caller"},
		{host: "blocked-ip-address", caller: "caller"},
		{host: "blocked-ip-address:5432", caller: "caller"},
		{host: "ip-blocked-ip-address", caller: "caller"},
		{host: "blocked-ip-address.shop", caller: "caller"},
		// anything else
		{host: "", caller: "caller"},
		{host: ".", caller: "caller"},
		{host: "db.us-east-1.rds.amazonaws.com", caller: "caller"},
		{host: "orders:8080", caller: "caller"},
		{host: "orders.shop:8080", caller: "caller"},
		{host: "orders_v2", caller: "caller"},
		{host: "orders.shop_ns", caller: "caller"},
		{host: "-orders", caller: "caller"},
		{host: "orders-", caller: "caller"},
		{host: ".shop", caller: "caller"},
		{host: "orders.", caller: "caller"},
		{host: "orders..svc", caller: "caller"},
		{host: "a,b", caller: "caller"},
		{host: strings.Repeat("a", 64), caller: "caller"},
		{host: "orders." + strings.Repeat("a", 64), caller: "caller"},
		{host: strings.Repeat("a", 63) + ".shop", caller: "caller", namespace: "shop"},
	} {
		t.Run(tc.host+"/"+tc.caller, func(t *testing.T) {
			namespace, ok := peerK8sNamespace(tc.host, tc.caller)
			assert.Equal(t, tc.namespace, namespace)
			assert.Equal(t, tc.namespace != "", ok)
		})
	}
}

func TestPeerK8sScopeAppendTags(t *testing.T) {
	scope := peerK8sScope{clusterUIDTag: peerK8sClusterUIDTagPrefix + testClusterUID, callerNamespace: "caller"}

	t.Run("cluster-local hostname", func(t *testing.T) {
		in := []string{"peer.service:orders", "peer.hostname:orders.shop", "db.system:postgres"}
		out := scope.appendTags(in)
		assert.Equal(t, []string{
			"db.system:postgres",
			"peer.hostname:orders.shop",
			"peer.k8s.cluster.uid:" + testClusterUID,
			"peer.k8s.namespace:shop",
			"peer.service:orders",
		}, out)
		assert.Equal(t, []string{"peer.service:orders", "peer.hostname:orders.shop", "db.system:postgres"}, in, "input must not be modified")
	})
	t.Run("bare hostname", func(t *testing.T) {
		out := scope.appendTags([]string{"peer.hostname:orders"})
		assert.Equal(t, []string{"peer.hostname:orders", "peer.k8s.cluster.uid:" + testClusterUID, "peer.k8s.namespace:caller"}, out)
	})
	t.Run("unchanged", func(t *testing.T) {
		for _, in := range [][]string{
			nil,
			{"peer.service:orders"},
			{"out.host:orders.shop"},
			{"peer.hostname:blocked-ip-address"},
			{"peer.hostname:db.example.org"},
			{"peer.hostname:orders.shop", "peer.k8s.namespace:other"},
		} {
			out := scope.appendTags(in)
			assert.Equal(t, in, out)
			if len(in) > 0 {
				assert.Same(t, &in[0], &out[0])
			}
		}
	})
	t.Run("bare hostname without caller namespace", func(t *testing.T) {
		in := []string{"peer.hostname:orders"}
		assert.Equal(t, in, peerK8sScope{clusterUIDTag: scope.clusterUIDTag}.appendTags(in))
	})
	t.Run("inactive", func(t *testing.T) {
		in := []string{"peer.hostname:orders.shop"}
		assert.Equal(t, in, peerK8sScope{callerNamespace: "caller"}.appendTags(in))
	})
	t.Run("no allocation when not applicable", func(t *testing.T) {
		in := []string{"peer.hostname:db.example.org", "peer.service:db"}
		assert.Zero(t, testing.AllocsPerRun(100, func() { scope.appendTags(in) }))
	})
}

func TestHasBarePeerHostname(t *testing.T) {
	assert.True(t, hasBarePeerHostname([]string{"db.system:redis", "peer.hostname:orders"}))
	assert.False(t, hasBarePeerHostname([]string{"peer.hostname:orders.shop"}))
	assert.False(t, hasBarePeerHostname([]string{"peer.hostname:"}))
	assert.False(t, hasBarePeerHostname([]string{"out.host:orders"}))
	assert.False(t, hasBarePeerHostname(nil))
}

func TestKubeNamespaceFromTags(t *testing.T) {
	assert.Equal(t, "shop", KubeNamespaceFromTags([]string{"image_tag:1", "kube_namespace:shop", "pod_name:orders-0"}))
	assert.Equal(t, "", KubeNamespaceFromTags([]string{"image_tag:1", "kube_namespace_label:x"}))
	assert.Equal(t, "", KubeNamespaceFromTags(nil))
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
