// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package create

import (
	"testing"
)

func TestCreateFromParms(t *testing.T) {
	/*
		defer func(val string, ok bool) {
			if ok {
				os.Setenv("DD_CONF_NODETREEMODEL", val)
			}
		}(os.LookupEnv("DD_CONF_NODETREEMODEL"))
		os.Unsetenv("DD_CONF_NODETREEMODEL")

		m := NewConfig("test", "")
		assert.Equal(t, "viper", m.GetLibType())

		m = NewConfig("test", "enable")
		assert.Equal(t, "nodetreemodel", m.GetLibType())

		m = NewConfig("test", "tee")
		assert.Equal(t, "tee", m.GetLibType())

		m = NewConfig("test", "enable-tee")
		assert.Equal(t, "tee", m.GetLibType())

		m = NewConfig("test", "something invalid")
		assert.Equal(t, "viper", m.GetLibType())

		defer func(orig string) {
			version.AgentVersion = orig
		}(version.AgentVersion)

		version.AgentVersion = "7.75.2"

		m = NewConfig("test", "7.75")
		assert.Equal(t, "viper", m.GetLibType())

		m = NewConfig("test", "7.76.0")
		assert.Equal(t, "viper", m.GetLibType())

		m = NewConfig("test", "7.75.3")
		assert.Equal(t, "viper", m.GetLibType())

		m = NewConfig("test", "7.80.0")
		assert.Equal(t, "viper", m.GetLibType())

		m = NewConfig("test", "6.80.0")
		assert.Equal(t, "nodetreemodel", m.GetLibType())

		m = NewConfig("test", "7.75.0")
		assert.Equal(t, "nodetreemodel", m.GetLibType())
	*/
}

func TestCreateFromEnv(t *testing.T) {
	/*
		defer func(val string, ok bool) {
			if ok {
				os.Setenv("DD_CONF_NODETREEMODEL", val)
			}
		}(os.LookupEnv("DD_CONF_NODETREEMODEL"))
		os.Unsetenv("DD_CONF_NODETREEMODEL")

		m := NewConfig("test", "")
		assert.Equal(t, "viper", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "enable")
		m = NewConfig("test", "")
		assert.Equal(t, "nodetreemodel", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "tee")
		m = NewConfig("test", "")
		assert.Equal(t, "tee", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "enable-tee")
		m = NewConfig("test", "")
		assert.Equal(t, "tee", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "something invalid")
		m = NewConfig("test", "")
		assert.Equal(t, "viper", m.GetLibType())

		defer func(orig string) {
			version.AgentVersion = orig
		}(version.AgentVersion)

		version.AgentVersion = "7.75.2"

		t.Setenv("DD_CONF_NODETREEMODEL", "7.75")
		m = NewConfig("test", "")
		assert.Equal(t, "viper", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "7.76.0")
		m = NewConfig("test", "")
		assert.Equal(t, "viper", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "7.75.3")
		m = NewConfig("test", "")
		assert.Equal(t, "viper", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "7.80")
		m = NewConfig("test", "")
		assert.Equal(t, "viper", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "6.80.0")
		m = NewConfig("test", "")
		assert.Equal(t, "nodetreemodel", m.GetLibType())

		t.Setenv("DD_CONF_NODETREEMODEL", "7.75.0")
		m = NewConfig("test", "")
		assert.Equal(t, "nodetreemodel", m.GetLibType())
	*/
}
