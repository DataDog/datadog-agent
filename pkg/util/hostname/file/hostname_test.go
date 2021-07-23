// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHostname(t *testing.T) {
	hostnameFile, err := writeTempHostnameFile("expectedfilehostname")
	if !assert.Nil(t, err) {
		return
	}
	defer os.RemoveAll(hostnameFile)

	options := map[string]interface{}{
		"filename": hostnameFile,
	}

	hostname, err := HostnameProvider(context.TODO(), options)
	if !assert.Nil(t, err) {
		return
	}

	assert.Equal(t, "expectedfilehostname", hostname)
}

func TestGetHostnameWhitespaceTrim(t *testing.T) {
	hostnameFile, err := writeTempHostnameFile("  \n\r expectedfilehostname  \r\n\n ")
	if !assert.Nil(t, err) {
		return
	}
	defer os.RemoveAll(hostnameFile)

	options := map[string]interface{}{
		"filename": hostnameFile,
	}

	hostname, err := HostnameProvider(context.TODO(), options)
	if !assert.Nil(t, err) {
		return
	}

	assert.Equal(t, "expectedfilehostname", hostname)
}

func TestGetHostnameNoOptions(t *testing.T) {
	_, err := HostnameProvider(context.TODO(), nil)
	assert.NotNil(t, err)
}

func TestGetHostnameNoFilenameOption(t *testing.T) {
	options := map[string]interface{}{
		"one": "one",
		"two": "two",
	}

	_, err := HostnameProvider(context.TODO(), options)
	assert.NotNil(t, err)
}

func TestGetHostnameInvalidHostname(t *testing.T) {
	hostnameFile, err := writeTempHostnameFile(strings.Repeat("a", 256))
	if !assert.Nil(t, err) {
		return
	}
	defer os.RemoveAll(hostnameFile)

	options := map[string]interface{}{
		"filename": hostnameFile,
	}

	_, err = HostnameProvider(context.TODO(), options)
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
