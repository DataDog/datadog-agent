// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/preprocessor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/dockerfile"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/dockerstream"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/encodedtext"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func InitializeDecoderForTest(source *sources.LogSource, parser parsers.Parser) Decoder {
	info := status.NewInfoRegistry()
	return InitializeDecoder(sources.NewReplaceableSource(source), parser, info)
}

func TestDecoderWithDockerHeader(t *testing.T) {
	source := sources.NewLogSource("config", &config.LogsConfig{})
	d := InitializeDecoderForTest(source, noop.New())
	d.Start()

	input := []byte("hello\n")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	d.InputChan() <- NewInput(input)

	var output *message.Message
	output = <-d.OutputChan()
	assert.Equal(t, "hello", string(output.GetContent()))
	assert.Equal(t, len("hello")+1, output.RawDataLen)

	output = <-d.OutputChan()
	expected := []byte{1, 0, 0, 0, 0}
	assert.Equal(t, expected, output.GetContent())
	assert.Equal(t, 6, output.RawDataLen)

	output = <-d.OutputChan()
	expected = append([]byte{0, 0}, []byte("2018-06-14T18:27:03.246999277Z app logs")...)
	assert.Equal(t, expected, output.GetContent())
	assert.Equal(t, len(expected)+1, output.RawDataLen)

	d.Stop()
}

func TestDecoderWithDockerHeaderSingleline(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	d := InitializeDecoderForTest(sources.NewLogSource("", &config.LogsConfig{}), dockerstream.New("abc123"))
	d.Start()
	defer d.Stop()

	line = append([]byte{2, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852911Z message\n")...)
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.ParsingExtra.Timestamp)

	line = []byte("wrong message\n")
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "wrong", output.ParsingExtra.Timestamp)

}

