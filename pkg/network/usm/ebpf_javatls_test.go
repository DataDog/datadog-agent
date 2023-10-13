// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	javatestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/java/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
)

func createJavaTempFile(t *testing.T, dir string) string {
	tempfile, err := os.CreateTemp(dir, "TestAgentLoaded.agentmain.*")
	require.NoError(t, err)
	tempfile.Close()
	os.Remove(tempfile.Name())
	t.Cleanup(func() { os.Remove(tempfile.Name()) })

	return tempfile.Name()
}

func TestJavaInjection(t *testing.T) {
	cfg := networkconfig.New()
	cfg.EnableJavaTLSSupport = true
	if !http.HTTPSSupported(cfg) {
		t.Skip("Java injection tests are not supported on this machine")
	}

	defaultCfg := cfg

	dir, _ := testutil.CurDir()
	testdataDir := filepath.Join(dir, "../protocols/tls/java/testdata")
	// create a fake agent-usm.jar based on TestAgentLoaded.jar by forcing cfg.JavaDir
	fakeAgentDir, err := os.MkdirTemp("", "fake.agent-usm.jar.")
	require.NoError(t, err)
	defer os.RemoveAll(fakeAgentDir)
	_, err = nettestutil.RunCommand("install -m444 " + filepath.Join(testdataDir, "TestAgentLoaded.jar") + " " + filepath.Join(fakeAgentDir, "agent-usm.jar"))
	require.NoError(t, err)

	commonTearDown := func(t *testing.T, ctx map[string]interface{}) {
		cfg.JavaAgentArgs = ctx["JavaAgentArgs"].(string)

		testfile := ctx["testfile"].(string)
		_, err := os.Stat(testfile)
		if err == nil {
			os.Remove(testfile)
		}
	}

	commonValidation := func(t *testing.T, ctx map[string]interface{}) {
		testfile := ctx["testfile"].(string)
		_, err := os.Stat(testfile)
		require.NoError(t, err)
	}

	tests := []struct {
		name            string
		context         map[string]interface{}
		preTracerSetup  func(t *testing.T, ctx map[string]interface{})
		postTracerSetup func(t *testing.T, ctx map[string]interface{})
		validation      func(t *testing.T, ctx map[string]interface{})
		teardown        func(t *testing.T, ctx map[string]interface{})
	}{
		{
			// Test the java hotspot injection is working
			name:    "java_hotspot_injection_8u151",
			context: make(map[string]interface{}),
			preTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				cfg.JavaDir = fakeAgentDir
				ctx["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx["testfile"].(string))
			},
			postTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				// if RunJavaVersion failing to start it's probably because the java process has not been injected
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:8u151-jre", "Wait JustWait"), "Failed running Java version")
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			// Test the java hotspot injection is working
			name:    "java_hotspot_injection_21_allow_only",
			context: make(map[string]interface{}),
			preTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				cfg.JavaDir = fakeAgentDir
				ctx["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx["testfile"].(string))

				// testing allow/block list, as Allow list have higher priority
				// this test will pass normally
				cfg.JavaAgentAllowRegex = ".*JustWait.*"
				cfg.JavaAgentBlockRegex = ""
			},
			postTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				// if RunJavaVersion failing to start it's probably because the java process has not been injected
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "Wait JustWait"), "Failed running Java version")
				javatestutil.RunJavaVersionAndWaitForRejection(t, "openjdk:21-oraclelinux8", "Wait AnotherWait", regexp.MustCompile(`AnotherWait pid.*`))
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			// Test the java hotspot injection is working
			name:    "java_hotspot_injection_21_block_only",
			context: make(map[string]interface{}),
			preTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				ctx["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx["testfile"].(string))

				// block the agent attachment
				cfg.JavaAgentAllowRegex = ""
				cfg.JavaAgentBlockRegex = ".*JustWait.*"
			},
			postTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				// if RunJavaVersion failing to start it's probably because the java process has not been injected
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "Wait AnotherWait"), "Failed running Java version")
				javatestutil.RunJavaVersionAndWaitForRejection(t, "openjdk:21-oraclelinux8", "Wait JustWait", regexp.MustCompile(`JustWait pid.*`))
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			name:    "java_hotspot_injection_21_allowblock",
			context: make(map[string]interface{}),
			preTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				ctx["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx["testfile"].(string))

				// block the agent attachment
				cfg.JavaAgentAllowRegex = ".*JustWait.*"
				cfg.JavaAgentBlockRegex = ".*AnotherWait.*"
			},
			postTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "Wait JustWait"), "Failed running Java version")
				javatestutil.RunJavaVersionAndWaitForRejection(t, "openjdk:21-oraclelinux8", "Wait AnotherWait", regexp.MustCompile(`AnotherWait pid.*`))
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			name:    "java_hotspot_injection_21_allow_higher_priority",
			context: make(map[string]interface{}),
			preTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				ctx["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx["testfile"].(string))

				// allow has a higher priority
				cfg.JavaAgentAllowRegex = ".*JustWait.*"
				cfg.JavaAgentBlockRegex = ".*JustWait.*"
			},
			postTracerSetup: func(t *testing.T, ctx map[string]interface{}) {
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "Wait JustWait"), "Failed running Java version")
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				tt.teardown(t, tt.context)
			})
			cfg = defaultCfg
			tt.preTracerSetup(t, tt.context)
			javaTLSProg, err := newJavaTLSProgram(cfg)
			require.NoError(t, err)
			require.NoError(t, javaTLSProg.PreStart(nil))
			t.Cleanup(func() {
				javaTLSProg.Stop(nil)
			})
			require.NoError(t, javaTLSProg.(*javaTLSProgram).processMonitor.Initialize())

			tt.postTracerSetup(t, tt.context)
			tt.validation(t, tt.context)
		})
	}
}
