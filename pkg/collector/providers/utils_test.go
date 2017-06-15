package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

func TestParseJSONValue(t *testing.T) {
	// empty value
	res, err := parseJSONValue("")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// value is not a list
	res, err = parseJSONValue("{}")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// invalid json
	res, err = parseJSONValue("[{]")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// bad type
	res, err = parseJSONValue("[1, {\"test\": 1}, \"test\"]")
	assert.Nil(t, res)
	assert.NotNil(t, err)
	assert.Equal(t, "found non JSON object type, value is: '1'", err.Error())

	// valid input
	res, err = parseJSONValue("[{\"test\": 1}, {\"test\": 2}]")
	assert.Nil(t, err)
	assert.NotNil(t, res)
	require.Len(t, res, 2)
	assert.Equal(t, check.ConfigData("{\"test\":1}"), res[0])
	assert.Equal(t, check.ConfigData("{\"test\":2}"), res[1])
}

func TestParseCheckNames(t *testing.T) {
	// empty value
	res, err := parseCheckNames("")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// value is not a list
	res, err = parseCheckNames("{}")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// invalid json
	res, err = parseCheckNames("[{]")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// ignore bad type
	res, err = parseCheckNames("[1, {\"test\": 1}, \"test\"]")
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// valid input
	res, err = parseCheckNames("[\"test1\", \"test2\"]")
	assert.Nil(t, err)
	assert.NotNil(t, res)
	require.Len(t, res, 2)
	assert.Equal(t, []string{"test1", "test2"}, res)
}

func TestBuildStoreKey(t *testing.T) {
	res := buildStoreKey()
	assert.Equal(t, "/datadog/check_configs", res)
	res = buildStoreKey("")
	assert.Equal(t, "/datadog/check_configs", res)
	res = buildStoreKey("foo")
	assert.Equal(t, "/datadog/check_configs/foo", res)
	res = buildStoreKey("foo", "bar")
	assert.Equal(t, "/datadog/check_configs/foo/bar", res)
	res = buildStoreKey("foo", "bar", "bazz")
	assert.Equal(t, "/datadog/check_configs/foo/bar/bazz", res)
}

func TestBuildTemplates(t *testing.T) {
	// wrong number of checkNames
	res := buildTemplates("id",
		[]string{"a", "b"},
		[]check.ConfigData{check.ConfigData("")},
		[]check.ConfigData{check.ConfigData("")})
	assert.Len(t, res, 0)

	res = buildTemplates("id",
		[]string{"a", "b"},
		[]check.ConfigData{check.ConfigData("{\"test\": 1}"), check.ConfigData("{}")},
		[]check.ConfigData{check.ConfigData("{}"), check.ConfigData("{1:2}")})
	require.Len(t, res, 2)

	assert.Equal(t, res[0].ID, check.ID("id"))
	assert.Equal(t, res[0].Name, "a")
	assert.Equal(t, res[0].InitConfig, check.ConfigData("{\"test\": 1}"))
	assert.Equal(t, res[0].Instances, []check.ConfigData{check.ConfigData("{}")})

	assert.Equal(t, res[1].ID, check.ID("id"))
	assert.Equal(t, res[1].Name, "b")
	assert.Equal(t, res[1].InitConfig, check.ConfigData("{}"))
	assert.Equal(t, res[1].Instances, []check.ConfigData{check.ConfigData("{1:2}")})
}
