// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	seelogCfg "github.com/DataDog/datadog-agent/pkg/config/logs/internal/seelog"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func TestExtractShortPathFromFullPath(t *testing.T) {
	// omnibus path
	assert.Equal(t, "pkg/collector/scheduler.go", extractShortPathFromFullPath("/go/src/github.com/DataDog/datadog-agent/.omnibus/src/datadog-agent/src/github.com/DataDog/datadog-agent/pkg/collector/scheduler.go"))
	// dev env path
	assert.Equal(t, "cmd/agent/app/start.go", extractShortPathFromFullPath("/home/vagrant/go/src/github.com/DataDog/datadog-agent/cmd/agent/app/start.go"))
	// relative path
	assert.Equal(t, "pkg/collector/scheduler.go", extractShortPathFromFullPath("pkg/collector/scheduler.go"))
	// no path
	assert.Equal(t, "main.go", extractShortPathFromFullPath("main.go"))
	// process agent
	assert.Equal(t, "cmd/agent/collector.go", extractShortPathFromFullPath("/home/jenkins/workspace/process-agent-build-ddagent/go/src/github.com/DataDog/datadog-process-agent/cmd/agent/collector.go"))
	// various possible dependency paths
	assert.Equal(t, "collector@v0.35.0/receiver/otlpreceiver/otlp.go", extractShortPathFromFullPath("/Users/runner/programming/go/pkg/mod/go.opentelemetry.io/collector@v0.35.0/receiver/otlpreceiver/otlp.go"))
	assert.Equal(t, "collector@v0.35.0/receiver/otlpreceiver/otlp.go", extractShortPathFromFullPath("/modcache/go.opentelemetry.io/collector@v0.35.0/receiver/otlpreceiver/otlp.go"))
}

func TestSeelogConfig(t *testing.T) {
	cfg := seelogCfg.NewSeelogConfig("TEST", "off", "common", "", "", false)
	cfg.EnableConsoleLog(true)
	cfg.EnableFileLogging("/dev/null", 123, 456)

	seelogConfigStr, err := cfg.Render()
	assert.Nil(t, err)

	logger, err := seelog.LoggerFromConfigAsString(seelogConfigStr)
	assert.Nil(t, err)
	assert.NotNil(t, logger)
}

func benchmarkLogFormat(logFormat string, b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, logFormat)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		buff.Reset()
		l.Infof("Hello I am a log")
	}
}

func BenchmarkLogFormatFilename(b *testing.B) {
	benchmarkLogFormat("%Date(%s) | %LEVEL | (%File:%Line in %FuncShort) | %Msg", b)
}

func BenchmarkLogFormatShortFilePath(b *testing.B) {
	benchmarkLogFormat("%Date(%s) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg", b)
}

func TestExtractContextString(t *testing.T) {
	assert.Equal(t, `,"foo":"bar"`, extractContextString(jsonFormat, []interface{}{"foo", "bar"}))
	assert.Equal(t, `foo:bar | `, extractContextString(textFormat, []interface{}{"foo", "bar"}))
	assert.Equal(t, `,"foo":"bar","bar":"buzz"`, extractContextString(jsonFormat, []interface{}{"foo", "bar", "bar", "buzz"}))
	assert.Equal(t, `foo:bar,bar:buzz | `, extractContextString(textFormat, []interface{}{"foo", "bar", "bar", "buzz"}))
	assert.Equal(t, `,"foo":"b\"a\"r"`, extractContextString(jsonFormat, []interface{}{"foo", "b\"a\"r"}))
	assert.Equal(t, `,"foo":"3"`, extractContextString(jsonFormat, []interface{}{"foo", 3}))
	assert.Equal(t, `,"foo":"4.131313131"`, extractContextString(jsonFormat, []interface{}{"foo", float64(4.131313131)}))
	assert.Equal(t, "", extractContextString(jsonFormat, nil))
	assert.Equal(t, ",", extractContextString(jsonFormat, []interface{}{2, 3}))
	assert.Equal(t, `,"foo":"bar","bar":"buzz"`, extractContextString(jsonFormat, []interface{}{"foo", "bar", 2, 3, "bar", "buzz"}))
}

func benchmarkLogFormatWithContext(logFormat string, b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, logFormat)
	context := []interface{}{"extra", "context", "foo", "bar"}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		buff.Reset()
		l.SetContext(context)
		l.Infof("Hello I am a log")
		l.SetContext(nil)
	}
}

func BenchmarkLogFormatWithoutContextFormatting(b *testing.B) {
	benchmarkLogFormatWithContext("%Date(%s) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg", b)
}

func BenchmarkLogFormatWithContextFormatting(b *testing.B) {
	benchmarkLogFormatWithContext("%Date(%s) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg %ExtraJSONContext", b)
}

func TestMergedKeys(t *testing.T) {
	// Test that the merged keys are correctly computed
	s1 := []string{"foo", "bar"}
	s2 := []string{"bar", "buzz"}
	assert.Equal(t, []string{"foo", "bar", "buzz"}, mergeAdditionalKeysToScrubber(s1, s2))
}

func TestENVAdditionalKeysToScrubber(t *testing.T) {
	// Test that the scrubber is correctly configured with the expected keys
	cfg := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	pkgconfigsetup.InitConfig(cfg)

	cfg.SetWithoutSource("scrubber.additional_keys", []string{"yet_another_key"})
	cfg.SetWithoutSource("flare_stripped_keys", []string{"some_other_key"})

	pathDir := t.TempDir()

	// Add a log file at the end of pathDir string
	pathDir = pathDir + "/tests.log"

	SetupLogger(
		"TestENVAdditionalKeysToScrubberLogger",
		"info",
		pathDir,
		"",
		false,
		false,
		false,
		cfg)

	stringToScrub := `api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
some_other_key: 'bbbb'
app_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaacccc'
yet_another_key: 'dddd'`

	scrubbed, err := scrubber.ScrubYamlString(stringToScrub)
	assert.Nil(t, err)
	expected := `api_key: '***************************aaaaa'
some_other_key: "********"
app_key: '***********************************acccc'
yet_another_key: "********"`
	require.YAMLEq(t, expected, scrubbed)
}
