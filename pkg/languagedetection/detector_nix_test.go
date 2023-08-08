// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package languagedetection

import (
	"google.golang.org/protobuf/proto"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	languagepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/languagedetection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeProcess(cmdline []string) *procutil.Process {
	return &procutil.Process{
		Pid:     rand.Int31(),
		Cmdline: cmdline,
	}
}

func TestLanguageFromCommandline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		expected languagemodels.LanguageName
	}{
		{
			name:     "python2",
			cmdline:  []string{"/opt/Python/2.7.11/bin/python2.7", "/opt/foo/bar/baz", "--config=asdf"},
			expected: languagemodels.Python,
		},
		{
			name:     "Java",
			cmdline:  []string{"/usr/bin/Java", "-Xfoo=true", "org.elasticsearch.bootstrap.Elasticsearch"},
			expected: languagemodels.Java,
		},
		{
			name:     "Unknown",
			cmdline:  []string{"mine-bitcoins", "--all"},
			expected: languagemodels.Unknown,
		},
		{
			name:     "Python with space and special chars in path",
			cmdline:  []string{"//..//path/\"\\ to/Python", "asdf"},
			expected: languagemodels.Python,
		},
		{
			name:     "args in first element",
			cmdline:  []string{"/usr/bin/Python myapp.py --config=/etc/mycfg.yaml"},
			expected: languagemodels.Python,
		},
		{
			name:     "javac is not Java",
			cmdline:  []string{"javac", "main.Java"},
			expected: languagemodels.Unknown,
		},
		{
			name:     "py is Python",
			cmdline:  []string{"py", "test.py"},
			expected: languagemodels.Python,
		},
		{
			name:     "py is not a prefix",
			cmdline:  []string{"pyret", "main.pyret"},
			expected: languagemodels.Unknown,
		},
		{
			name:     "node",
			cmdline:  []string{"node", "/etc/app/index.js"},
			expected: languagemodels.Node,
		},
		{
			name:     "npm",
			cmdline:  []string{"npm", "start"},
			expected: languagemodels.Node,
		},
		{
			name:     "dotnet",
			cmdline:  []string{"dotnet", "myApp"},
			expected: languagemodels.Dotnet,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, languageNameFromCommandLine(tc.cmdline))
		})
	}
}

func TestGetExe(t *testing.T) {
	type test struct {
		name     string
		cmdline  []string
		expected string
	}

	for _, tc := range []test{
		{
			name:     "blank",
			cmdline:  []string{},
			expected: "",
		},
		{
			name:     "python",
			cmdline:  []string{"/usr/bin/python", "test.py"},
			expected: "python",
		},
		{
			name:     "numeric ending",
			cmdline:  []string{"/usr/bin/python3.9", "test.py"},
			expected: "python3.9",
		},
		{
			name:     "packed args",
			cmdline:  []string{"java -jar Test.jar"},
			expected: "java",
		},
		{
			name:     "uppercase",
			cmdline:  []string{"/usr/bin/MyBinary"},
			expected: "mybinary",
		},
		{
			name:     "dont trim .exe on linux",
			cmdline:  []string{"/usr/bin/helloWorld.exe"},
			expected: "helloworld.exe",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, getExe(tc.cmdline))
		})
	}
}

func BenchmarkDetectLanguage(b *testing.B) {
	commands := [][]string{
		{"Python", "--version"},
		{"python3", "--version"},
		{"py", "--version"},
		{"Python", "-c", "import platform; print(platform.python_version())"},
		{"python3", "-c", "import platform; print(platform.python_version())"},
		{"py", "-c", "import platform; print(platform.python_version())"},
		{"Python", "-c", "import sys; print(sys.version)"},
		{"python3", "-c", "import sys; print(sys.version)"},
		{"py", "-c", "import sys; print(sys.version)"},
		{"Python", "-c", "print('Python')"},
		{"python3", "-c", "print('Python')"},
		{"py", "-c", "print('Python')"},
		{"Java", "-version"},
		{"Java", "-jar", "myapp.jar"},
		{"Java", "-cp", ".", "MyClass"},
		{"javac", "MyClass.Java"},
		{"javap", "-c", "MyClass"},
	}

	var procs []*procutil.Process
	for _, command := range commands {
		procs = append(procs, makeProcess(command))
	}

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		DetectLanguage(procs, nil)
	}
}

func startTestUnixServer(t *testing.T, handler http.Handler) string {
	t.Helper()

	socketPath := path.Join(t.TempDir(), "test.sock")
	listener, err := net.NewListener(socketPath)
	require.NoError(t, err)
	t.Cleanup(listener.Stop)

	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = listener.GetListener()
	srv.Start()
	t.Cleanup(srv.Close)

	return socketPath
}

func TestBinaryAnalysisClient(t *testing.T) {
	socketPath := startTestUnixServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		b, err := proto.Marshal(&languagepb.DetectLanguageResponse{
			Languages: []*languagepb.Language{
				{
					Name:    string(languagemodels.Go),
					Version: "1.19",
				},
				{
					Name:    string(languagemodels.Go),
					Version: "1.20",
				},
				{
					Name:    string(languagemodels.Go),
					Version: "1.13",
				},
			},
		})
		require.NoError(t, err)

		_, err = writer.Write(b)
		require.NoError(t, err)
	}))

	var procs []*procutil.Process
	for _, command := range [][]string{
		{"python3", "--version"},
		{"go", "run", "main.go"},
		{"java", "-c", "org.elasticsearch.bootstrap.Elasticsearch"},
		{"process-agent"},
		{"my-internal-go-service", "-p", "8080"},
		{"xonotic"},
	} {
		procs = append(procs, makeProcess(command))
	}

	cfg := config.Mock(t)
	cfg.Set("system_probe_config.language_detection.enabled", true)
	cfg.Set("system_probe_config.sysprobe_socket", socketPath)

	res := DetectLanguage(procs, cfg)
	assert.Equal(t, languagemodels.Python, res[0].Name)

	assert.Equal(t, languagemodels.Go, res[1].Name)
	assert.Equal(t, "1.19", res[1].Version)

	assert.Equal(t, languagemodels.Java, res[2].Name)

	assert.Equal(t, languagemodels.Go, res[3].Name)
	assert.Equal(t, "1.20", res[3].Version)

	assert.Equal(t, languagemodels.Go, res[4].Name)
	assert.Equal(t, "1.13", res[4].Version)

	assert.Equal(t, languagemodels.Unknown, res[5].Name)
}
