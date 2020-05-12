// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package integration

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	yaml "gopkg.in/yaml.v2"
)

func TestConfigEqual(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	another := &Config{}
	assert.True(t, config.Equal(another))

	another.Name = "foo"
	assert.False(t, config.Equal(another))
	config.Name = another.Name
	assert.True(t, config.Equal(another))

	another.InitConfig = Data("{fooBarBaz}")
	assert.False(t, config.Equal(another))
	config.InitConfig = another.InitConfig
	assert.True(t, config.Equal(another))

	another.Instances = []Data{Data("{justFoo}")}
	assert.False(t, config.Equal(another))
	config.Instances = another.Instances
	assert.True(t, config.Equal(another))

	config.ADIdentifiers = []string{"foo", "bar"}
	assert.False(t, config.Equal(another))
	another.ADIdentifiers = []string{"foo", "bar"}
	assert.True(t, config.Equal(another))
	another.ADIdentifiers = []string{"bar", "foo"}
	assert.False(t, config.Equal(another))

	checkConfigWithOrderedTags := &Config{
		Name:       "test",
		InitConfig: Data("{foo}"),
		Instances:  []Data{Data("tags: [\"bar:foo\", \"foo:bar\"]")},
		LogsConfig: Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
	}
	checkConfigWithUnorderedTags := &Config{
		Name:       "test",
		InitConfig: Data("{foo}"),
		Instances:  []Data{Data("tags: [\"foo:bar\", \"bar:foo\"]")},
		LogsConfig: Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
	}
	assert.Equal(t, checkConfigWithOrderedTags.Digest(), checkConfigWithUnorderedTags.Digest())
}

func TestIsLogConfig(t *testing.T) {
	config := &Config{}
	assert.False(t, config.IsLogConfig())
	config.Instances = []Data{Data("tags: [\"foo:bar\", \"bar:foo\"]")}
	assert.False(t, config.IsLogConfig())
	config.LogsConfig = Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]")
	assert.True(t, config.IsLogConfig())
}

func TestIsCheckConfig(t *testing.T) {
	config := &Config{}
	assert.False(t, config.IsCheckConfig())
	config.Instances = []Data{Data("tags: [\"foo:bar\", \"bar:foo\"]")}
	assert.True(t, config.IsCheckConfig())
	config.ClusterCheck = true
	assert.False(t, config.IsCheckConfig())
}

func TestString(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	config.Name = "foo"
	config.InitConfig = Data("fooBarBaz: test")
	config.Instances = []Data{Data("justFoo")}

	expected := `check_name: foo
init_config:
  fooBarBaz: test
instances:
- justFoo
logs_config: null
`
	assert.Equal(t, config.String(), expected)
}

func TestMergeAdditionalTags(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	config.Name = "foo"
	config.InitConfig = Data("fooBarBaz")
	config.Instances = []Data{Data("tags: [\"foo\", \"foo:bar\"]")}

	config.Instances[0].MergeAdditionalTags([]string{"foo", "bar"})

	rawConfig := RawMap{}
	err := yaml.Unmarshal(config.Instances[0], &rawConfig)
	assert.Nil(t, err)
	assert.Contains(t, rawConfig["tags"], "foo")
	assert.Contains(t, rawConfig["tags"], "bar")
	assert.Contains(t, rawConfig["tags"], "foo:bar")

	config.Name = "foo"
	config.InitConfig = Data("fooBarBaz")
	config.Instances = []Data{Data("other: foo")}

	config.Instances[0].MergeAdditionalTags([]string{"foo", "bar"})

	rawConfig = RawMap{}
	err = yaml.Unmarshal(config.Instances[0], &rawConfig)
	assert.Nil(t, err)
	assert.Contains(t, rawConfig["tags"], "foo")
	assert.Contains(t, rawConfig["tags"], "bar")
}

func TestSetField(t *testing.T) {
	instance := Data("onefield: true\ntags: [\"foo\", \"foo:bar\"]")

	// Add new field
	instance.SetField("otherfield", 50)
	rawConfig := RawMap{}
	err := yaml.Unmarshal(instance, &rawConfig)
	assert.Nil(t, err)
	assert.Contains(t, rawConfig["tags"], "foo")
	assert.Contains(t, rawConfig["tags"], "foo:bar")
	assert.Equal(t, true, rawConfig["onefield"])
	assert.Equal(t, 50, rawConfig["otherfield"])

	// Override existing field
	instance.SetField("onefield", "testing")
	rawConfig = RawMap{}
	err = yaml.Unmarshal(instance, &rawConfig)
	assert.Nil(t, err)
	assert.Contains(t, rawConfig["tags"], "foo")
	assert.Contains(t, rawConfig["tags"], "foo:bar")
	assert.Equal(t, "testing", rawConfig["onefield"])
	assert.Equal(t, 50, rawConfig["otherfield"])
}