func TestDecoderWithDockerHeaderMultiline(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	c := &config.LogsConfig{
		ProcessingRules: []*config.ProcessingRule{
			{
				Type:  config.MultiLine,
				Regex: regexp.MustCompile("1234"),
			},
		},
	}

	d := InitializeDecoderForTest(sources.NewLogSource("", c), dockerstream.New("abc123"))
	d.Start()
	defer d.Stop()

	line = append([]byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852911Z 1234 hello\n")...)
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	line = append([]byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852912Z world\n")...)
	lineLen += len(line)
	d.InputChan() <- NewInput(line)

	line = append([]byte{2, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852913Z 1234 bye\n")...)
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("1234 hello\\nworld"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.ParsingExtra.Timestamp)

	lineLen = len(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("1234 bye"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.ParsingExtra.Timestamp)
}

func TestDecoderWithDockerJSONSingleline(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	d := InitializeDecoderForTest(sources.NewLogSource("", &config.LogsConfig{}), dockerfile.New())
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"message\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.ParsingExtra.Timestamp)

	line = []byte("wrong message\n")
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("wrong message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "", output.ParsingExtra.Timestamp)
}

func TestDecoderWithDockerJSONMultiline(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	c := &config.LogsConfig{
		ProcessingRules: []*config.ProcessingRule{
			{
				Type:  config.MultiLine,
				Regex: regexp.MustCompile("1234"),
			},
		},
	}

	d := InitializeDecoderForTest(sources.NewLogSource("", c), dockerfile.New())
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"1234 hello\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	line = []byte(`{"log":"world\n","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}` + "\n")
	lineLen += len(line)
	d.InputChan() <- NewInput(line)

	line = []byte(`{"log":"1234 bye\n","stream":"stderr","time":"2019-06-06T16:35:55.930852913Z"}` + "\n")
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("1234 hello\\nworld"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.ParsingExtra.Timestamp)

	lineLen = len(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("1234 bye"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.ParsingExtra.Timestamp)
}

func TestDecoderWithDockerJSONSplittedByDocker(t *testing.T) {
	var output *message.Message
	var line []byte

	d := InitializeDecoderForTest(sources.NewLogSource("", &config.LogsConfig{}), dockerfile.New())
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"part1","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	rawLen := len(line)
	d.InputChan() <- NewInput(line)

	line = []byte(`{"log":"part2\n","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}` + "\n")
	rawLen += len(line)
	d.InputChan() <- NewInput(line)

	// We don't reaggregate partial messages but we expect content of line not finishing with a '\n' character to be reconciliated
	// with the next line.
	output = <-d.OutputChan()
	assert.Equal(t, []byte("part1part2"), output.GetContent())
	assert.Equal(t, rawLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.ParsingExtra.Timestamp)
}

func TestDecoderWithDecodingParser(t *testing.T) {
	source := sources.NewLogSource("config", &config.LogsConfig{})

	info := status.NewInfoRegistry()
	d := NewDecoderWithFraming(sources.NewReplaceableSource(source), encodedtext.New(encodedtext.UTF16LE), framer.UTF16LENewline, nil, info)
	d.Start()

	input := []byte{'h', 0x0, 'e', 0x0, 'l', 0x0, 'l', 0x0, 'o', 0x0, '\n', 0x0}
	d.InputChan() <- NewInput(input)

	var output *message.Message
	output = <-d.OutputChan()
	assert.Equal(t, "hello", string(output.GetContent()))
	assert.Equal(t, len(input), output.RawDataLen)

	// Test with BOM
	input = []byte{0xFF, 0xFE, 'h', 0x0, 'e', 0x0, 'l', 0x0, 'l', 0x0, 'o', 0x0, '\n', 0x0}
	d.InputChan() <- NewInput(input)

	output = <-d.OutputChan()
	assert.Equal(t, "hello", string(output.GetContent()))
	assert.Equal(t, len(input), output.RawDataLen)

	d.Stop()
}

func TestDecoderWithSinglelineKubernetes(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	d := InitializeDecoderForTest(sources.NewLogSource("", &config.LogsConfig{}), kubernetes.New())
	d.Start()
	defer d.Stop()

	line = []byte("2019-06-06T16:35:55.930852911Z stderr F message\n")
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.ParsingExtra.Timestamp)

	line = []byte("wrong message\n")
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("wrong message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "", output.ParsingExtra.Timestamp)
}

func TestDecoderWithMultilineKubernetes(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	c := &config.LogsConfig{
		ProcessingRules: []*config.ProcessingRule{
			{
				Type:  config.MultiLine,
				Regex: regexp.MustCompile("1234"),
			},
		},
	}
	d := InitializeDecoderForTest(sources.NewLogSource("", c), kubernetes.New())
	d.Start()
	defer d.Stop()

	line = []byte("2019-06-06T16:35:55.930852911Z stdout F 1234 hello\n")
	lineLen = len(line)
	d.InputChan() <- NewInput(line)

	line = []byte("2019-06-06T16:35:55.930852912Z stdout F world\n")
	lineLen += len(line)
	d.InputChan() <- NewInput(line)

	line = []byte("2019-06-06T16:35:55.930852913Z stderr F 1234 bye\n")
	d.InputChan() <- NewInput(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("1234 hello\\nworld"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.ParsingExtra.Timestamp)

	lineLen = len(line)

	output = <-d.OutputChan()
	assert.Equal(t, []byte("1234 bye"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.ParsingExtra.Timestamp)
}

func TestResolveTokenizerAndLabelerMaxInputBytes(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.auto_multi_line.tokenizer_max_input_bytes", 60, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.experimental_adaptive_sampling.tokenizer_max_input_bytes", 256, pkgconfigmodel.SourceAgentRuntime)

	sourceOverride20 := 20
	sourceOverride500 := 500
	sourceSamplerTokenizerOverride128 := 128
	enabledTrue := true
	enabledFalse := false

	tests := []struct {
		name                  string
		globalSamplerEnabled  bool
		globalNoisyDetection  bool
		sourceAutoMLSettings  *config.SourceAutoMultiLineOptions
		sourceSamplerSettings *config.SourceAdaptiveSamplingOptions
		sourceNoisyDetection  *bool
		wantTokenizerMax      int
		wantLabelerMax        int
	}{
		{
			name:                 "global defaults no sampler",
			globalSamplerEnabled: false,
			sourceAutoMLSettings: nil,
			wantTokenizerMax:     60,
			wantLabelerMax:       60,
		},
		{
			name:                 "source override no sampler",
			globalSamplerEnabled: false,
			sourceAutoMLSettings: &config.SourceAutoMultiLineOptions{
				TokenizerMaxInputBytes: &sourceOverride20,
			},
			wantTokenizerMax: 20,
			wantLabelerMax:   20,
		},
		{
			name:                 "global sampler widens tokenizer from global",
			globalSamplerEnabled: true,
			sourceAutoMLSettings: nil,
			wantTokenizerMax:     256,
			wantLabelerMax:       60,
		},
		{
			name:                 "global sampler widens tokenizer while keeping source labeler limit",
			globalSamplerEnabled: true,
			sourceAutoMLSettings: &config.SourceAutoMultiLineOptions{
				TokenizerMaxInputBytes: &sourceOverride20,
			},
			wantTokenizerMax: 256,
			wantLabelerMax:   20,
		},
		{
			name:                 "source labeler override larger than sampler minimum",
			globalSamplerEnabled: true,
			sourceAutoMLSettings: &config.SourceAutoMultiLineOptions{
				TokenizerMaxInputBytes: &sourceOverride500,
			},
			wantTokenizerMax: 500,
			wantLabelerMax:   500,
		},
		{
			name:                 "source sampler enable overrides global false",
			globalSamplerEnabled: false,
			sourceSamplerSettings: &config.SourceAdaptiveSamplingOptions{
				Enabled: &enabledTrue,
			},
			wantTokenizerMax: 256,
			wantLabelerMax:   60,
		},
		{
			name:                 "source sampler disable overrides global true",
			globalSamplerEnabled: true,
			sourceSamplerSettings: &config.SourceAdaptiveSamplingOptions{
				Enabled: &enabledFalse,
			},
			wantTokenizerMax: 60,
			wantLabelerMax:   60,
		},
		{
			name:                 "source sampler tokenizer override wins over global sampler tokenizer",
			globalSamplerEnabled: true,
			sourceSamplerSettings: &config.SourceAdaptiveSamplingOptions{
				TokenizerMaxInputBytes: &sourceSamplerTokenizerOverride128,
			},
			wantTokenizerMax: 128,
			wantLabelerMax:   60,
		},
		{
			name:                 "global noisy detection widens tokenizer when sampler disabled",
			globalNoisyDetection: true,
			wantTokenizerMax:     256,
			wantLabelerMax:       60,
		},
		{
			name:                 "source noisy detection enable widens tokenizer when sampler disabled",
			sourceNoisyDetection: &enabledTrue,
			wantTokenizerMax:     256,
			wantLabelerMax:       60,
		},
		{
			name:                 "source noisy detection disable does not widen tokenizer",
			sourceNoisyDetection: &enabledFalse,
			wantTokenizerMax:     60,
			wantLabelerMax:       60,
		},
		{
			name:                 "source noisy detection disable overrides global noisy detection",
			globalNoisyDetection: true,
			sourceNoisyDetection: &enabledFalse,
			wantTokenizerMax:     60,
			wantLabelerMax:       60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig.Set("logs_config.experimental_adaptive_sampling.enabled", tt.globalSamplerEnabled, pkgconfigmodel.SourceAgentRuntime)
			mockConfig.Set("logs_config.experimental_noisy_log_detection", tt.globalNoisyDetection, pkgconfigmodel.SourceAgentRuntime)
			gotTokenizerMax, gotLabelerMax := resolveTokenizerAndLabelerMaxInputBytes(tt.sourceAutoMLSettings, tt.sourceSamplerSettings, tt.sourceNoisyDetection)
			assert.Equal(t, tt.wantTokenizerMax, gotTokenizerMax)
			assert.Equal(t, tt.wantLabelerMax, gotLabelerMax)
		})
	}
}

func TestResolveAdaptiveSamplerEnabled(t *testing.T) {
	mockConfig := configmock.New(t)
	enabledTrue := true
	enabledFalse := false

	tests := []struct {
		name          string
		globalEnabled bool
		sourceCfg     *config.SourceAdaptiveSamplingOptions
		want          bool
	}{
		{
			name:          "falls back to global when source unset",
			globalEnabled: true,
			sourceCfg:     nil,
			want:          true,
		},
		{
			name:          "source disable overrides global enable",
			globalEnabled: true,
			sourceCfg: &config.SourceAdaptiveSamplingOptions{
				Enabled: &enabledFalse,
			},
			want: false,
		},
		{
			name:          "source enable overrides global disable",
			globalEnabled: false,
			sourceCfg: &config.SourceAdaptiveSamplingOptions{
				Enabled: &enabledTrue,
			},
			want: true,
		},
		{
			name:          "source block without enabled still falls back to global",
			globalEnabled: false,
			sourceCfg:     &config.SourceAdaptiveSamplingOptions{},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig.Set("logs_config.experimental_adaptive_sampling.enabled", tt.globalEnabled, pkgconfigmodel.SourceAgentRuntime)
			got := resolveAdaptiveSamplerEnabled(tt.sourceCfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveNoisyLogDetectionEnabled(t *testing.T) {
	mockConfig := configmock.New(t)
	enabledTrue := true
	enabledFalse := false

	tests := []struct {
		name          string
		globalEnabled bool
		sourceCfg     *bool
		want          bool
	}{
		{
			name:          "falls back to global when source unset",
			globalEnabled: true,
			sourceCfg:     nil,
			want:          true,
		},
		{
			name:          "source disable overrides global enable",
			globalEnabled: true,
			sourceCfg:     &enabledFalse,
			want:          false,
		},
		{
			name:          "source enable overrides global disable",
			globalEnabled: false,
			sourceCfg:     &enabledTrue,
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig.Set("logs_config.experimental_noisy_log_detection", tt.globalEnabled, pkgconfigmodel.SourceAgentRuntime)
			got := resolveNoisyLogDetectionEnabled(tt.sourceCfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveSamplerMode(t *testing.T) {
	mockConfig := configmock.New(t)
	enabledTrue := true
	enabledFalse := false

	tests := []struct {
		name                  string
		globalSamplerEnabled  bool
		globalNoisyDetection  bool
		sourceSamplerSettings *config.SourceAdaptiveSamplingOptions
		sourceNoisyDetection  *bool
		want                  samplerMode
	}{
		{
			name:                 "disabled when both features disabled",
			globalSamplerEnabled: false,
			globalNoisyDetection: false,
			want:                 samplerDisabled,
		},
		{
			name:                 "adaptive sampling wins when both globals enabled",
			globalSamplerEnabled: true,
			globalNoisyDetection: true,
			want:                 samplerAdaptiveSampling,
		},
		{
			name:                 "noisy detection runs when sampler disabled",
			globalSamplerEnabled: false,
			globalNoisyDetection: true,
			want:                 samplerNoisyLogDetection,
		},
		{
			name:                 "source adaptive sampling wins over source noisy detection",
			globalSamplerEnabled: false,
			sourceSamplerSettings: &config.SourceAdaptiveSamplingOptions{
				Enabled: &enabledTrue,
			},
			sourceNoisyDetection: &enabledTrue,
			want:                 samplerAdaptiveSampling,
		},
		{
			name:                 "source adaptive disable allows source noisy detection",
			globalSamplerEnabled: true,
			sourceSamplerSettings: &config.SourceAdaptiveSamplingOptions{
				Enabled: &enabledFalse,
			},
			sourceNoisyDetection: &enabledTrue,
			want:                 samplerNoisyLogDetection,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig.Set("logs_config.experimental_adaptive_sampling.enabled", tt.globalSamplerEnabled, pkgconfigmodel.SourceAgentRuntime)
			mockConfig.Set("logs_config.experimental_noisy_log_detection", tt.globalNoisyDetection, pkgconfigmodel.SourceAgentRuntime)
			got := resolveSamplerMode(tt.sourceSamplerSettings, tt.sourceNoisyDetection)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveAdaptiveSamplerConfig(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.experimental_adaptive_sampling.max_patterns", 100, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.experimental_adaptive_sampling.rate_limit", 2.5, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.experimental_adaptive_sampling.burst_size", 50.0, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.experimental_adaptive_sampling.match_threshold", 0.8, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.experimental_adaptive_sampling.protect_important_logs", true, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.experimental_adaptive_sampling.tag_pattern_hash", false, pkgconfigmodel.SourceAgentRuntime)

	t.Run("falls back to global config", func(t *testing.T) {
		got := resolveAdaptiveSamplerConfig(nil, preprocessor.NewTokenizer(0))
		assert.Equal(t, 100, got.MaxPatterns)
		assert.Equal(t, 2.5, got.RateLimit)
		assert.Equal(t, 50.0, got.BurstSize)
		assert.Equal(t, 0.8, got.MatchThreshold)
		assert.True(t, got.ProtectImportantLogs)
		assert.False(t, got.DetectionOnly)
		assert.False(t, got.TagPatternHash)
	})

	t.Run("source overrides global config", func(t *testing.T) {
		maxPatterns := 200
		rateLimit := 3.5
		burstSize := 75.0
		matchThreshold := 0.65
		protectImportantLogs := false
		tagPatternHash := true

		got := resolveAdaptiveSamplerConfig(&config.SourceAdaptiveSamplingOptions{
			MaxPatterns:          &maxPatterns,
			RateLimit:            &rateLimit,
			BurstSize:            &burstSize,
			MatchThreshold:       &matchThreshold,
			ProtectImportantLogs: &protectImportantLogs,
			TagPatternHash:       &tagPatternHash,
		}, preprocessor.NewTokenizer(0))

		assert.Equal(t, 200, got.MaxPatterns)
		assert.Equal(t, 3.5, got.RateLimit)
		assert.Equal(t, 75.0, got.BurstSize)
		assert.Equal(t, 0.65, got.MatchThreshold)
		assert.False(t, got.ProtectImportantLogs)
		assert.False(t, got.DetectionOnly)
		assert.True(t, got.TagPatternHash)
	})

	t.Run("source partial override preserves global values", func(t *testing.T) {
		rateLimit := 9.5

		got := resolveAdaptiveSamplerConfig(&config.SourceAdaptiveSamplingOptions{
			RateLimit: &rateLimit,
		}, preprocessor.NewTokenizer(0))

		assert.Equal(t, 100, got.MaxPatterns)
		assert.Equal(t, 9.5, got.RateLimit)
		assert.Equal(t, 50.0, got.BurstSize)
		assert.Equal(t, 0.8, got.MatchThreshold)
		assert.True(t, got.ProtectImportantLogs)
		assert.False(t, got.DetectionOnly)
		assert.False(t, got.TagPatternHash)
	})

	t.Run("invalid global match threshold falls back to default", func(t *testing.T) {
		mockConfig.Set("logs_config.experimental_adaptive_sampling.match_threshold", -1.0, pkgconfigmodel.SourceAgentRuntime)
		defer mockConfig.Set("logs_config.experimental_adaptive_sampling.match_threshold", 0.8, pkgconfigmodel.SourceAgentRuntime)

		got := resolveAdaptiveSamplerConfig(nil, preprocessor.NewTokenizer(0))
		assert.Equal(t, defaultAdaptiveSamplerMatchThreshold, got.MatchThreshold)
	})

	t.Run("invalid source match threshold keeps global value", func(t *testing.T) {
		sourceInvalidThreshold := 1.5
		got := resolveAdaptiveSamplerConfig(&config.SourceAdaptiveSamplingOptions{
			MatchThreshold: &sourceInvalidThreshold,
		}, preprocessor.NewTokenizer(0))

		assert.Equal(t, 0.8, got.MatchThreshold)
	})

	t.Run("source filters are resolved", func(t *testing.T) {
		got := resolveAdaptiveSamplerConfig(&config.SourceAdaptiveSamplingOptions{
			Include: []*config.AdaptiveSamplingRule{
				{Regex: "foo.*bar"},
				{Sample: "my 123 fun log sample"},
			},
			Exclude: []*config.AdaptiveSamplingRule{
				{Regex: "baz.*qux"},
				{Sample: "my 456 bad log sample"},
			},
		}, preprocessor.NewTokenizer(0))

		assert.True(t, got.IncludeConfigured)
		require.Len(t, got.Include, 2)
		require.NotNil(t, got.Include[0].Regex)
		assert.Equal(t, "foo.*bar", got.Include[0].Regex.String())
		assert.NotEmpty(t, got.Include[1].SampleTokens)
		require.Len(t, got.Exclude, 2)
		require.NotNil(t, got.Exclude[0].Regex)
		assert.Equal(t, "baz.*qux", got.Exclude[0].Regex.String())
		assert.NotEmpty(t, got.Exclude[1].SampleTokens)
	})

	t.Run("global filters are resolved", func(t *testing.T) {
		mockConfig.Set("logs_config.experimental_adaptive_sampling.include", []map[string]interface{}{
			{"regex": "foo.*bar"},
		}, pkgconfigmodel.SourceAgentRuntime)
		mockConfig.Set("logs_config.experimental_adaptive_sampling.exclude", []map[string]interface{}{
			{"sample": "my 456 bad log sample"},
		}, pkgconfigmodel.SourceAgentRuntime)

		got := resolveAdaptiveSamplerConfig(nil, preprocessor.NewTokenizer(0))

		assert.True(t, got.IncludeConfigured)
		require.Len(t, got.Include, 1)
		require.NotNil(t, got.Include[0].Regex)
		assert.Equal(t, "foo.*bar", got.Include[0].Regex.String())
		require.Len(t, got.Exclude, 1)
		assert.NotEmpty(t, got.Exclude[0].SampleTokens)
	})

	t.Run("source filters override global filters", func(t *testing.T) {
		got := resolveAdaptiveSamplerConfig(&config.SourceAdaptiveSamplingOptions{
			Include: []*config.AdaptiveSamplingRule{
				{Regex: "source.*only"},
			},
			Exclude: []*config.AdaptiveSamplingRule{},
		}, preprocessor.NewTokenizer(0))

		assert.True(t, got.IncludeConfigured)
		require.Len(t, got.Include, 1)
		require.NotNil(t, got.Include[0].Regex)
		assert.Equal(t, "source.*only", got.Include[0].Regex.String())
		assert.Empty(t, got.Exclude)
	})

	t.Run("noisy log detection config forces detection-only mode", func(t *testing.T) {
		got := resolveNoisyLogDetectionConfig(nil, preprocessor.NewTokenizer(0))

		assert.Equal(t, 100, got.MaxPatterns)
		assert.Equal(t, 2.5, got.RateLimit)
		assert.Equal(t, 50.0, got.BurstSize)
		assert.Equal(t, 0.8, got.MatchThreshold)
		assert.True(t, got.DetectionOnly)
	})
}

func TestDecoderWithDockerJSONPartialLineDetectionOnlyMarksOversizedLogicalLineTruncated(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.max_message_size_bytes", 1000, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.tag_truncated_logs", true, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_multi_line_detection_tagging", true, pkgconfigmodel.SourceAgentRuntime)

	source := sources.NewLogSource("", &config.LogsConfig{})
	d := InitializeDecoderForTest(source, dockerfile.New())
	d.Start()
	defer d.Stop()

	part1 := strings.Repeat("a", 600)
	part2 := strings.Repeat("b", 600)

	line1 := []byte(fmt.Sprintf(`{"log":"%s","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`+"\n", part1))
	line2 := []byte(fmt.Sprintf(`{"log":"%s\n","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}`+"\n", part2))

	d.InputChan() <- NewInput(line1)
	d.InputChan() <- NewInput(line2)

	output := <-d.OutputChan()
	assert.Equal(t, part1+part2+string(message.TruncatedFlag), string(output.GetContent()))
	assert.True(t, output.ParsingExtra.IsTruncated)
	assert.Contains(t, output.ParsingExtra.Tags, message.TruncatedReasonTag("single_line"))
	assert.Equal(t, len(line1)+len(line2), output.RawDataLen)
}

// TestDecoderKubernetesGoStackTraceAggregatesBlankLine is a regression test for
// blank lines being silently dropped on the partial-line parser path used by
// container/CRI log sources (Kubernetes, Docker JSON file). The Go stack trace
// aggregator relies on the blank line that separates a panic header from its
// goroutine block; when the MultiLineParser dropped that blank line the parser
// state machine could not transition and abandoned aggregation, emitting the
// crash as many individual lines instead of one combined message.
func TestDecoderKubernetesGoStackTraceAggregatesBlankLine(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.auto_multi_line_detection", true, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_multi_line.stack_trace_parsers", []string{"go"}, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.tag_multi_line_logs", true, pkgconfigmodel.SourceAgentRuntime)

	source := sources.NewLogSource("", &config.LogsConfig{})
	d := InitializeDecoderForTest(source, kubernetes.New())
	d.Start()

	// A Go panic exactly as a container runtime would emit it in CRI format:
	// '<timestamp> <stream> <flag> <content>'. Note the blank line (empty
	// content) between the panic header and the goroutine block.
	criLine := func(content string) string {
		return "2024-01-01T00:00:00.000000000Z stderr F " + content + "\n"
	}
	panicLines := []string{
		"panic: something went wrong",
		"",
		"goroutine 1 [running]:",
		"main.plainPanic(...)",
		"\t/path/main.go:81",
		"main.main()",
		"\t/path/main.go:46 +0x5b4",
	}
	var input []byte
	for _, l := range panicLines {
		input = append(input, []byte(criLine(l))...)
	}
	d.InputChan() <- NewInput(input)

	// Closing the input flushes the stack trace aggregator, emitting the
	// combined crash as a single message.
	d.Stop()

	var outputs []*message.Message
	for output := range d.OutputChan() {
		outputs = append(outputs, output)
	}

	require.Len(t, outputs, 1, "expected the whole Go panic to aggregate into a single message")
	combined := outputs[0]
	assert.True(t, combined.ParsingExtra.IsMultiLine, "expected the combined crash to be tagged multi-line")
	assert.Contains(t, combined.ParsingExtra.Tags, message.MultiLineSourceTag("go_stack"))
	content := string(combined.GetContent())
	assert.Contains(t, content, "panic: something went wrong")
	assert.Contains(t, content, "goroutine 1 [running]:")
	assert.Contains(t, content, "main.main()")
}

// TestDecoderGoStackTraceDisabledWithEmptyParsers confirms that even with auto
// multi-line detection enabled, an empty logs_config.auto_multi_line.stack_trace_parsers
// override disables Go stack trace aggregation: the same panic that combines into
// a single go_stack-tagged message in TestDecoderKubernetesGoStackTraceAggregatesBlankLine
// is instead emitted line-by-line, with no message carrying the go_stack tag.
func TestDecoderGoStackTraceDisabledWithEmptyParsers(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.auto_multi_line_detection", true, pkgconfigmodel.SourceAgentRuntime)
	// Empty override: no stack trace parsers -> feature disabled.
	mockConfig.Set("logs_config.auto_multi_line.stack_trace_parsers", []string{}, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.tag_multi_line_logs", true, pkgconfigmodel.SourceAgentRuntime)

	source := sources.NewLogSource("", &config.LogsConfig{})
	d := InitializeDecoderForTest(source, kubernetes.New())
	d.Start()

	criLine := func(content string) string {
		return "2024-01-01T00:00:00.000000000Z stderr F " + content + "\n"
	}
	panicLines := []string{
		"panic: something went wrong",
		"",
		"goroutine 1 [running]:",
		"main.plainPanic(...)",
		"\t/path/main.go:81",
		"main.main()",
		"\t/path/main.go:46 +0x5b4",
	}
	var input []byte
	for _, l := range panicLines {
		input = append(input, []byte(criLine(l))...)
	}
	d.InputChan() <- NewInput(input)

	d.Stop()

	var outputs []*message.Message
	for output := range d.OutputChan() {
		outputs = append(outputs, output)
	}

	// With the feature disabled the panic is NOT combined: it comes through as
	// multiple individual messages, and none of them carry the go_stack tag.
	require.Greater(t, len(outputs), 1, "expected the panic to NOT be aggregated into a single message")
	for _, out := range outputs {
		assert.NotContains(t, out.ParsingExtra.Tags, message.MultiLineSourceTag("go_stack"),
			"no message should be tagged go_stack when stack_trace_parsers is empty")
	}
	// The panic header line is still present in the (un-aggregated) output.
	var sawPanicHeader bool
	for _, out := range outputs {
		if string(out.GetContent()) == "panic: something went wrong" {
			sawPanicHeader = true
		}
	}
	assert.True(t, sawPanicHeader, "expected the panic header to be emitted as its own message")
}

// TestGoStackTraceParsersEmptyListFromYAML confirms that the exact YAML string
// "[]" for logs_config.auto_multi_line.stack_trace_parsers overrides the ["go"]
// default with an empty slice (rather than being ignored and falling back to the
// default), which is what disables Go stack trace aggregation.
func TestGoStackTraceParsersEmptyListFromYAML(t *testing.T) {
	// Sanity check: with the key absent, the default is ["go"].
	defaultCfg := configmock.NewFromYAML(t, "logs_config:\n  auto_multi_line_detection: true\n")
	require.Equal(t, []string{"go"},
		defaultCfg.GetStringSlice("logs_config.auto_multi_line.stack_trace_parsers"),
		"expected the built-in default to be [\"go\"] when the key is not set")

	// The exact string "[]" in YAML must parse to an empty slice, overriding the default.
	yaml := "logs_config:\n" +
		"  auto_multi_line_detection: true\n" +
		"  auto_multi_line:\n" +
		"    stack_trace_parsers: []\n"
	cfg := configmock.NewFromYAML(t, yaml)
	got := cfg.GetStringSlice("logs_config.auto_multi_line.stack_trace_parsers")
	assert.Empty(t, got, "expected YAML \"[]\" to produce an empty slice, got %#v", got)

	// End-to-end: an empty slice yields the no-op aggregator (feature disabled).
	agg := preprocessor.NewStackTraceAggregatorFromNames(got, 1000, true)
	assert.True(t, agg.IsEmpty())
	out := agg.Process(message.NewMessage([]byte("panic: boom"), nil, "", 0))
	require.Len(t, out, 1, "no-op aggregator should pass the panic through unchanged")
	assert.NotContains(t, out[0].ParsingExtra.Tags, message.MultiLineSourceTag("go_stack"))
}

// TestGoStackTraceParsersFromEnvVar confirms that
// DD_LOGS_CONFIG_AUTO_MULTI_LINE_STACK_TRACE_PARSERS is the correct environment
// variable binding for logs_config.auto_multi_line.stack_trace_parsers, and that
// list values are provided space-separated.
func TestGoStackTraceParsersFromEnvVar(t *testing.T) {
	const key = "logs_config.auto_multi_line.stack_trace_parsers"

	// Multiple values: space-separated, as the agent parses env string slices.
	t.Setenv("DD_LOGS_CONFIG_AUTO_MULTI_LINE_STACK_TRACE_PARSERS", "go java")
	cfg := configmock.New(t)
	assert.Equal(t, []string{"go", "java"}, cfg.GetStringSlice(key),
		"env var should override the default and parse as a space-separated list")

	// Single value.
	t.Setenv("DD_LOGS_CONFIG_AUTO_MULTI_LINE_STACK_TRACE_PARSERS", "go")
	cfg = configmock.New(t)
	assert.Equal(t, []string{"go"}, cfg.GetStringSlice(key))
}

// TestGoStackTraceParsersDisableViaEnvVar probes which env var VALUE actually
// disables the feature, documenting the empty-string gotcha.
func TestGoStackTraceParsersDisableViaEnvVar(t *testing.T) {
	const key = "logs_config.auto_multi_line.stack_trace_parsers"
	const envName = "DD_LOGS_CONFIG_AUTO_MULTI_LINE_STACK_TRACE_PARSERS"

	// A real (enabled) aggregator buffers a "panic:" start line and emits
	// nothing yet; the no-op (disabled) aggregator passes it straight through.
	disabled := func(cfg pkgconfigmodel.Reader) bool {
		agg := preprocessor.NewStackTraceAggregatorFromNames(
			cfg.GetStringSlice(key), 1000, true)
		out := agg.Process(message.NewMessage([]byte("panic: boom"), nil, "", 0))
		return len(out) == 1 && agg.IsEmpty()
	}

	// Gotcha: an empty-string env var is treated as unset and falls back to the
	// ["go"] default, so it does NOT disable the feature.
	t.Run("empty string falls back to default", func(t *testing.T) {
		t.Setenv(envName, "")
		cfg := configmock.New(t)
		assert.Equal(t, []string{"go"}, cfg.GetStringSlice(key))
		assert.False(t, disabled(cfg), "empty-string env falls back to default, not disabled")
	})

	// To disable via env, provide a non-empty value with no known parser names.
	// Unknown names are skipped, leaving zero parsers -> the no-op aggregator.
	t.Run("sentinel value disables", func(t *testing.T) {
		t.Setenv(envName, "none")
		cfg := configmock.New(t)
		assert.Equal(t, []string{"none"}, cfg.GetStringSlice(key))
		assert.True(t, disabled(cfg), "an unknown parser name resolves to no parsers -> disabled")
	})

	// The literal string "[]" is recognized as JSON-style empty-list syntax and
	// parsed to an empty slice, cleanly disabling the feature (no unknown-parser
	// warning), unlike an empty string which falls back to the default.
	t.Run("literal brackets string", func(t *testing.T) {
		t.Setenv(envName, "[]")
		cfg := configmock.New(t)
		assert.Empty(t, cfg.GetStringSlice(key), "\"[]\" should parse to an empty slice")
		assert.True(t, disabled(cfg), "\"[]\" -> empty slice -> disabled")
	})

	t.Run("known value stays enabled", func(t *testing.T) {
		t.Setenv(envName, "go")
		cfg := configmock.New(t)
		assert.False(t, disabled(cfg), "'go' should keep the feature enabled")
	})
}

// TestDecoderBlankLineNewlineHandling verifies how blank lines emitted on the
// container/CRI partial-line path are represented after the fix: standalone
// blanks vs blanks embedded in an aggregated Go stack trace, and whether a
// trailing blank produces a trailing (escaped) newline.
func TestDecoderBlankLineNewlineHandling(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.auto_multi_line_detection", true, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_multi_line.stack_trace_parsers", []string{"go"}, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.tag_multi_line_logs", true, pkgconfigmodel.SourceAgentRuntime)

	criLine := func(content string) string {
		return "2024-01-01T00:00:00.000000000Z stderr F " + content + "\n"
	}
	run := func(_ *testing.T, lines []string) []*message.Message {
		source := sources.NewLogSource("", &config.LogsConfig{})
		d := InitializeDecoderForTest(source, kubernetes.New())
		d.Start()
		var input []byte
		for _, l := range lines {
			input = append(input, []byte(criLine(l))...)
		}
		d.InputChan() <- NewInput(input)
		d.Stop()
		var outs []*message.Message
		for o := range d.OutputChan() {
			outs = append(outs, o)
		}
		return outs
	}

	// A standalone blank line (not part of any aggregate) becomes its own
	// message with EMPTY content. It is NOT merged into or appended to the
	// neighbouring messages, so no embedded/trailing newline appears in them.
	// Downstream, tailers drop empty messages via HasContent() before the
	// processor, so it never reaches the backend.
	t.Run("standalone blank line yields empty message, not embedded newline", func(t *testing.T) {
		outs := run(t, []string{"hello", "", "world"})
		require.Len(t, outs, 3)
		assert.Equal(t, "hello", string(outs[0].GetContent()))
		assert.Empty(t, outs[1].GetContent(), "blank line is its own empty message")
		assert.False(t, outs[1].GetContent() != nil && len(outs[1].GetContent()) > 0)
		assert.False(t, outs[1].HasContent(), "empty message is dropped by tailers before the processor")
		assert.Equal(t, "world", string(outs[2].GetContent()))
	})

	// A blank line WITHIN an aggregated trace is preserved as an embedded
	// (escaped) newline in the middle of the combined message — never a real
	// newline, and no trailing newline.
	t.Run("blank line inside stack trace is embedded, no trailing newline", func(t *testing.T) {
		outs := run(t, []string{
			"panic: boom",
			"",
			"goroutine 1 [running]:",
			"main.main()",
			"\t/app/main.go:14 +0x2c",
		})
		require.Len(t, outs, 1)
		content := string(outs[0].GetContent())
		assert.Contains(t, content, `\n\n`, "the blank line survives as an embedded escaped newline")
		assert.NotContains(t, content, "\n", "no real newline bytes are embedded")
		assert.False(t, strings.HasSuffix(content, `\n`), "no trailing escaped newline")
	})

	// EDGE CASE: when an aggregated trace ENDS with a blank line, that trailing
	// blank is currently included, producing a trailing escaped "\n" in the
	// combined message. This matches the pre-existing file-source (SingleLineParser)
	// behavior; the fix brings the container path to parity. It is an escaped
	// "\n", never a real newline.
	t.Run("trailing blank line produces a trailing escaped newline", func(t *testing.T) {
		outs := run(t, []string{
			"panic: boom",
			"",
			"goroutine 1 [running]:",
			"main.main()",
			"\t/app/main.go:14 +0x2c",
			"",
		})
		require.Len(t, outs, 1)
		content := string(outs[0].GetContent())
		assert.True(t, strings.HasSuffix(content, `\n`), "trailing blank is kept as an escaped newline")
		assert.NotContains(t, content, "\n", "still no real newline bytes")
	})
}

// TestDecoderAutoMultilineTimestampBlankLine exercises the DEFAULT auto-multiline
// path (timestamp-based combiningAggregator, NOT the stack trace aggregator) to
// observe how blank lines behave there.
func TestDecoderAutoMultilineTimestampBlankLine(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.auto_multi_line_detection", true, pkgconfigmodel.SourceAgentRuntime)
	// Isolate the timestamp-based path: no stack trace parsers.
	mockConfig.Set("logs_config.auto_multi_line.stack_trace_parsers", []string{}, pkgconfigmodel.SourceAgentRuntime)
	mockConfig.Set("logs_config.tag_multi_line_logs", true, pkgconfigmodel.SourceAgentRuntime)

	criLine := func(content string) string {
		return "2024-01-01T00:00:00.000000000Z stderr F " + content + "\n"
	}
	run := func(_ *testing.T, lines []string) []*message.Message {
		source := sources.NewLogSource("", &config.LogsConfig{})
		d := InitializeDecoderForTest(source, kubernetes.New())
		d.Start()
		var input []byte
		for _, l := range lines {
			input = append(input, []byte(criLine(l))...)
		}
		d.InputChan() <- NewInput(input)
		d.Stop()
		var outs []*message.Message
		for o := range d.OutputChan() {
			outs = append(outs, o)
		}
		return outs
	}

	// A blank line following a timestamped log has no timestamp, so the timestamp
	// detector leaves it labeled "aggregate": it folds into the preceding group as a
	// trailing escaped newline and flips that log to multi-line. The next timestamped
	// line starts a fresh group. This confirms grouping is driven by timestamp presence,
	// and that the blank line is carried through (not dropped) as an escaped newline.
	t.Run("blank between two timestamped lines folds into the preceding group", func(t *testing.T) {
		outs := run(t, []string{
			"2024-03-28 13:45:30 line one",
			"",
			"2024-03-28 13:45:31 line two",
		})
		require.Len(t, outs, 2)

		first := string(outs[0].GetContent())
		assert.Equal(t, `2024-03-28 13:45:30 line one\n`, first)
		assert.True(t, outs[0].ParsingExtra.IsMultiLine, "the folded-in blank line makes this a multi-line log")
		assert.Contains(t, outs[0].ParsingExtra.Tags, "multiline:auto_multiline")
		assert.NotContains(t, first, "\n", "no real newline bytes, only the escaped sequence")

		second := string(outs[1].GetContent())
		assert.Equal(t, "2024-03-28 13:45:31 line two", second)
		assert.False(t, outs[1].ParsingExtra.IsMultiLine)
	})

	// A blank line in the middle of a timestamp-grouped block becomes an embedded
	// escaped newline (\n\n) rather than splitting the group or being dropped.
	t.Run("blank in middle of a timestamped group is embedded as an escaped newline", func(t *testing.T) {
		outs := run(t, []string{
			"2024-03-28 13:45:30 ERROR boom",
			"    at Foo",
			"",
			"    at Bar",
			"2024-03-28 13:45:31 INFO next",
		})
		require.Len(t, outs, 2)

		grouped := string(outs[0].GetContent())
		assert.Equal(t, `2024-03-28 13:45:30 ERROR boom\n    at Foo\n\n    at Bar`, grouped)
		assert.True(t, outs[0].ParsingExtra.IsMultiLine)
		assert.Contains(t, outs[0].ParsingExtra.Tags, "multiline:auto_multiline")
		assert.NotContains(t, grouped, "\n", "no real newline bytes, only escaped sequences")

		assert.Equal(t, "2024-03-28 13:45:31 INFO next", string(outs[1].GetContent()))
		assert.False(t, outs[1].ParsingExtra.IsMultiLine)
	})
}
