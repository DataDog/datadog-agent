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

	t.Run("peer.aws.arn contains only callee ARNs", func(t *testing.T) {
		var names []string
		for _, info := range r.GetAttributePrecedence(semantics.ConceptPeerAWSARN) {
			names = append(names, info.Name)
		}
		assert.Equal(t, []string{
			"aws.sns.topic_arn",
			"aws.topic.arn",
			"aws.stream.arn",
			"aws.state_machine.arn",
			"aws.function.arn",
			"aws.table.arn",
			"aws.sns.topic.arn",
			"aws.step_functions.state_machine.arn",
		}, names)
		// cloud.resource_id is an OTel resource attribute naming the caller's own resource,
		// and OTLP spans carry resource attributes in Meta.
		assert.NotContains(t, basePeerTags(r), "cloud.resource_id")
		// Lambda functions run by Step Functions carry statemachinearn for their own state machine.
		assert.NotContains(t, basePeerTags(r), "statemachinearn")
	})

	t.Run("peer.aws.sqs.queue.url contains queue URLs", func(t *testing.T) {
		var names []string
		for _, info := range r.GetAttributePrecedence(semantics.ConceptPeerAWSSQSQueueURL) {
			names = append(names, info.Name)
		}
		assert.Equal(t, []string{"aws.queue.url", "aws.sqs.queue.url"}, names)
	})

	t.Run("scope concepts contain only callee values", func(t *testing.T) {
		names := func(concept semantics.Concept) []string {
			var out []string
			for _, info := range r.GetAttributePrecedence(concept) {
				out = append(out, info.Name)
			}
			return out
		}
		assert.Equal(t, []string{"aws.region"}, names(semantics.ConceptPeerAWSRegion))
		assert.Equal(t, []string{"gcloud.project_id"}, names(semantics.ConceptPeerGCPProjectID))
		assert.Equal(t, []string{"messaging.kafka.cluster_id", "kafka.cluster_id", "messaging.kafka.cluster.id"}, names(semantics.ConceptPeerKafkaClusterID))
		// The Lambda extension sets region and aws_account from the function's own ARN,
		// so on Lambda spans they describe the caller, not the peer.
		assert.NotContains(t, basePeerTags(r), "region")
		assert.NotContains(t, basePeerTags(r), "aws_account")
	})

	t.Run("Pub/Sub identities come only from their dedicated full-path attributes", func(t *testing.T) {
		names := func(concept semantics.Concept) []string {
			var out []string
			for _, info := range r.GetAttributePrecedence(concept) {
				out = append(out, info.Name)
			}
			return out
		}
		assert.Equal(t, []string{"gcp.pubsub.topic.name"}, names(semantics.ConceptPeerGCPPubSubTopicName))
		assert.Equal(t, []string{"gcp.pubsub.subscription.name"}, names(semantics.ConceptPeerGCPPubSubSubscriptionName))
		// Consumers may put the subscription in the generic messaging destination fields,
		// and pubsub.topic can be a short name or a placeholder.
		for _, concept := range []semantics.Concept{semantics.ConceptPeerGCPPubSubTopicName, semantics.ConceptPeerGCPPubSubSubscriptionName} {
			for _, notSource := range []string{"messaging.destination", "messaging.destination.name", "pubsub.topic", "pubsub.subscription"} {
				assert.NotContains(t, names(concept), notSource, concept)
			}
		}
		assert.NotContains(t, basePeerTags(r), "pubsub.topic")
		assert.NotContains(t, basePeerTags(r), "pubsub.subscription")
		assert.Contains(t, basePeerTags(r), "gcp.pubsub.topic.name")
		assert.Contains(t, basePeerTags(r), "gcp.pubsub.subscription.name")
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
