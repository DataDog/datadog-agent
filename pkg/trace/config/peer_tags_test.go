// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	"github.com/stretchr/testify/assert"
)

func TestPreparePeerTags(t *testing.T) {
	type testCase struct {
		input  []string
		output []string
	}

	for _, tc := range []testCase{
		{
			input:  nil,
			output: nil,
		},
		{
			input:  []string{},
			output: nil,
		},
		{
			input:  []string{"zz_tag", "peer.service", "some.other.tag", "db.name", "db.instance"},
			output: []string{"db.name", "db.instance", "peer.service", "some.other.tag", "zz_tag"},
		},
		{
			input:  append([]string{"zz_tag"}, basePeerTags(semantics.DefaultRegistry())...),
			output: append(basePeerTags(semantics.DefaultRegistry()), "zz_tag"),
		},
	} {
		sort.Strings(tc.output)
		assert.Equal(t, tc.output, preparePeerTags(tc.input))
	}
}

func TestDefaultPeerTags(t *testing.T) {
	assert.Contains(t, basePeerTags(semantics.DefaultRegistry()), "db.name")
	assert.Contains(t, basePeerTags(semantics.DefaultRegistry()), "_dd.base_service")
}

func TestPeerTagConceptsHaveMappings(t *testing.T) {
	r := semantics.DefaultRegistry()

	for _, concept := range peerTagConcepts {
		keys := r.GetAttributePrecedence(concept)
		assert.NotEmpty(t, keys, "concept %q has no fallbacks in mappings.json", concept)
	}
}

func TestPeerTagConceptKeysInOrder(t *testing.T) {
	r := semantics.DefaultRegistry()

	t.Run("peer.hostname first key is peer.hostname", func(t *testing.T) {
		infos := r.GetAttributePrecedence(semantics.ConceptPeerHostname)
		assert.NotEmpty(t, infos)
		if len(infos) > 0 {
			assert.Equal(t, "peer.hostname", infos[0].Name)
		}
		var names []string
		for _, info := range infos {
			names = append(names, info.Name)
		}
		assert.Contains(t, names, "hostname")
		assert.Contains(t, names, "net.peer.name")
		assert.Contains(t, names, "db.hostname")
	})

	t.Run("peer.db.name contains expected keys", func(t *testing.T) {
		infos := r.GetAttributePrecedence(semantics.ConceptPeerDBName)
		assert.NotEmpty(t, infos)
		var names []string
		for _, info := range infos {
			names = append(names, info.Name)
		}
		assert.Contains(t, names, "db.name")
		assert.Contains(t, names, "mongodb.db")
		assert.Contains(t, names, "db.instance")
		assert.Contains(t, names, "cassandra.keyspace")
	})
}

func TestPeerTagsAggregation(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := New()
		cfg.PeerTagsAggregation = false
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Empty(t, cfg.PeerTags)
		assert.Empty(t, cfg.ConfiguredPeerTags())
	})

	t.Run("default-enabled", func(t *testing.T) {
		cfg := New()
		assert.Empty(t, cfg.PeerTags)
		assert.Equal(t, basePeerTags(semantics.DefaultRegistry()), cfg.ConfiguredPeerTags())
	})
	t.Run("disabled-user-tags", func(t *testing.T) {
		cfg := New()
		cfg.PeerTagsAggregation = false
		cfg.PeerTags = []string{"user_peer_tag"}
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Empty(t, cfg.ConfiguredPeerTags())
	})
	t.Run("enabled-user-tags", func(t *testing.T) {
		cfg := New()
		cfg.PeerTags = []string{"user_peer_tag"}
		assert.Equal(t, append(basePeerTags(semantics.DefaultRegistry()), "user_peer_tag"), cfg.ConfiguredPeerTags())
	})
	t.Run("dedup", func(t *testing.T) {
		cfg := New()
		cfg.PeerTags = basePeerTags(semantics.DefaultRegistry())[:2]
		assert.Equal(t, basePeerTags(semantics.DefaultRegistry()), cfg.ConfiguredPeerTags())
	})
}

// TestBasePeerTagsMatchINISource verifies that basePeerTags contains all the keys
// that were previously sourced from peer_tags.ini, ensuring behavioral equivalence
// between the old INI-based and new semantics-based paths.
func TestBasePeerTagsMatchINISource(t *testing.T) {
	// Canonical source: https://github.com/DataDog/semantic-core/
	expectedFromINI := []string{
		"_dd.base_service",
		"active_record.db.vendor",
		"amqp.destination",
		"amqp.exchange",
		"amqp.queue",
		"aws.queue.name",
		"aws.s3.bucket",
		"bucketname",
		"cassandra.keyspace",
		"db.cassandra.contact.points",
		"db.couchbase.seed.nodes",
		"db.hostname",
		"db.instance",
		"db.name",
		"db.namespace",
		"db.system",
		"db.type",
		"dns.hostname",
		"grpc.host",
		"http.host",
		"http.server_name",
		"messaging.destination",
		"messaging.destination.name",
		"messaging.kafka.bootstrap.servers",
		"messaging.rabbitmq.exchange",
		"messaging.system",
		"msmq.queue.path",
		"mongodb.db",
		"net.peer.name",
		"network.destination.ip",
		"network.destination.name",
		"out.host",
		"peer.hostname",
		"peer.service",
		"queuename",
		"rpc.service",
		"rpc.system",
		"sequel.db.vendor",
		"server.address",
		"streamname",
		"tablename",
		"topicname",
	}

	for _, key := range expectedFromINI {
		assert.Contains(t, basePeerTags(semantics.DefaultRegistry()), key, "basePeerTags is missing key %q that was present in peer_tags.ini", key)
	}
}
