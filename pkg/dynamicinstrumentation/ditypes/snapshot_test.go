// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ditypes

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDynamicInstrumentationLogJSONRoundTrip(t *testing.T) {
	files := []string{
		"testdata/snapshot-00.json",
		"testdata/snapshot-01.json",
		"testdata/snapshot-02.json",
	}
	for _, filePath := range files {
		file, err := os.Open(filePath)
		if err != nil {
			t.Error(err)
		}
		defer file.Close()

		bytes, err := io.ReadAll(file)
		if err != nil {
			t.Error(err)
		}

		var s SnapshotUpload
		err = json.Unmarshal(bytes, &s)
		if err != nil {
			t.Error(err)
		}

		mBytes, err := json.Marshal(s)
		if err != nil {
			t.Error(err)
		}

		assert.JSONEq(t, string(bytes), string(mBytes))
	}
}
