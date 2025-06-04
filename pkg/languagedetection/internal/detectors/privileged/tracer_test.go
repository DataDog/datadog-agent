// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package privileged

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

func TestTracerResult(t *testing.T) {
	pid := os.Getpid()
	p := &proc{pid: int32(pid)}

	detector := NewTracerDetector()
	_, err := detector.DetectLanguage(p)
	require.Error(t, err)

	tests := []struct {
		tdata string
		out   languagemodels.LanguageName
		err   bool
	}{
		{
			tdata: "tracer_cpp.data",
			out:   languagemodels.CPP,
		},
		{
			tdata: "tracer_python.data",
			out:   languagemodels.Python,
		},
		{
			tdata: "tracer_wrong.data",
			err:   true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.tdata, func(t *testing.T) {
			curDir, err := testutil.CurDir()
			require.NoError(t, err)
			testDataPath := filepath.Join(curDir, "testdata/tracer", testCase.tdata)
			data, err := os.ReadFile(testDataPath)
			require.NoError(t, err)
			createTracerMemfd(t, data)
			lang, err := detector.DetectLanguage(p)
			if testCase.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.out, lang.Name)
			}
		})
	}
}

func createTracerMemfd(t *testing.T, l []byte) {
	t.Helper()
	fd, err := memfile("datadog-tracer-info-xxx", l)
	t.Cleanup(func() { unix.Close(fd) })
	require.NoError(t, err)
}
