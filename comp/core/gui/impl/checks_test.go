// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadConfDir(t *testing.T) {
	testdata := testdataDir(t)
	files, err := readConfDir(testdata)
	assert.NoError(t, err)

	sort.Strings(files)
	expected := []string{
		"check.yaml",
		"check.yaml.default",
		"check.yaml.example",
		"foo.d/conf.yaml",
		"foo.d/conf.yaml.default",
		"foo.d/conf.yaml.example",
		"foo.d/metrics.yaml",
	}

	assert.Equal(t, expected, files)
}

func TestConfigsInPath(t *testing.T) {
	testdata := testdataDir(t)
	files, err := getConfigsInPath(testdata)
	assert.NoError(t, err)

	sort.Strings(files)
	expected := []string{
		"check.yaml",
		"check.yaml.example",
		"foo.d/conf.yaml",
		"foo.d/conf.yaml.example",
	}

	assert.Equal(t, expected, files)
}

func TestGetFileNameAndFolder(t *testing.T) {
	type vars map[string]string
	tests := []struct {
		name            string
		vars            vars
		wantFileName    string
		wantCheckFolder string
		wantErr         bool
	}{
		{"empty both", vars{}, "", "", true},
		{"empty path", vars{"fileName": "foo"}, "foo", "", false},
		{"empty name", vars{"checkFolder": "foo"}, "", "", true},
		{"empty none", vars{"fileName": "foo", "checkFolder": "bar"}, "foo", "bar", false},
		{"invalid name 1", vars{"fileName": "..", "checkFolder": "bar"}, "", "", true},
		{"invalid name 2", vars{"fileName": "/foo", "checkFolder": "bar"}, "", "", true},
		{"invalid path 1", vars{"fileName": "foo", "checkFolder": ".."}, "", "", true},
		{"invalid path 2", vars{"fileName": "foo", "checkFolder": "..\\.."}, "", "", true},
		{"invalid path 3", vars{"fileName": "foo", "checkFolder": "foo\\.."}, "", "", true},
		{"invalid path 4", vars{"fileName": "foo", "checkFolder": "../.."}, "", "", true},
		{"invalid path 5", vars{"fileName": "foo", "checkFolder": "foo/.."}, "", "", true},
		{"invalid both", vars{"fileName": "/foo", "checkFolder": ".."}, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.vars {
				req.SetPathValue(k, v)
			}
			gotFileName, gotCheckFolder, err := getFileNameAndFolder(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("getFileNameAndFolder() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotFileName != tt.wantFileName {
				t.Errorf("getFileNameAndFolder() gotFileName = %v, want %v", gotFileName, tt.wantFileName)
			}
			if gotCheckFolder != tt.wantCheckFolder {
				t.Errorf("getFileNameAndFolder() gotCheckFolder = %v, want %v", gotCheckFolder, tt.wantCheckFolder)
			}
		})
	}
}

func testdataDir(t *testing.T) string {
	t.Helper()

	const localTestdata = "testdata"
	if entries, err := os.ReadDir(localTestdata); err == nil && len(entries) > 0 {
		return localTestdata
	}

	path, err := runfiles.Rlocation(filepath.Join(
		runfiles.CallerRepository(),
		"comp/core/gui/impl/testdata/check.yaml",
	))
	require.NoError(t, err)

	return filepath.Dir(path)
}
