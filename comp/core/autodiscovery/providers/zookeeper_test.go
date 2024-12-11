// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zk

package providers

import (
	"context"
	"fmt"
	"testing"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

//
// Mock
//

type zkTest struct {
	mock.Mock
}

func (m *zkTest) Get(key string) ([]byte, *zk.Stat, error) {
	args := m.Called(key)
	array, arrOK := args.Get(0).([]byte)
	stats, statsOK := args.Get(1).(*zk.Stat)
	if arrOK && statsOK {
		return array, stats, args.Error(2)
	}
	if arrOK {
		return array, nil, args.Error(2)
	}
	return nil, nil, args.Error(2)
}

func (m *zkTest) Children(key string) ([]string, *zk.Stat, error) {
	args := m.Called(key)
	array, arrOK := args.Get(0).([]string)
	stats, statsOK := args.Get(1).(*zk.Stat)
	if arrOK && statsOK {
		return array, stats, args.Error(2)
	}
	if arrOK {
		return array, nil, args.Error(2)
	}
	return nil, nil, args.Error(2)
}

//
// Tests
//

func TestZKGetIdentifiers(t *testing.T) {
	backend := &zkTest{}

	backend.On("Children", "/test/").Return(nil, nil, fmt.Errorf("some error")).Times(1)
	backend.On("Children", "/datadog/tpl").Return([]string{"nginx", "redis", "incomplete", "error"}, nil, nil).Times(1)

	expectedKeys := []string{checkNamePath, initConfigPath, instancePath}
	backend.On("Children", "/datadog/tpl/nginx").Return(expectedKeys, nil, nil).Times(1)
	backend.On("Children", "/datadog/tpl/redis").Return(append(expectedKeys, "an extra one"), nil, nil).Times(1)
	backend.On("Children", "/datadog/tpl/incomplete").Return([]string{checkNamePath, "other one"}, nil, nil).Times(1)
	backend.On("Children", "/datadog/tpl/error").Return(nil, nil, fmt.Errorf("some error")).Times(1)

	zk := ZookeeperConfigProvider{client: backend}

	res, err := zk.getIdentifiers("/test/")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	res, err = zk.getIdentifiers("/datadog/tpl")
	require.Nil(t, err)

	assert.Len(t, res, 2)
	assert.Equal(t, []string{"/datadog/tpl/nginx", "/datadog/tpl/redis"}, res)
	backend.AssertExpectations(t)
}

func TestZKGetTemplates(t *testing.T) {
	backend := &zkTest{}

	backend.On("Get", "/error1/check_names").Return(nil, nil, fmt.Errorf("some error")).Times(1)
	zk := ZookeeperConfigProvider{client: backend}
	res := zk.getTemplates("/error1/")
	assert.Nil(t, res)

	backend.On("Get", "/error2/check_names").Return([]byte("[\"first_name\"]"), nil, nil).Times(1)
	backend.On("Get", "/error2/init_configs").Return(nil, nil, fmt.Errorf("some error")).Times(1)
	res = zk.getTemplates("/error2/")
	assert.Nil(t, res)

	backend.On("Get", "/error3/check_names").Return([]byte("[\"first_name\"]"), nil, nil).Times(1)
	backend.On("Get", "/error3/init_configs").Return([]byte("[{}]"), nil, nil).Times(1)
	backend.On("Get", "/error3/instances").Return(nil, nil, fmt.Errorf("some error")).Times(1)
	res = zk.getTemplates("/error3/")
	assert.Nil(t, res)

	backend.On("Get", "/error4/check_names").Return([]byte("[\"first_name\"]"), nil, nil).Times(1)
	backend.On("Get", "/error4/instances").Return([]byte("[{}]"), nil, nil).Times(1)
	backend.On("Get", "/error4/init_configs").Return([]byte("[{}, {}]"), nil, nil).Times(1)
	res = zk.getTemplates("/error4/")
	assert.Len(t, res, 0)

	backend.On("Get", "/error5/check_names").Return([]byte(""), nil, nil).Times(1)
	backend.On("Get", "/error5/instances").Return([]byte("[{}]"), nil, nil).Times(1)
	backend.On("Get", "/error5/init_configs").Return([]byte("[{}]"), nil, nil).Times(1)
	res = zk.getTemplates("/error5/")
	assert.Len(t, res, 0)

	backend.On("Get", "/config/check_names").Return([]byte("[\"first_name\", \"second_name\"]"), nil, nil).Times(1)
	backend.On("Get", "/config/instances").Return([]byte("[{\"test\": 21, \"test2\": \"data\"}, {\"data1\": \"21\", \"data2\": {\"number\": 21}}]"), nil, nil).Times(1)
	backend.On("Get", "/config/init_configs").Return([]byte("[{\"a\": \"b\"}, {}]"), nil, nil).Times(1)
	//zk = ZookeeperConfigProvider{client: backend}
	res = zk.getTemplates("/config/")
	assert.NotNil(t, res)
	assert.Len(t, res, 2)

	assert.Len(t, res[0].ADIdentifiers, 1)
	assert.Equal(t, "/config/", res[0].ADIdentifiers[0])
	assert.Equal(t, "first_name", res[0].Name)
	assert.Equal(t, "{\"a\":\"b\"}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.Equal(t, "{\"test\":21,\"test2\":\"data\"}", string(res[0].Instances[0]))

	assert.Len(t, res[1].ADIdentifiers, 1)
	assert.Equal(t, "/config/", res[1].ADIdentifiers[0])
	assert.Equal(t, "second_name", res[1].Name)
	assert.Equal(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.Equal(t, "{\"data1\":\"21\",\"data2\":{\"number\":21}}", string(res[1].Instances[0]))
}

func TestZKCollect(t *testing.T) {
	ctx := context.Background()
	backend := &zkTest{}

	backend.On("Children", "/datadog/check_configs").Return([]string{"other", "config_folder_1", "config_folder_2"}, nil, nil).Times(1)
	backend.On("Children", "/datadog/check_configs/other").Return([]string{"test", "check_names"}, nil, nil).Times(1)

	backend.On("Children", "/datadog/check_configs/config_folder_1").Return([]string{"check_names", "instances", "init_configs"}, nil, nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_1/check_names").Return([]byte("[\"first_name\", \"second_name\"]"), nil, nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_1/instances").Return([]byte("[{}, {}]"), nil, nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_1/init_configs").Return([]byte("[{}, {}]"), nil, nil).Times(1)

	backend.On("Children", "/datadog/check_configs/config_folder_2").Return([]string{"check_names", "instances", "init_configs", "test"}, nil, nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_2/check_names").Return([]byte("[\"third_name\"]"), nil, nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_2/instances").Return([]byte("[{}]"), nil, nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_2/init_configs").Return([]byte("[{}]"), nil, nil).Times(1)

	zk := ZookeeperConfigProvider{client: backend, templateDir: "/datadog/check_configs"}

	res, err := zk.Collect(ctx)
	assert.Nil(t, err)
	assert.Len(t, res, 3)

	assert.Len(t, res[0].ADIdentifiers, 1)
	assert.Equal(t, "/datadog/check_configs/config_folder_1", res[0].ADIdentifiers[0])
	assert.Equal(t, "first_name", res[0].Name)
	assert.Equal(t, "zookeeper:/datadog/check_configs/config_folder_1", res[0].Source)
	assert.Equal(t, "{}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.Equal(t, "{}", string(res[0].Instances[0]))

	assert.Len(t, res[1].ADIdentifiers, 1)
	assert.Equal(t, "/datadog/check_configs/config_folder_1", res[1].ADIdentifiers[0])
	assert.Equal(t, "second_name", res[1].Name)
	assert.Equal(t, "zookeeper:/datadog/check_configs/config_folder_1", res[1].Source)
	assert.Equal(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.Equal(t, "{}", string(res[1].Instances[0]))

	assert.Len(t, res[2].ADIdentifiers, 1)
	assert.Equal(t, "/datadog/check_configs/config_folder_2", res[2].ADIdentifiers[0])
	assert.Equal(t, "third_name", res[2].Name)
	assert.Equal(t, "zookeeper:/datadog/check_configs/config_folder_2", res[2].Source)
	assert.Equal(t, "{}", string(res[2].InitConfig))
	require.Len(t, res[2].Instances, 1)
	assert.Equal(t, "{}", string(res[2].Instances[0]))
}

func TestZKIsUpToDate(t *testing.T) {
	// We want to check:
	// The cache is properly initialized
	// If one Mtime is newer than what was in cache we update
	// If the number of ADTemplate is modified we update
	// If nothing changed we don't update

	ctx := context.Background()
	backend := &zkTest{}
	z := new(zk.Stat)
	backend.On("Children", "/datadog/check_configs").Return([]string{"config_folder_1"}, nil, nil).Times(1)
	expectedKeys := []string{checkNamePath, initConfigPath, instancePath}
	backend.On("Children", "/datadog/check_configs/config_folder_1").Return(expectedKeys, nil, nil)
	backend.On("Get", "/datadog/check_configs/config_folder_1/check_names").Return([]byte("[\"first_name\", \"second_name\"]"), z, nil)
	backend.On("Get", "/datadog/check_configs/config_folder_1/instances").Return([]byte("[{}, {}]"), z, nil)
	z.Mtime = int64(709662600)
	backend.On("Get", "/datadog/check_configs/config_folder_1/init_configs").Return([]byte("[{}, {}]"), z, nil)

	cache := newProviderCache()

	zkr := ZookeeperConfigProvider{client: backend, templateDir: "/datadog/check_configs", cache: cache}
	assert.Equal(t, float64(0), zkr.cache.mostRecentMod)
	assert.Equal(t, int(0), zkr.cache.count)

	update, _ := zkr.IsUpToDate(ctx)
	assert.False(t, update)
	assert.Equal(t, float64(709662600), zkr.cache.mostRecentMod)

	backend.On("Children", "/datadog/check_configs").Return([]string{"config_folder_1", "config_folder_2"}, nil, nil)
	backend.On("Children", "/datadog/check_configs/config_folder_2").Return([]string{"check_names", "instances", "init_configs"}, z, nil)

	backend.On("Get", "/datadog/check_configs/config_folder_2/check_names").Return([]byte("[\"third_name\"]"), z, nil)
	backend.On("Get", "/datadog/check_configs/config_folder_2/instances").Return([]byte("[{}]"), z, nil)
	backend.On("Get", "/datadog/check_configs/config_folder_2/init_configs").Return([]byte("[{}]"), z, nil)

	update, _ = zkr.IsUpToDate(ctx)
	assert.False(t, update)
	assert.Equal(t, int(2), zkr.cache.count)

	update, _ = zkr.IsUpToDate(ctx)
	assert.True(t, update)
	backend.AssertExpectations(t)
}
