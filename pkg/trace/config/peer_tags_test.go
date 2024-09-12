// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
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
			input:  append([]string{"zz_tag"}, basePeerTags...),
			output: append(basePeerTags, "zz_tag"),
		},
	} {
		sort.Strings(tc.output)
		assert.Equal(t, tc.output, preparePeerTags(tc.input))
	}
}

func TestDefaultPeerTags(t *testing.T) {
	expectedListOfPeerTags := []string{
		"_dd.base_service",
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
		"grpc.host",
		"hostname",
		"http.host",
		"http.server_name",
		"messaging.destination",
		"messaging.destination.name",
		"messaging.kafka.bootstrap.servers",
		"messaging.rabbitmq.exchange",
		"messaging.system",
		"mongodb.db",
		"msmq.queue.path",
		"net.peer.name",
		"network.destination.name",
		"peer.hostname",
		"peer.service",
		"queuename",
		"rpc.service",
		"rpc.system",
		"server.address",
		"streamname",
		"tablename",
		"topicname",
		"dns.hostname",
		"out.host",
	}
	actualListOfPeerTags := basePeerTags

	// Sort both arrays for comparison
	sort.Strings(actualListOfPeerTags)
	sort.Strings(expectedListOfPeerTags)

	assert.Empty(t, cmp.Diff(toSet(expectedListOfPeerTags), toSet(actualListOfPeerTags)))
}

func toSet(list []string) map[string]bool {
	result := make(map[string]bool)
	for _, l := range list {
		result[l] = true
	}
	return result
}
