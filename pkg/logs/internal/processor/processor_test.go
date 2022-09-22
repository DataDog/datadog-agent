// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestExclusion(t *testing.T) {
	p := &Processor{}

	var shouldProcess bool
	var redactedMessage []byte

	source := newSource("exclude_at_match", "", "world")
	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("hello"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, _ = p.applyRedactingRules(newMessage([]byte("world"), &source, ""))
	assert.Equal(t, false, shouldProcess)

	shouldProcess, _ = p.applyRedactingRules(newMessage([]byte("a brand new world"), &source, ""))
	assert.Equal(t, false, shouldProcess)

	source = newSource("exclude_at_match", "", "$world")
	shouldProcess, _ = p.applyRedactingRules(newMessage([]byte("a brand new world"), &source, ""))
	assert.Equal(t, true, shouldProcess)
}

func TestReplaceTraceID(t *testing.T) {
	for _, tt := range []struct {
		in, out string
	}{
		{
			in:  "aslkdjasdldjas trace_id=1b8c31d6d2fa97a97d0763ab555c1d19",
			out: "aslkdjasdldjas trace_id=9009279167100624153",
		},
		{
			in:  "aslkdjasdldjas trace_id=1b8c31d6d2fa97a97d0763ab555c1d19 other_attr=2",
			out: "aslkdjasdldjas trace_id=9009279167100624153 other_attr=2",
		},
		{in: "aslkdjasdldjas trace_id=b8c31d6d2fa97a97d0763ab555c1d19 xyz"},   // too short
		{in: "aslkdjasdldjas trace_id=11b8c31d6d2fa97a97d0763ab555c1d19 xyz"}, // too long
		{in: "aslkdjasdldjas trace_id=b8c31d6d2fa97a97d0763ab555c1d19"},
		{in: "aslkdjasdldjas trace_id=11b8c31d6d2fa97a97d0763ab555c1d19"},
		{in: "aslkdjasdldjas trace_id="},
		{in: "aslkdjasdldjas trace_id= "},
		{in: "aslkdjasdldjas trace_id= xyz"},
		{in: "aslkdjasdldjas trace_id=1"},
		{in: "aslkdjasdldjas trace_id=1g8c31d6d2fa97a97d0763ab555c1d19"},
	} {
		out := tt.in
		if tt.out != "" {
			out = tt.out
		}
		got := string(replaceTraceID([]byte(tt.in)))
		require.Equal(t, out, got)
	}
}

// Avoid benchmark compiler optimisations, see:
// https://dave.cheney.net/2013/06/30/how-to-write-benchmarks-in-go#compiler-optimisation
var result1, result2 []byte

func BenchmarkReplaceTraceID(b *testing.B) {
	for name, in := range map[string]string{
		"regular":  "log message without a date or any sort of trace id contained within it",
		"short":    "log message with a weird trace id contained at the end that is not long enough trace_id=12387987ds",
		"uint64":   "log message with a weird trace id contained at the end that is not long enough trace_id=123879879 span_id=123 other_field=3",
		"128bit":   "log message with a weird trace id contained at the end that is not long enough trace_id=1b8c31d6d2fa97a97d0763ab555c1d19",
		"128bit/2": "log message with a weird trace id contained at the end that is not long enough trace_id=1b8c31d6d2fa97a97d0763ab555c1d19 span_id=123 other_field=3",
	} {
		content := []byte(in)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				result1 = replaceTraceID(content)
			}
		})
	}
}

