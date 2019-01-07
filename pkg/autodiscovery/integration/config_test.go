// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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

func TestString(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	config.Name = "foo"
	config.InitConfig = Data("fooBarBaz: test")
	config.Instances = []Data{Data("justFoo")}

	expected := `init_config:
  fooBarBaz: test
instances:
- justFoo
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
		Instances:  []Data{Data("{foo:bar}")},
	}
	assert.Equal(t, "d8cbc7186ba13533", simpleConfig.Digest())
	simpleConfigWithLogs := &Config{
		Name:       "foo",
		InitConfig: Data(""),
		Instances:  []Data{Data("{foo:bar}")},
		LogsConfig: Data("[{\"service\":\"any_service\",\"source\":\"any_source\"}]"),
	}
	assert.Equal(t, "6253da85b1624771", simpleConfigWithLogs.Digest())
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