func TestDigest(t *testing.T) {
	emptyConfig := &Config{}
	assert.Equal(t, "cbf29ce484222325", emptyConfig.Digest())
	simpleConfig := &Config{
		Name:       "foo",
		InitConfig: Data(""),
	}
	assert.Equal(t, "d8cbc7186ba13533", simpleConfig.Digest())
	simpleConfigWithLogs := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		LogsConfig: Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
	}
	assert.Equal(t, "6253da85b1624771", simpleConfigWithLogs.Digest())
	simpleConfigWithInstances := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		Instances:  []Data{Data("{foo:bar}")},
	}
	assert.Equal(t, "af69af81e022838d", simpleConfigWithInstances.Digest())
	simpleConfigWithInstancesAndLogs := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		Instances:  []Data{Data("{foo:bar}")},
		LogsConfig: Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
	}
	assert.Equal(t, "35def8f2c169307f", simpleConfigWithInstancesAndLogs.Digest())
	simpleConfigWithTags := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		Instances:  []Data{Data("tags: [\"foo\", \"foo:bar\"]")},
	}
	assert.Equal(t, "87e916adf286a67f", simpleConfigWithTags.Digest())
	simpleConfigWithOtherTags := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		Instances:  []Data{Data("tags: [\"foo\", \"foo:baf\"]")},
	}
	assert.Equal(t, "87e902adf2868473", simpleConfigWithOtherTags.Digest())

	// assert a character change in a tag produces different hash
	assert.NotEqual(t, simpleConfigWithTags.Digest(), simpleConfigWithOtherTags.Digest())

	simpleConfigWithTagsDifferentOrder := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		Instances:  []Data{Data("tags: [\"foo:bar\", \"foo\"]")},
	}
	assert.Equal(t, "87e916adf286a67f", simpleConfigWithTagsDifferentOrder.Digest())

	// assert an order change in the tags list doesn't change the hash
	assert.Equal(t, simpleConfigWithTags.Digest(), simpleConfigWithTagsDifferentOrder.Digest())

	simpleClusterCheckConfig := &Config{
		Name:         "foo",
		InitConfig:   Data(""),
		ClusterCheck: true,
	}
	assert.Equal(t, "d8cbc7186ba13533", simpleClusterCheckConfig.Digest())

	// assert the ClusterCheck field is not taken into account
	assert.Equal(t, simpleConfig.Digest(), simpleClusterCheckConfig.Digest())

	configWithEntity := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		Entity:     "docker://f556178a47cf65fb70cd5772a9e80e661f71e021da49d3dc99565b861707041c",
	}
	assert.Equal(t, "41b0c2b6f9c4f3e0", configWithEntity.Digest())

	configWithAnotherEntity := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		Entity:     "docker://ddcd8a64616772f7ad4524f09fd75c9e3a265144050fc077563e63ea2eb46db0",
	}
	assert.Equal(t, "b5c2d0fd13694bf0", configWithAnotherEntity.Digest())

	// assert an entity change produces different hash
	assert.NotEqual(t, configWithEntity.Digest(), configWithAnotherEntity.Digest())
}

func TestGetNameForInstance(t *testing.T) {
	config := &Config{}

	config.Name = "foo"
	config.InitConfig = Data("fooBarBaz")
	config.Instances = []Data{Data("name: foobar")}
	assert.Equal(t, config.Instances[0].GetNameForInstance(), "foobar")

	config.Name = "foo"
	config.InitConfig = Data("fooBarBaz")
	config.Instances = []Data{Data("namespace: foobar\nname: bar")}
	assert.Equal(t, config.Instances[0].GetNameForInstance(), "bar")

	config.Name = "foo"
	config.InitConfig = Data("fooBarBaz")
	config.Instances = []Data{Data("namespace: foobar")}
	assert.Equal(t, config.Instances[0].GetNameForInstance(), "foobar")

	config.Name = "foo"
	config.InitConfig = Data("fooBarBaz")
	config.Instances = []Data{Data("foo: bar")}
	assert.Equal(t, config.Instances[0].GetNameForInstance(), "")
}

// this is here to prevent compiler optimization on the benchmarking code
var result string

func BenchmarkID(b *testing.B) {
	var id string // store return value to avoid the compiler to strip the function call
	config := &Config{}
	config.InitConfig = make([]byte, 32000)
	config.Instances = []Data{make([]byte, 32000)}
	config.LogsConfig = make([]byte, 32000)
	rand.Read(config.InitConfig)
	rand.Read(config.Instances[0])
	rand.Read(config.LogsConfig)
	for n := 0; n < b.N; n++ {
		id = config.Digest()
	}
	result = id
}
