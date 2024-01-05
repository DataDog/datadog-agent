// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverlessdebug

package tmpdebug

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type TestClientSuccess struct {
	key  []string
	body []string
}

func (tc *TestClientSuccess) putObject(bucket string, key string, file *os.File) error {
	buf := new(strings.Builder)
	io.Copy(buf, file)
	tc.body = append(tc.body, buf.String())
	tc.key = append(tc.key, key)
	return nil
}

func TestBlockingS3Upload(t *testing.T) {
	currentTime := time.Now()
	expectedKeyPrefix := currentTime.Format("2006-01-02")
	client := &TestClientSuccess{
		key:  make([]string, 0),
		body: make([]string, 0),
	}
	BlockingS3Upload("./testdata", "myTestBucket", client)
	assert.Equal(t, client.body[0], "aaa")
	assert.Equal(t, client.body[1], "bbb")
	assert.True(t, strings.HasPrefix(client.key[0], expectedKeyPrefix))
	assert.True(t, strings.HasPrefix(client.key[1], expectedKeyPrefix))
}
