// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHostname(t *testing.T) {
	hostnameFile, err := writeTempHostnameFile("expectedfilehostname")
	if !require.Nil(t, err) {
		return
	}
	defer os.RemoveAll(hostnameFile)

	defer func(val string) { config.Datadog.Set("hostname_file", val) }(config.Datadog.GetString("hostname_file"))
	config.Datadog.Set("hostname_file", hostnameFile)

	hostname, err := fromHostnameFile(context.TODO(), "")
	if !assert.Nil(t, err) {
		return
	}

	assert.Equal(t, "expectedfilehostname", hostname)
}

func TestGetHostnameWhitespaceTrim(t *testing.T) {
	hostnameFile, err := writeTempHostnameFile("  \n\r expectedfilehostname  \r\n\n ")
	if !require.Nil(t, err) {
		return
	}
	defer os.RemoveAll(hostnameFile)

	defer func(val string) { config.Datadog.Set("hostname_file", val) }(config.Datadog.GetString("hostname_file"))
	config.Datadog.Set("hostname_file", hostnameFile)

	hostname, err := fromHostnameFile(context.TODO(), "")
	if !assert.Nil(t, err) {
		return
	}

	assert.Equal(t, "expectedfilehostname", hostname)
}

func TestGetHostnameNoFilenameOption(t *testing.T) {
	_, err := fromHostnameFile(context.TODO(), "")
	assert.NotNil(t, err)
}

func TestGetHostnameInvalidHostname(t *testing.T) {
	hostnameFile, err := writeTempHostnameFile(strings.Repeat("a", 256))
	if !require.Nil(t, err) {
		return
	}
	defer os.RemoveAll(hostnameFile)

	defer func(val string) { config.Datadog.Set("hostname_file", val) }(config.Datadog.GetString("hostname_file"))
	config.Datadog.Set("hostname_file", hostnameFile)

	_, err = fromHostnameFile(context.TODO(), "")
	assert.NotNil(t, err)
}

func writeTempHostnameFile(content string) (string, error) {
	destFile, err := ioutil.TempFile("", "test-hostname-file-config-")
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(destFile.Name(), []byte(content), os.ModePerm)
	if err != nil {
		os.RemoveAll(destFile.Name())
		return "", err
	}

	return destFile.Name(), nil
}
