// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build consul

package providers

import (
	"errors"
	"testing"

	consul "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

//
// Mock
//

type consulKVMock struct {
	mock.Mock
}

type consulMock struct {
	mock.Mock
	kv consulKVBackend
}

func (m *consulMock) KV() consulKVBackend {
	return m.kv
}

func (m *consulKVMock) Get(key string, q *consul.QueryOptions) (*consul.KVPair, *consul.QueryMeta, error) {
	args := m.Called(key, q)
	if v, ok := args.Get(0).(*consul.KVPair); ok {
		return v, nil, args.Error(2)
	}
	return nil, nil, args.Error(2)
}

func (m *consulKVMock) Keys(prefix, separator string, q *consul.QueryOptions) ([]string, *consul.QueryMeta, error) {
	args := m.Called(prefix, separator, q)
	if v, ok := args.Get(0).([]string); ok {
		return v, nil, args.Error(2)
	}
	return nil, nil, args.Error(2)
}

func (m *consulKVMock) List(prefix string, q *consul.QueryOptions) (consul.KVPairs, *consul.QueryMeta, error) {
	args := m.Called(prefix, q)
	kvpairs, kvpairs_ok := args.Get(0).(consul.KVPairs)
	if kvpairs_ok {
		return kvpairs, nil, nil
	}
	return nil, nil, args.Error(2)
}

//
// Tests
//

func TestConsulGetIdentifiers(t *testing.T) {
	kv := &consulKVMock{}
	provider := &consulMock{kv: kv}

	kv.On("Keys", "/datadog/tpl", "", (*consul.QueryOptions)(nil)).Return([]string{
		"/datadog/tpl/nginx/check_names",
		"/datadog/tpl/nginx/init_configs",
		"/datadog/tpl/nginx/instances",
		"/datadog/tpl/consul/check_names",
		"/datadog/tpl/consul/init_configs",
		"/datadog/tpl/consul/instances",
	}, nil, nil).Times(1)
	kv.On("Keys", "/datadog/tpl/", "", (*consul.QueryOptions)(nil)).Return([]string{
		"/datadog/tpl/nginx/check_names",
		"/datadog/tpl/nginx/init_configs",
		"/datadog/tpl/nginx/instances",
		"/datadog/tpl/consul/check_names",
		"/datadog/tpl/consul/init_configs",
		"/datadog/tpl/consul/instances",
	}, nil, nil).Times(1)
	kv.On("Keys", "/datadog/foo", "", (*consul.QueryOptions)(nil)).Return([]string{
		"/datadog/tpl/foo/check_names",
		"/datadog/tpl/foo/init_config",
	}, nil, nil).Times(1)

	consulCli := ConsulConfigProvider{
		Client:      provider,
		TemplateDir: "/datadog/tpl",
	}

	ids := consulCli.getIdentifiers("/datadog/foo")
	assert.NotNil(t, ids)
	assert.Len(t, ids, 0)

	ids = consulCli.getIdentifiers("/datadog/tpl")
	assert.NotNil(t, ids)
	assert.Len(t, ids, 2)

	assert.Equal(t, ids[0], "consul")
	assert.Equal(t, ids[1], "nginx")

	assert.Len(t, ids, 2)

	ids = consulCli.getIdentifiers("/datadog/tpl/")
	assert.NotNil(t, ids)
	assert.Len(t, ids, 2)

	assert.Equal(t, ids[0], "consul")
	assert.Equal(t, ids[1], "nginx")

	assert.Len(t, ids, 2)
	provider.AssertExpectations(t)
	kv.AssertExpectations(t)
}

func TestConsulGetTemplates(t *testing.T) {
	kv := &consulKVMock{}
	provider := &consulMock{kv: kv}

	config.Datadog.Set("autoconf_template_dir", "/datadog/tpl")

	//Restore default
	defer config.Datadog.Set("autoconf_template_dir", "/datadog/check_configs")

	kvNginxNames := &consul.KVPair{
		Key:         "/datadog/tpl/nginx/check_names",
		CreateIndex: 0,
		ModifyIndex: 10,
		Value:       []byte("[\"nginx\", \"haproxy\"]"),
	}
	kvNginxInit := &consul.KVPair{
		Key:         "/datadog/tpl/nginx/init_configs",
		CreateIndex: 1,
		ModifyIndex: 11,
		Value:       []byte("[{}, {}]"),
	}
	kvNginxInstances := &consul.KVPair{
		Key:         "/datadog/tpl/nginx/instances",
		CreateIndex: 2,
		ModifyIndex: 12,
		Value:       []byte("[{\"port\": 21, \"host\": \"localhost\"}, {\"port\": \"21\", \"pool\": {\"server\": \"foo\"}}]"),
	}
	kv.On("Get", "/datadog/tpl/nginx/check_names", (*consul.QueryOptions)(nil)).Return(kvNginxNames, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/nginx/init_configs", (*consul.QueryOptions)(nil)).Return(kvNginxInit, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/nginx/instances", (*consul.QueryOptions)(nil)).Return(kvNginxInstances, nil, nil).Times(1)

	consulCli := ConsulConfigProvider{
		Client:      provider,
		TemplateDir: "/datadog/tpl",
	}

	res := consulCli.getTemplates("nginx")
	require.Len(t, res, 2)

	assert.Len(t, res[0].ADIdentifiers, 1)
	assert.Equal(t, "nginx", res[0].ADIdentifiers[0])
	assert.Equal(t, "nginx", res[0].Name)
	assert.JSONEq(t, "{}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.JSONEq(t, "{\"host\":\"localhost\",\"port\":21}", string(res[0].Instances[0]))

	assert.Len(t, res[1].ADIdentifiers, 1)
	assert.Equal(t, "nginx", res[1].ADIdentifiers[0])
	assert.Equal(t, "haproxy", res[1].Name)
	assert.JSONEq(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.JSONEq(t, "{\"pool\":{\"server\":\"foo\"},\"port\":\"21\"}", string(res[1].Instances[0]))

	kv.On("Get", "/datadog/tpl/nginx_aux/check_names", (*consul.QueryOptions)(nil)).Return(kvNginxNames, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/nginx_aux/init_configs", (*consul.QueryOptions)(nil)).Return(kvNginxInit, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/nginx_aux/instances", (*consul.QueryOptions)(nil)).Return(nil, nil, errors.New("unavailable")).Times(1)

	res = consulCli.getTemplates("nginx_aux")
	require.Len(t, res, 0)

	provider.AssertExpectations(t)
	kv.AssertExpectations(t)
}

func TestConsulCollect(t *testing.T) {
	kv := &consulKVMock{}
	provider := &consulMock{kv: kv}

	config.Datadog.Set("autoconf_template_dir", "/datadog/tpl")

	//Restore default
	defer config.Datadog.Set("autoconf_template_dir", "/datadog/check_configs")

	kv.On("Keys", "/datadog/tpl", "", (*consul.QueryOptions)(nil)).Return([]string{
		"/datadog/tpl/nginx/check_names",
		"/datadog/tpl/nginx/init_configs",
		"/datadog/tpl/nginx/instances",
		"/datadog/tpl/consul/check_names",
		"/datadog/tpl/consul/init_configs",
		"/datadog/tpl/consul/instances",
	}, nil, nil).Times(1)

	kvNginxNames := &consul.KVPair{
		Key:         "/datadog/tpl/nginx/check_names",
		CreateIndex: 0,
		ModifyIndex: 10,
		Value:       []byte("[\"nginx\", \"haproxy\"]"),
	}
	kvNginxInit := &consul.KVPair{
		Key:         "/datadog/tpl/nginx/init_configs",
		CreateIndex: 1,
		ModifyIndex: 11,
		Value:       []byte("[{}, {}]"),
	}
	kvNginxInstances := &consul.KVPair{
		Key:         "/datadog/tpl/nginx/instances",
		CreateIndex: 2,
		ModifyIndex: 12,
		Value:       []byte("[{\"port\": 21, \"host\": \"localhost\"}, {\"port\": \"21\", \"pool\": {\"server\": \"foo\"}}]"),
	}
	kvConsulNames := &consul.KVPair{
		Key:         "/datadog/tpl/consul/check_names",
		CreateIndex: 0,
		ModifyIndex: 10,
		Value:       []byte("[\"consul\"]"),
	}
	kvConsulInit := &consul.KVPair{
		Key:         "/datadog/tpl/consul/init_configs",
		CreateIndex: 1,
		ModifyIndex: 11,
		Value:       []byte("[{}]"),
	}
	kvConsulInstances := &consul.KVPair{
		Key:         "/datadog/tpl/consul/instances",
		CreateIndex: 2,
		ModifyIndex: 12,
		Value:       []byte("[{\"port\": 4500, \"host\": \"localhost\"}]"),
	}
	kv.On("Get", "/datadog/tpl/nginx/check_names", (*consul.QueryOptions)(nil)).Return(kvNginxNames, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/nginx/init_configs", (*consul.QueryOptions)(nil)).Return(kvNginxInit, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/nginx/instances", (*consul.QueryOptions)(nil)).Return(kvNginxInstances, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/consul/check_names", (*consul.QueryOptions)(nil)).Return(kvConsulNames, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/consul/init_configs", (*consul.QueryOptions)(nil)).Return(kvConsulInit, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/consul/instances", (*consul.QueryOptions)(nil)).Return(kvConsulInstances, nil, nil).Times(1)

	consulCli := ConsulConfigProvider{
		Client:      provider,
		TemplateDir: "/datadog/tpl",
	}

	res, err := consulCli.Collect()
	assert.Nil(t, err)
	assert.Len(t, res, 3)

	assert.Len(t, res[0].ADIdentifiers, 1)
	assert.Equal(t, "consul", res[0].ADIdentifiers[0])
	assert.Equal(t, "consul", res[0].Name)
	assert.JSONEq(t, "{}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.JSONEq(t, "{\"host\":\"localhost\",\"port\":4500}", string(res[0].Instances[0]))

	assert.Len(t, res[1].ADIdentifiers, 1)
	assert.Equal(t, "nginx", res[1].ADIdentifiers[0])
	assert.Equal(t, "nginx", res[1].Name)
	assert.JSONEq(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.JSONEq(t, "{\"host\":\"localhost\",\"port\":21}", string(res[1].Instances[0]))

	assert.Len(t, res[2].ADIdentifiers, 1)
	assert.Equal(t, "nginx", res[2].ADIdentifiers[0])
	assert.Equal(t, "haproxy", res[2].Name)
	assert.JSONEq(t, "{}", string(res[2].InitConfig))
	require.Len(t, res[2].Instances, 1)
	assert.JSONEq(t, "{\"pool\":{\"server\":\"foo\"},\"port\":\"21\"}", string(res[2].Instances[0]))

	provider.AssertExpectations(t)
	kv.AssertExpectations(t)
}

func TestIsUpToDate(t *testing.T) {
	// We want to check:
	// The cache is properly initialized
	// LatestTemplateIdx and NumAdTemplates are properly set
	// If the number of ADTemplate is modified we update
	// If nothing changed we don't update

	kv := &consulKVMock{}
	provider := &consulMock{kv: kv}

	kvNginx := &consul.KVPair{
		Key:         "/datadog/check_configs/nginx",
		CreateIndex: 0,
		ModifyIndex: 9,
		Value:       []byte(""),
	}
	kvNginxNames := &consul.KVPair{
		Key:         "/datadog/check_configs/nginx/check_names",
		CreateIndex: 1,
		ModifyIndex: 10,
		Value:       []byte("[\"nginx\", \"haproxy\"]"),
	}
	kvNginxInit := &consul.KVPair{
		Key:         "/datadog/check_configs/nginx/init_configs",
		CreateIndex: 2,
		ModifyIndex: 11,
		Value:       []byte("[{}, {}]"),
	}
	kvNginxInstances := &consul.KVPair{
		Key:         "/datadog/check_configs/nginx/instances",
		CreateIndex: 3,
		ModifyIndex: 12,
		Value:       []byte("[{\"port\": 21, \"host\": \"localhost\"}, {\"port\": \"21\", \"pool\": {\"server\": \"foo\"}}]"),
	}
	kv.On("List", "/datadog/check_configs", (*consul.QueryOptions)(nil)).Return(consul.KVPairs{kvNginx, kvNginxNames, kvNginxInit, kvNginxInstances}, nil, nil).Times(1)

	cache := NewCPCache()
	consulCli := ConsulConfigProvider{
		Client:      provider,
		TemplateDir: "/datadog/check_configs",
		cache:       cache,
	}
	assert.Equal(t, int(0), consulCli.cache.NumAdTemplates)
	assert.Equal(t, float64(0), consulCli.cache.LatestTemplateIdx)

	update, _ := consulCli.IsUpToDate()
	assert.False(t, update)
	assert.Equal(t, int(4), consulCli.cache.NumAdTemplates)
	assert.Equal(t, float64(12), consulCli.cache.LatestTemplateIdx)

	kvNewTemplate := &consul.KVPair{
		Key:         "/datadog/check_configs/new",
		CreateIndex: 15,
		ModifyIndex: 15,
		Value:       []byte(""),
	}
	kv.On("List", "/datadog/check_configs", (*consul.QueryOptions)(nil)).Return(consul.KVPairs{kvNewTemplate, kvNginx, kvNginxNames, kvNginxInit, kvNginxInstances}, nil, nil)
	update, _ = consulCli.IsUpToDate()
	assert.False(t, update)
	assert.Equal(t, int(5), consulCli.cache.NumAdTemplates)
	assert.Equal(t, float64(15), consulCli.cache.LatestTemplateIdx)

	update, _ = consulCli.IsUpToDate()
	assert.True(t, update)
	provider.AssertExpectations(t)
	kv.AssertExpectations(t)
}
