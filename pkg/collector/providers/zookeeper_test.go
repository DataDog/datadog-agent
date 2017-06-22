// +build zk

package providers

import (
	"fmt"
	"testing"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

//
// Mock
//

type zkTest struct {
	mock.Mock
}

func (m *zkTest) Get(key string) ([]byte, *zk.Stat, error) {
	args := m.Called(key)
	if v, ok := args.Get(0).([]byte); ok {
		return v, nil, args.Error(1)
	}
	return nil, nil, args.Error(1)
}

func (m *zkTest) Children(key string) ([]string, *zk.Stat, error) {
	args := m.Called(key)
	if v, ok := args.Get(0).([]string); ok {
		return v, nil, args.Error(1)
	}
	return nil, nil, args.Error(1)
}

//
// Tests
//

func TestZKGetIdentifiers(t *testing.T) {
	backend := &zkTest{}

	backend.On("Children", "/test/").Return(nil, fmt.Errorf("some error")).Times(1)
	backend.On("Children", "/datadog/tpl").Return([]string{"nginx", "redis", "incomplete", "error"}, nil).Times(1)

	expectedKeys := []string{checkNamePath, initConfigPath, instancePath}
	backend.On("Children", "/datadog/tpl/nginx").Return(expectedKeys, nil).Times(1)
	backend.On("Children", "/datadog/tpl/redis").Return(append(expectedKeys, "an extra one"), nil).Times(1)
	backend.On("Children", "/datadog/tpl/incomplete").Return([]string{checkNamePath, "other one"}, nil).Times(1)
	backend.On("Children", "/datadog/tpl/error").Return(nil, fmt.Errorf("some error")).Times(1)

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

	backend.On("Get", "/error1/check_names").Return(nil, fmt.Errorf("some error")).Times(1)
	zk := ZookeeperConfigProvider{client: backend}
	res := zk.getTemplates("/error1/")
	assert.Nil(t, res)

	backend.On("Get", "/error2/check_names").Return([]byte("[\"first_name\"]"), nil).Times(1)
	backend.On("Get", "/error2/init_configs").Return(nil, fmt.Errorf("some error")).Times(1)
	res = zk.getTemplates("/error2/")
	assert.Nil(t, res)

	backend.On("Get", "/error3/check_names").Return([]byte("[\"first_name\"]"), nil).Times(1)
	backend.On("Get", "/error3/init_configs").Return([]byte("[{}]"), nil).Times(1)
	backend.On("Get", "/error3/instances").Return(nil, fmt.Errorf("some error")).Times(1)
	res = zk.getTemplates("/error3/")
	assert.Nil(t, res)

	backend.On("Get", "/error4/check_names").Return([]byte("[\"first_name\"]"), nil).Times(1)
	backend.On("Get", "/error4/instances").Return([]byte("[{}]"), nil).Times(1)
	backend.On("Get", "/error4/init_configs").Return([]byte("[{}, {}]"), nil).Times(1)
	res = zk.getTemplates("/error4/")
	assert.Len(t, res, 0)

	backend.On("Get", "/error5/check_names").Return([]byte(""), nil).Times(1)
	backend.On("Get", "/error5/instances").Return([]byte("[{}]"), nil).Times(1)
	backend.On("Get", "/error5/init_configs").Return([]byte("[{}]"), nil).Times(1)
	res = zk.getTemplates("/error5/")
	assert.Len(t, res, 0)

	backend.On("Get", "/config/check_names").Return([]byte("[\"first_name\", \"second_name\"]"), nil).Times(1)
	backend.On("Get", "/config/instances").Return([]byte("[{\"test\": 21, \"test2\": \"data\"}, {\"data1\": \"21\", \"data2\": {\"number\": 21}}]"), nil).Times(1)
	backend.On("Get", "/config/init_configs").Return([]byte("[{\"a\": \"b\"}, {}]"), nil).Times(1)
	//zk = ZookeeperConfigProvider{client: backend}
	res = zk.getTemplates("/config/")
	assert.NotNil(t, res)
	assert.Len(t, res, 2)

	assert.Equal(t, check.ID("/config/"), res[0].ID)
	assert.Equal(t, "first_name", res[0].Name)
	assert.Equal(t, "{\"a\":\"b\"}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.Equal(t, "{\"test\":21,\"test2\":\"data\"}", string(res[0].Instances[0]))

	assert.Equal(t, check.ID("/config/"), res[1].ID)
	assert.Equal(t, "second_name", res[1].Name)
	assert.Equal(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.Equal(t, "{\"data1\":\"21\",\"data2\":{\"number\":21}}", string(res[1].Instances[0]))
}

func TestZKCollect(t *testing.T) {
	backend := &zkTest{}

	backend.On("Children", "/datadog/check_configs").Return([]string{"other", "config_folder_1", "config_folder_2"}, nil).Times(1)
	backend.On("Children", "/datadog/check_configs/other").Return([]string{"test", "check_names"}, nil).Times(1)

	backend.On("Children", "/datadog/check_configs/config_folder_1").Return([]string{"check_names", "instances", "init_configs"}, nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_1/check_names").Return([]byte("[\"first_name\", \"second_name\"]"), nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_1/instances").Return([]byte("[{}, {}]"), nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_1/init_configs").Return([]byte("[{}, {}]"), nil).Times(1)

	backend.On("Children", "/datadog/check_configs/config_folder_2").Return([]string{"check_names", "instances", "init_configs", "test"}, nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_2/check_names").Return([]byte("[\"third_name\"]"), nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_2/instances").Return([]byte("[{}]"), nil).Times(1)
	backend.On("Get", "/datadog/check_configs/config_folder_2/init_configs").Return([]byte("[{}]"), nil).Times(1)

	zk := ZookeeperConfigProvider{client: backend}

	res, err := zk.Collect()
	assert.Nil(t, err)
	assert.Len(t, res, 3)

	assert.Equal(t, check.ID("/datadog/check_configs/config_folder_1"), res[0].ID)
	assert.Equal(t, "first_name", res[0].Name)
	assert.Equal(t, "{}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.Equal(t, "{}", string(res[0].Instances[0]))

	assert.Equal(t, check.ID("/datadog/check_configs/config_folder_1"), res[1].ID)
	assert.Equal(t, "second_name", res[1].Name)
	assert.Equal(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.Equal(t, "{}", string(res[1].Instances[0]))

	assert.Equal(t, check.ID("/datadog/check_configs/config_folder_2"), res[2].ID)
	assert.Equal(t, "third_name", res[2].Name)
	assert.Equal(t, "{}", string(res[2].InitConfig))
	require.Len(t, res[2].Instances, 1)
	assert.Equal(t, "{}", string(res[2].Instances[0]))
}
