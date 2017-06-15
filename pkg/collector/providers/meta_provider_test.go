package providers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

type providerTest struct {
	mock.Mock
}

//
// Mock
//

func (m *providerTest) List(key string) ([]string, error) {
	args := m.Called(key)
	if v, ok := args.Get(0).([]string); ok {
		return v, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *providerTest) ListName(key string) ([]string, error) {
	args := m.Called(key)
	return args.Get(0).([]string), args.Error(1)
}

func (m *providerTest) Get(key string) ([]byte, error) {
	args := m.Called(key)
	return args.Get(0).([]byte), args.Error(1)
}

//
// Tests
//

func TestInitClient(t *testing.T) {
	testClient := Client{}
	backend := &providerTest{}

	testClient.Init(backend)
	assert.Equal(t, config.Datadog.GetString("autoconf_template_dir"), testClient.tplDir)
	assert.Equal(t, backend, testClient.backend)
}

func TestBuildStoreKey(t *testing.T) {
	testClient := Client{}

	assert.Equal(t, "a/b/c", testClient.buildStoreKey("a", "b", "c"))
	assert.Equal(t, "a", testClient.buildStoreKey("a"))
}

func TestGetIdentifiers(t *testing.T) {
	backend := &providerTest{}

	backend.On("List", "/test/").Return(nil, fmt.Errorf("some error")).Times(1)
	backend.On("List", "/datadog/tpl").Return([]string{"/datadog/tpl/nginx",
		"/datadog/tpl/redis",
		"/datadog/tpl/incomplete"}, nil).Times(1)

	expectedKeys := []string{checkNamePath, initConfigPath, instancePath}
	backend.On("ListName", "/datadog/tpl/nginx").Return(expectedKeys, nil).Times(1)
	backend.On("ListName", "/datadog/tpl/redis").Return(append(expectedKeys, "an extra one"), nil).Times(1)
	backend.On("ListName", "/datadog/tpl/incomplete").Return([]string{checkNamePath, "other one"}, nil).Times(1)

	testClient := Client{}
	testClient.Init(backend)

	res, err := testClient.getIdentifiers("/test/")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	res, err = testClient.getIdentifiers("/datadog/tpl")
	require.Nil(t, err)

	assert.Len(t, res, 2)
	assert.Equal(t, []string{"/datadog/tpl/nginx", "/datadog/tpl/redis"}, res)
	backend.AssertExpectations(t)
}

func TestGetCheckNames(t *testing.T) {
	backend := &providerTest{}

	backend.On("Get", "test_empty").Return([]byte{}, nil).Times(1)
	backend.On("Get", "error").Return([]byte{}, fmt.Errorf("some error")).Times(1)
	backend.On("Get", "bad_json").Return([]byte("[\"test\""), nil).Times(1)
	backend.On("Get", "correct").Return([]byte("[\"nginx\", \"redis\", \"some other\"]"), nil).Times(1)

	testClient := Client{}
	testClient.Init(backend)

	res, err := testClient.getCheckNames("test_empty")
	assert.Nil(t, res)
	assert.NotNil(t, err)
	assert.Equal(t, "check_names is empty", err.Error())

	res, err = testClient.getCheckNames("error")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	res, err = testClient.getCheckNames("bad_json")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	res, err = testClient.getCheckNames("correct")
	assert.Equal(t, []string{"nginx", "redis", "some other"}, res)
	assert.Nil(t, err)

	backend.AssertExpectations(t)
}

func TestGetJSONValue(t *testing.T) {
	backend := &providerTest{}

	backend.On("Get", "test_empty").Return([]byte{}, nil).Times(1)
	backend.On("Get", "error").Return([]byte{}, fmt.Errorf("some error")).Times(1)
	backend.On("Get", "bad_json").Return([]byte("[\"test\"]]"), nil).Times(1)
	backend.On("Get", "wrong_json_type").Return([]byte("[{}, \"test\"]"), nil).Times(1)
	backend.On("Get", "correct").Return([]byte("[{\"nginx_status_url\": \"http://%%host%%/nginx_status\"}, {\"name\": \"test\", \"url\": \"http://%%host%%/\", \"timeout\": 1}, {}]"), nil).Times(1)

	testClient := Client{}
	testClient.Init(backend)

	res, err := testClient.getJSONValue("test_empty")
	assert.Nil(t, res)
	assert.NotNil(t, err)
	assert.Equal(t, "Value at test_empty is empty", err.Error())

	res, err = testClient.getJSONValue("error")
	assert.Nil(t, res)
	assert.NotNil(t, err)
	assert.Equal(t, "some error", err.Error())

	res, err = testClient.getJSONValue("wrong_json_type")
	assert.Nil(t, res)
	assert.NotNil(t, err)
	assert.Equal(t, "found non JSON object type at key 'wrong_json_type', value is: 'test'", err.Error())

	res, err = testClient.getJSONValue("bad_json")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	res, err = testClient.getJSONValue("correct")
	assert.Equal(t, []check.ConfigData{
		[]byte("{\"nginx_status_url\":\"http://%%host%%/nginx_status\"}"),
		[]byte("{\"name\":\"test\",\"timeout\":1,\"url\":\"http://%%host%%/\"}"),
		[]byte("{}"),
	}, res)
	assert.Nil(t, err)

	backend.AssertExpectations(t)
}

func TestGetTemplates(t *testing.T) {
	backend := &providerTest{}
	testClient := Client{}

	backend.On("Get", "/error1/check_names").Return([]byte{}, fmt.Errorf("some error")).Times(1)
	testClient.Init(backend)
	res := testClient.getTemplates("/error1/")
	assert.Nil(t, res)

	backend.On("Get", "/error2/check_names").Return([]byte("[\"first_name\"]"), nil).Times(1)
	backend.On("Get", "/error2/init_configs").Return([]byte{}, fmt.Errorf("some error")).Times(1)
	testClient.Init(backend)
	res = testClient.getTemplates("/error2/")
	assert.Nil(t, res)

	backend.On("Get", "/error3/check_names").Return([]byte("[\"first_name\"]"), nil).Times(1)
	backend.On("Get", "/error3/init_configs").Return([]byte("[{}]"), nil).Times(1)
	backend.On("Get", "/error3/instances").Return([]byte{}, fmt.Errorf("some error")).Times(1)
	testClient.Init(backend)
	res = testClient.getTemplates("/error3/")
	assert.Nil(t, res)

	backend.On("Get", "/error4/check_names").Return([]byte("[\"first_name\"]"), nil).Times(1)
	backend.On("Get", "/error4/instances").Return([]byte("[{}]"), nil).Times(1)
	backend.On("Get", "/error4/init_configs").Return([]byte("[{}, {}]"), nil).Times(1)
	testClient.Init(backend)
	res = testClient.getTemplates("/error4/")
	assert.Nil(t, res)

	backend.On("Get", "/config/check_names").Return([]byte("[\"first_name\", \"second_name\"]"), nil).Times(1)
	backend.On("Get", "/config/instances").Return([]byte("[{\"test\": 21, \"test2\": \"data\"}, {\"data1\": \"21\", \"data2\": {\"number\": 21}}]"), nil).Times(1)
	backend.On("Get", "/config/init_configs").Return([]byte("[{\"a\": \"b\"}, {}]"), nil).Times(1)
	testClient.Init(backend)
	res = testClient.getTemplates("/config/")
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

func TestCollect(t *testing.T) {
	backend := &providerTest{}

	backend.On("List", "/root").Return([]string{"/root/other", "/root/config_folder_1", "/root/config_folder_2"}, nil).Times(1)

	backend.On("ListName", "/root/other").Return([]string{"test", "check_names"}, nil).Times(1)
	backend.On("ListName", "/root/config_folder_1").Return([]string{"check_names", "instances", "init_configs"}, nil).Times(1)
	backend.On("ListName", "/root/config_folder_2").Return([]string{"check_names", "instances", "init_configs", "test"}, nil).Times(1)

	backend.On("Get", "/root/config_folder_1/check_names").Return([]byte("[\"first_name\", \"second_name\"]"), nil).Times(1)
	backend.On("Get", "/root/config_folder_1/instances").Return([]byte("[{}, {}]"), nil).Times(1)
	backend.On("Get", "/root/config_folder_1/init_configs").Return([]byte("[{}, {}]"), nil).Times(1)

	backend.On("Get", "/root/config_folder_2/check_names").Return([]byte("[\"third_name\"]"), nil).Times(1)
	backend.On("Get", "/root/config_folder_2/instances").Return([]byte("[{}]"), nil).Times(1)
	backend.On("Get", "/root/config_folder_2/init_configs").Return([]byte("[{}]"), nil).Times(1)

	testClient := Client{}
	testClient.Init(backend)
	testClient.tplDir = "/root"

	res, err := testClient.Collect()
	assert.Nil(t, err)
	assert.Len(t, res, 3)

	assert.Equal(t, check.ID("/root/config_folder_1"), res[0].ID)
	assert.Equal(t, "first_name", res[0].Name)
	assert.Equal(t, "{}", string(res[0].InitConfig))
	require.Len(t, res[0].Instances, 1)
	assert.Equal(t, "{}", string(res[0].Instances[0]))

	assert.Equal(t, check.ID("/root/config_folder_1"), res[1].ID)
	assert.Equal(t, "second_name", res[1].Name)
	assert.Equal(t, "{}", string(res[1].InitConfig))
	require.Len(t, res[1].Instances, 1)
	assert.Equal(t, "{}", string(res[1].Instances[0]))

	assert.Equal(t, check.ID("/root/config_folder_2"), res[2].ID)
	assert.Equal(t, "third_name", res[2].Name)
	assert.Equal(t, "{}", string(res[2].InitConfig))
	require.Len(t, res[2].Instances, 1)
	assert.Equal(t, "{}", string(res[2].Instances[0]))
}
