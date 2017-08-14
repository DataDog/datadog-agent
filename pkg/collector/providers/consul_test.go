// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

import (
	"errors"
	"testing"

	consul "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
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

func (m *consulKVMock) Txn(txn consul.KVTxnOps, q *consul.QueryOptions) (bool, *consul.KVTxnResponse, *consul.QueryMeta, error) {
	// tweak this.
	args := m.Called(txn, q)
	if v, ok := args.Get(0).(bool); ok {
		return v, nil, nil, args.Error(3)
	}
	return true, nil, nil, args.Error(3)
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

	res, idx := consulCli.getTemplates("nginx")
	require.Len(t, res, 2)
	assert.NotNil(t, idx)

	assert.Len(t, res[0].ADIdentifiers, 1)
	assert.Equal(t, "nginx", res[0].ADIdentifiers[0])
	assert.Equal(t, "nginx", res[0].Name)
	assert.Equal(t, "{}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.Equal(t, "{\"host\":\"localhost\",\"port\":21}", string(res[0].Instances[0]))

	assert.Len(t, res[1].ADIdentifiers, 1)
	assert.Equal(t, "nginx", res[1].ADIdentifiers[0])
	assert.Equal(t, "haproxy", res[1].Name)
	assert.Equal(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.Equal(t, "{\"pool\":{\"server\":\"foo\"},\"port\":\"21\"}", string(res[1].Instances[0]))

	kv.On("Get", "/datadog/tpl/nginx_aux/check_names", (*consul.QueryOptions)(nil)).Return(kvNginxNames, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/nginx_aux/init_configs", (*consul.QueryOptions)(nil)).Return(kvNginxInit, nil, nil).Times(1)
	kv.On("Get", "/datadog/tpl/nginx_aux/instances", (*consul.QueryOptions)(nil)).Return(nil, nil, errors.New("unavailable")).Times(1)

	res, idx = consulCli.getTemplates("nginx_aux")
	require.Len(t, res, 0)
	assert.Nil(t, idx)

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

	txConsul := consul.KVTxnOps{
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   "/datadog/tpl/consul/check_names",
			Index: 0,
		},
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   "/datadog/tpl/consul/init_configs",
			Index: 0,
		},
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   "/datadog/tpl/consul/instances",
			Index: 0,
		},
	}
	txNginx := consul.KVTxnOps{
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   "/datadog/tpl/nginx/check_names",
			Index: 0,
		},
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   "/datadog/tpl/nginx/init_configs",
			Index: 0,
		},
		&consul.KVTxnOp{
			Verb:  consul.KVCheckIndex,
			Key:   "/datadog/tpl/nginx/instances",
			Index: 0,
		},
	}
	kv.On("Txn", txConsul, (*consul.QueryOptions)(nil)).Return(false, nil, nil, nil).Times(1)
	kv.On("Txn", txNginx, (*consul.QueryOptions)(nil)).Return(false, nil, nil, nil).Times(1)
	// kv.On("Txn", , (*consul.QueryOptions)(nil)).Return(kvConsulInit, nil, nil).Times(1)
	// kv.On("Txn", , (*consul.QueryOptions)(nil)).Return(kvConsulInstances, nil, nil).Times(1)

	consulCli := ConsulConfigProvider{
		Client:      provider,
		TemplateDir: "/datadog/tpl",
		Cache:       make(map[string][]check.Config),
		cacheIdx:    make(map[string]ADEntryIndex),
	}

	res, err := consulCli.Collect()
	assert.Nil(t, err)
	assert.Len(t, res, 3)

	assert.Len(t, res[0].ADIdentifiers, 1)
	assert.Equal(t, "consul", res[0].ADIdentifiers[0])
	assert.Equal(t, "consul", res[0].Name)
	assert.Equal(t, "{}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.Equal(t, "{\"host\":\"localhost\",\"port\":4500}", string(res[0].Instances[0]))

	assert.Len(t, res[1].ADIdentifiers, 1)
	assert.Equal(t, "nginx", res[1].ADIdentifiers[0])
	assert.Equal(t, "nginx", res[1].Name)
	assert.Equal(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.Equal(t, "{\"host\":\"localhost\",\"port\":21}", string(res[1].Instances[0]))

	assert.Len(t, res[2].ADIdentifiers, 1)
	assert.Equal(t, "nginx", res[2].ADIdentifiers[0])
	assert.Equal(t, "haproxy", res[2].Name)
	assert.Equal(t, "{}", string(res[2].InitConfig))
	require.Len(t, res[2].Instances, 1)
	assert.Equal(t, "{\"pool\":{\"server\":\"foo\"},\"port\":\"21\"}", string(res[2].Instances[0]))

	provider.AssertExpectations(t)
	kv.AssertExpectations(t)
}
