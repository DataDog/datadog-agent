// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build etcd

package providers

import (
	"testing"

	"github.com/coreos/etcd/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
)

type etcdTest struct {
	mock.Mock
}

func (m *etcdTest) Get(ctx context.Context, key string, opts *client.GetOptions) (*client.Response, error) {
	args := m.Called(ctx, key, opts)
	resp, respOK := args.Get(0).(*client.Response)
	if respOK {
		return resp, nil
	}
	return nil, args.Error(1)
}

func createTestNode(key string) *client.Node {
	return &client.Node{
		Key:           key,
		Value:         "test",
		CreatedIndex:  123456,
		ModifiedIndex: 123456,
		TTL:           123456789,
	}
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

func TestGetIdentifiers(t *testing.T) {
	backend := &etcdTest{}
	resp := new(client.Response)
	configPath := new(client.Node)
	node1 := createTestNode("check_names")
	node2 := createTestNode("init_configs")
	node3 := createTestNode("instances")
	nodes := []*client.Node{node1, node2, node3}
	configPath.Key = "/datadog/check_configs/"
	nginx := &client.Node{
		Key:   "/datadog/check_configs/nginx",
		Dir:   true,
		Nodes: nodes,
	}
	adTemplate := []*client.Node{nginx}
	configPath.Nodes = adTemplate
	resp.Node = configPath

	backend.On("Get", context.Background(), "/datadog/check_configs", &client.GetOptions{Recursive: true}).Return(resp, nil).Times(1)
	etcd := EtcdConfigProvider{Client: backend, templateDir: "/datadog/check_configs"}
	array := etcd.getIdentifiers("/datadog/check_configs")

	assert.Len(t, array, 1)
	assert.Equal(t, array, []string{"nginx"})

	badConf := new(client.Node)
	toofew := []*client.Node{node1, node2}
	badConf.Key = "/datadog/check_configs/"
	haproxy := &client.Node{
		Key:   "/datadog/check_configs/haproxy",
		Dir:   true,
		Nodes: toofew,
	}
	adTemplate = []*client.Node{haproxy}
	badConf.Nodes = adTemplate
	resp.Node = badConf
	backend.On("Get", context.Background(), "/datadog/check_configs", &client.GetOptions{Recursive: true}).Return(resp, nil)

	errArray := etcd.getIdentifiers("/datadog/check_configs")

	assert.Len(t, errArray, 0)
	assert.Equal(t, errArray, []string{})

	backend.AssertExpectations(t)
}

func TestETCDIsUpToDate(t *testing.T) {
	// We want to check:
	// The cache is properly initialized
	// LatestTemplateIdx and NumAdTemplates are properly set
	// If the number of ADTemplate is modified we update
	// If nothing changed we don't update

	backend := &etcdTest{}
	resp := new(client.Response)

	configPath := new(client.Node)
	node1 := createTestNode("check_names")
	node2 := createTestNode("init_configs")
	node3 := createTestNode("instances")
	nodes := []*client.Node{node1, node2, node3}
	configPath.Key = "/datadog/check_configs/"
	nginx := &client.Node{
		Key:   "/datadog/check_configs/nginx",
		Dir:   true,
		Nodes: nodes,
	}
	adTemplate := []*client.Node{nginx}
	configPath.Nodes = adTemplate
	resp.Node = configPath

	backend.On("Get", context.Background(), "/datadog/check_configs", &client.GetOptions{Recursive: true}).Return(resp, nil).Times(1)
	cache := NewCPCache()
	etcd := EtcdConfigProvider{Client: backend, templateDir: "/datadog/check_configs", cache: cache}
	update, _ := etcd.IsUpToDate()

	assert.False(t, update)
	assert.Equal(t, float64(123456), etcd.cache.LatestTemplateIdx)
	assert.Equal(t, 1, etcd.cache.NumAdTemplates)

	node4 := &client.Node{
		Key:           "instances",
		Value:         "val",
		CreatedIndex:  123457,
		ModifiedIndex: 9000000,
		TTL:           123456789,
	}
	nodes = []*client.Node{node1, node2, node4}
	apache := &client.Node{
		Key:   "/datadog/check_configs/nginx",
		Dir:   true,
		Nodes: nodes,
	}
	adTemplate = []*client.Node{nginx, apache}
	configPath.Nodes = adTemplate
	resp.Node = configPath
	backend.On("Get", context.Background(), "/datadog/check_configs", &client.GetOptions{Recursive: true}).Return(resp, nil).Times(1)
	update, _ = etcd.IsUpToDate()

	assert.False(t, update)
	assert.Equal(t, float64(9000000), etcd.cache.LatestTemplateIdx)
	assert.Equal(t, 2, etcd.cache.NumAdTemplates)

	backend.On("Get", context.Background(), "/datadog/check_configs", &client.GetOptions{Recursive: true}).Return(resp, nil).Times(1)
	update, _ = etcd.IsUpToDate()

	assert.True(t, update)
	assert.Equal(t, float64(9000000), etcd.cache.LatestTemplateIdx)
	assert.Equal(t, 2, etcd.cache.NumAdTemplates)
	backend.AssertExpectations(t)
}
