package providers

import (
	"testing"

	"github.com/coreos/etcd/client"
	"github.com/stretchr/testify/assert"
)

func createTestNode(key string) *client.Node {
	return &client.Node{
		Key:           key,
		Value:         "test",
		CreatedIndex:  123456,
		ModifiedIndex: 123456,
		TTL:           123456789,
	}
}

func TestBuildStoreKey(t *testing.T) {
	res := buildStoreKey()
	assert.Equal(t, res, "/datadog/check_configs")
	res = buildStoreKey("")
	assert.Equal(t, res, "/datadog/check_configs")
	res = buildStoreKey("foo")
	assert.Equal(t, res, "/datadog/check_configs/foo")
	res = buildStoreKey("foo", "bar")
	assert.Equal(t, res, "/datadog/check_configs/foo/bar")
	res = buildStoreKey("foo", "bar", "bazz")
	assert.Equal(t, res, "/datadog/check_configs/foo/bar/bazz")
}

func TestHasTemplateFields(t *testing.T) {
	emptyNodes := []*client.Node{}
	node0 := createTestNode("foo")
	node1 := createTestNode("check_names")
	node2 := createTestNode("init_configs")
	node3 := createTestNode("instances")

	res := hasTemplateFields(emptyNodes)
	assert.False(t, res)

	tooFewNodes := []*client.Node{node0, node1}
	res = hasTemplateFields(tooFewNodes)
	assert.False(t, res)

	invalidNodes := []*client.Node{node0, node1, node2}
	res = hasTemplateFields(invalidNodes)
	assert.False(t, res)

	validNodes := []*client.Node{node1, node2, node3}
	res = hasTemplateFields(validNodes)
	assert.True(t, res)
}