func BenchmarkApplyRedactingRules(b *testing.B) {
	source := &sources.LogSource{Config: &config.LogsConfig{}}
	p := &Processor{
		processingRules: []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "world", "Matt"),
		},
	}
	base := "this is a log message that doesn't report anything interesting but does say hello to Matt without any trace ids or anything special !"
	content := []byte(base)
	msg := newMessage([]byte(content), source, "")
	b.Run("off", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, result2 = p.applyRedactingRules(msg)
		}
	})
	p.traceIDRule = true
	for name, in := range map[string]string{
		"on":     base,
		"uint64": "this is a log message that doesn't report anything interesting but does say hello to Matt with an old styl dd.trace_id=12387889321 !",
		"128bit": "this is a log message that doesn't report anything interesting but does say hello to Matt with a trace_id=1b8c31d6d2fa97a97d0763ab555c1d19 !",
	} {
		content := []byte(in)
		msg := newMessage([]byte(content), source, "")
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, result2 = p.applyRedactingRules(msg)
			}
		})
	}
}

func TestInclusion(t *testing.T) {
	p := &Processor{processingRules: []*config.ProcessingRule{newProcessingRule("include_at_match", "", "world")}}

	var shouldProcess bool
	var redactedMessage []byte

	source := sources.LogSource{Config: &config.LogsConfig{}}
	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("hello"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("world"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("world"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("a brand new world"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("a brand new world"), redactedMessage)

	source = newSource("include_at_match", "", "^world")
	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("a brand new world"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)
}

func TestExclusionWithInclusion(t *testing.T) {
	eRule := newProcessingRule("exclude_at_match", "", "^bob")
	iRule := newProcessingRule("include_at_match", "", ".*@datadoghq.com$")

	p := &Processor{processingRules: []*config.ProcessingRule{eRule}}

	var shouldProcess bool
	var redactedMessage []byte

	source := sources.LogSource{Config: &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{iRule}}}

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("bob@datadoghq.com"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("bill@datadoghq.com"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("bill@datadoghq.com"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("bob@amail.com"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("bill@amail.com"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)
}

func TestMask(t *testing.T) {
	p := &Processor{}

	var shouldProcess bool
	var redactedMessage []byte

	source := newSource("mask_sequences", "[masked_world]", "world")
	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("hello"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("hello world!"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello [masked_world]!"), redactedMessage)

	source = newSource("mask_sequences", "[masked_user]", "User=\\w+@datadoghq.com")
	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("new test launched by User=beats@datadoghq.com on localhost"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("new test launched by [masked_user] on localhost"), redactedMessage)

	source = newSource("mask_sequences", "[masked_credit_card]", "(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\\d{3})\\d{11})")
	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("The credit card 4323124312341234 was used to buy some time"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("The credit card [masked_credit_card] was used to buy some time"), redactedMessage)

	source = newSource("mask_sequences", "${1}[masked_value]", "([Dd]ata_?values=)\\S+")
	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("New data added to Datavalues=123456 on prod"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("New data added to Datavalues=[masked_value] on prod"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("New data added to data_values=123456 on prod"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("New data added to data_values=[masked_value] on prod"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newMessage([]byte("New data added to data_values= on prod"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("New data added to data_values= on prod"), redactedMessage)
}

func TestTruncate(t *testing.T) {
	p := &Processor{}

	source := sources.NewLogSource("", &config.LogsConfig{})
	var redactedMessage []byte

	_, redactedMessage = p.applyRedactingRules(newMessage([]byte("hello"), source, ""))
	assert.Equal(t, []byte("hello"), redactedMessage)
}

func newProcessingRule(ruleType, replacePlaceholder, pattern string) *config.ProcessingRule {
	return &config.ProcessingRule{
		Type:               ruleType,
		Name:               "test",
		ReplacePlaceholder: replacePlaceholder,
		Placeholder:        []byte(replacePlaceholder),
		Pattern:            pattern,
		Regex:              regexp.MustCompile(pattern),
	}
}

func newSource(ruleType, replacePlaceholder, pattern string) sources.LogSource {
	return sources.LogSource{Config: &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{newProcessingRule(ruleType, replacePlaceholder, pattern)}}}
}

func newMessage(content []byte, source *sources.LogSource, status string) *message.Message {
	return message.NewMessageWithSource(content, status, source, 0)
}
