// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package net

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNTPServersFromFileNotExist(t *testing.T) {
	_, err := getNTPServersFromFiles([]string{"file1", "file2"})
	assert.EqualError(t, err, "Cannot find ntp server in file1, file2")
}

func createTempFile(t *testing.T, content string, callback func(filename string)) {
	file, err := ioutil.TempFile("", "")

	filename := file.Name()
	defer os.Remove(filename)
	assert.NoError(t, err)

	ioutil.WriteFile(filename, []byte(content), 0)
	callback(filename)
}

func TestGetNTPServersFromFile(t *testing.T) {
	config := `
		# --- GENERAL CONFIGURATION ---
		server  aaa.bbb.ccc.ddd
		 server  127.127.1.0
		#server  127.0.0.1
		fudge   127.127.1.0 stratum 10
		`
	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		assert.Equal(t, []string{"aaa.bbb.ccc.ddd", "127.127.1.0"}, servers)
	})
}

func TestGetNTPServersFromFileTwoConfigs(t *testing.T) {
	config1 := "server  aaa.bbb.ccc.ddd"
	config2 := "server  127.0.0.1"

	createTempFile(t, config1, func(f1 string) {
		createTempFile(t, config2, func(f2 string) {
			servers, err := getNTPServersFromFiles([]string{f1, f2})
			assert.NoError(t, err)
			assert.Equal(t, []string{"aaa.bbb.ccc.ddd", "127.0.0.1"}, servers)
		})
	})
}

func TestGetNTPServersFromFileNoDuplicate(t *testing.T) {
	config := `
server  aaa.bbb.ccc.ddd
server  aaa.bbb.ccc.ddd
`
	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		assert.Equal(t, []string{"aaa.bbb.ccc.ddd"}, servers)
	})
}
