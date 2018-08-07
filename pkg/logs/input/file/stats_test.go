// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package file

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCtimeShouldFailWhenFileDoesNotExist(t *testing.T) {
	_, err := Ctime("")
	assert.NotNil(t, err)
}

func TestCtimeShouldSucceedWhenFileExists(t *testing.T) {
	dir, err := ioutil.TempDir("", "log-scanner-test-")
	assert.Nil(t, err)

	path := fmt.Sprintf("%s/file.log", dir)
	_, err = os.Create(path)
	assert.Nil(t, err)

	_, err = Ctime(path)
	assert.Nil(t, err)
}
