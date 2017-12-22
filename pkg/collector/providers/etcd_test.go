// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build etcd

package providers

import (
	"context"
	"testing"
	"github.com/stretchr/testify/mock"
	"github.com/coreos/etcd/client"
	"github.com/stretchr/testify/assert"
)

type etcdTest struct {
	mock.Mock
}

func (m *etcdTest) Get(ctx context.Context, key string, opts *client.GetOptions) (*client.Response, error){
	args := m.Called(ctx, key, opts)
	resp, resp_ok := args.Get(0).(*client.Response)
	if resp_ok {
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

func TestGetIdentifiers(t *testing.T){
	backend := &etcdTest{}
	resp := new(client.Response)
	//kv.On("Get", context.Background(), "/datadog/tpl/nginx/check_names", &client.GetOptions{Recursive: true}).Return(resp, nil).Times(1)
	etcd := EtcdConfigProvider{Client: backend, templateDir: "/datadog/check_configs" }

}