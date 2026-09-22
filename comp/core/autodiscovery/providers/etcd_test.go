// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build etcd

package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/client/v3"
)

type etcdTest struct {
	mock.Mock
}

func (m *etcdTest) Get(ctx context.Context, key string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	args := m.Called(ctx, key)
	resp, respOK := args.Get(0).(*clientv3.GetResponse)
	if respOK {
		return resp, nil
	}
	return nil, args.Error(1)
}

func createTestNode(prefix, suffix string) *mvccpb.KeyValue {
	return &mvccpb.KeyValue{
		Key:         []byte(prefix + "/" + suffix),
		Value:       []byte("test"),
		ModRevision: 123456,
	}
}

func TestHasTemplateFields(t *testing.T) {
	res := hasTemplateFields([]string{})
	assert.False(t, res)

	tooFewNodes := []string{"foo", "check_names"}
	res = hasTemplateFields(tooFewNodes)
	assert.False(t, res)

	invalidNodes := []string{"foo", "check_names", "init_configs"}
	res = hasTemplateFields(invalidNodes)
	assert.False(t, res)

	validNodes := []string{"check_names", "init_configs", "instances"}
	res = hasTemplateFields(validNodes)
	assert.True(t, res)
}

func TestGetIdentifiers(t *testing.T) {
	ctx := t.Context()
	backend := &etcdTest{}
	nginx := "/datadog/check_configs/nginx"
	node1 := createTestNode(nginx, "check_names")
	node2 := createTestNode(nginx, "init_configs")
	node3 := createTestNode(nginx, "instances")
	nodes := []*mvccpb.KeyValue{node1, node2, node3}

	backend.On("Get", ctx, "/datadog/check_configs/").Return(&clientv3.GetResponse{Kvs: nodes}, nil).Times(1)
	etcd := EtcdConfigProvider{Client: backend, templateDir: "/datadog/check_configs"}
	array := etcd.getIdentifiers(ctx, "/datadog/check_configs")

	assert.Len(t, array, 1)
	assert.Equal(t, array, []string{"nginx"})

	haproxy := "/datadog/check_configs/haproxy"
	node1 = createTestNode(haproxy, "check_names")
	node2 = createTestNode(haproxy, "init_configs")
	toofew := []*mvccpb.KeyValue{node1, node2}
	backend.On("Get", ctx, "/datadog/check_configs/").Return(&clientv3.GetResponse{Kvs: toofew}, nil)

	errArray := etcd.getIdentifiers(ctx, "/datadog/check_configs")

	assert.Len(t, errArray, 0)
	assert.Equal(t, errArray, []string{})

	backend.AssertExpectations(t)
}

func TestETCDIsUpToDate(t *testing.T) {
	// We want to check:
	// The cache is properly initialized
	// mostRecentMod and count are properly set
	// If the number of ADTemplate is modified we update
	// If nothing changed we don't update

	ctx := t.Context()
	backend := &etcdTest{}

	nginx := "/datadog/check_configs/nginx"
	node1 := createTestNode(nginx, "check_names")
	node2 := createTestNode(nginx, "init_configs")
	node3 := createTestNode(nginx, "instances")
	nodes := []*mvccpb.KeyValue{node1, node2, node3}

	backend.On("Get", ctx, "/datadog/check_configs/").Return(&clientv3.GetResponse{Kvs: nodes}, nil).Times(1)
	cache := newProviderCache()
	etcd := EtcdConfigProvider{Client: backend, templateDir: "/datadog/check_configs", cache: cache}
	update, _ := etcd.IsUpToDate(ctx)

	assert.False(t, update)
	assert.Equal(t, float64(123456), etcd.cache.mostRecentMod)
	assert.Equal(t, 1, etcd.cache.count)

	apache := "/datadog/check_configs/apache"
	node4 := createTestNode(apache, "check_names")
	node5 := createTestNode(apache, "init_configs")
	node6 := &mvccpb.KeyValue{
		Key:         []byte(apache + "/instances"),
		Value:       []byte("val"),
		ModRevision: 9000000,
	}
	nodes = append(nodes, node4, node5, node6)
	backend.On("Get", ctx, "/datadog/check_configs/").Return(&clientv3.GetResponse{Kvs: nodes}, nil).Times(1)
	update, _ = etcd.IsUpToDate(ctx)

	assert.False(t, update)
	assert.Equal(t, float64(9000000), etcd.cache.mostRecentMod)
	assert.Equal(t, 2, etcd.cache.count)

	backend.On("Get", ctx, "/datadog/check_configs/").Return(&clientv3.GetResponse{Kvs: nodes}, nil).Times(1)
	update, _ = etcd.IsUpToDate(ctx)

	assert.True(t, update)
	assert.Equal(t, float64(9000000), etcd.cache.mostRecentMod)
	assert.Equal(t, 2, etcd.cache.count)
	backend.AssertExpectations(t)
}
