// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdata/corpus.json holds one message for every syslog-capable integration,
// each copied verbatim from that integration's log-pipeline fixtures, together
// with the fields the parser is expected to produce.
//
// The hand-written tests elsewhere in this package pin one sample per parsing
// decision. This file exists for the opposite reason: to notice changes nobody
// thought to assert on. A message that parses without error into the wrong
// fields is invisible to a targeted test, since nobody writes an assertion for
// a value they never expected, but it shows up here as a diff, because every
// field of every message is compared.
//
// # Where the messages come from
//
// Every message names an origin, described in the "origins" object at the top of
// corpus.json, so that "source" can be resolved without guessing. An origin is
// one of three kinds:
//
//	repository      a public Datadog integrations repo, pinned to a commit;
//	                "source" is a path from its root plus a line number
//	documentation   a published vendor format example; "source" cites the section
//	synthesized     constructed here to model a documented shape, not copied
//
// The pins are not a dependency — nothing here fetches them — so they only need
// refreshing when messages are added or rechecked.
//
// A message will not always match its upstream fixture byte for byte. Pipeline
// YAML folds long samples across physical lines, so "source" points at the first
// and "line" holds the joined result; octet-counted samples are stored without
// the RFC 6587 MSG-LEN prefix the framer strips before the parser runs.
//
// To adopt new output after an intentional parser change:
//
//	UPDATE_SYSLOG_CORPUS=1 dda inv test \
//	  --targets=./pkg/logs/internal/parsers/syslog -e TestCorpusGolden
//
// Review the resulting diff message by message. A field moving from a real
// value to the nilvalue, or CONTENT shifting into a header field, is a
// regression even when the test would otherwise pass once regenerated.
//
// The trigger is an environment variable rather than a -update-golden flag
// because go test only accepts test-binary flags after the package list, while
// the dda inv wrapper always emits flags ahead of it. A flag would therefore be
// reachable only by invoking go test directly, which the repository forbids.
//
// Should regenerating ever grow past a single command — more golden files in
// this package, or work to do either side of the rewrite — the idiom to reach
// for is an invoke task running go test from the package directory, the way
// host-profiler.update-golden-tests does. That is not worth a task module and a
// collection registration while one environment variable covers it.
var updateGolden = os.Getenv("UPDATE_SYSLOG_CORPUS") != ""

const goldenPath = "testdata/corpus.json"

// corpusFields is the parse result recorded for each message. Fields are
// compared exactly, including the nilvalue "-", so a header field silently
// losing its value fails the test.
type corpusFields struct {
	Err       string `json:"err,omitempty"`
	Pri       int    `json:"pri"`
	Version   string `json:"version,omitempty"`
	Timestamp string `json:"timestamp"`
	Hostname  string `json:"hostname"`
	AppName   string `json:"appname"`
	ProcID    string `json:"procid"`
	MsgID     string `json:"msgid"`
	Msg       string `json:"msg"`
	Partial   bool   `json:"partial,omitempty"`
}

// Origin kinds. A repository origin is pinned to a commit so line numbers stay
// resolvable; documentation cites a published vendor format; synthesized covers
// messages written here rather than copied from anywhere.
const (
	originRepository    = "repository"
	originDocumentation = "documentation"
	originSynthesized   = "synthesized"
)

// corpusOrigin describes where a group of messages came from.
type corpusOrigin struct {
	Kind string `json:"kind"`
	URL  string `json:"url,omitempty"`
	// Commit the fixtures were read at. Repository origins only.
	Commit string `json:"commit,omitempty"`
}

// corpusFile is the on-disk shape of corpus.json: where the messages came from,
// then the messages themselves.
type corpusFile struct {
	Origins  map[string]corpusOrigin `json:"origins"`
	Messages []corpusCase            `json:"messages"`
}

type corpusCase struct {
	// Integration owning the fixture the message was copied from.
	Integration string `json:"integration"`
	// Source locates the message within Origin: a path and line number for a
	// repository, a cited section for documentation.
	Source string `json:"source"`
	// Origin names the entry in corpusFile.Origins that Source resolves against.
	Origin string `json:"origin"`
	// Transport is "wire" for messages captured with a PRI, "file" for the
	// PRI-less rendering a tailed file carries.
	Transport string `json:"transport"`
	// Framing is the wire layout, as classified when the corpus was built.
	// Octet-counted messages are stored without their RFC 6587 MSG-LEN prefix
	// because the framer strips it before the parser runs.
	Framing string       `json:"framing"`
	Line    string       `json:"line"`
	Want    corpusFields `json:"want"`
}

// parseCorpusLine mirrors the dispatch in parser.Parse: a leading '<' means the
// message arrived with a PRI, anything else is a PRI-less file rendering.
func parseCorpusLine(line string) corpusFields {
	content := []byte(line)
	var parsed SyslogMessage
	var err error
	if len(content) > 0 && content[0] == '<' {
		parsed, err = Parse(content)
	} else {
		parsed, err = ParseBSDLine(content)
	}

	got := corpusFields{
		Pri:       parsed.Pri,
		Version:   parsed.Version,
		Timestamp: parsed.Timestamp,
		Hostname:  parsed.Hostname,
		AppName:   parsed.AppName,
		ProcID:    parsed.ProcID,
		MsgID:     parsed.MsgID,
		Msg:       string(parsed.Msg),
		Partial:   parsed.Partial,
	}
	if err != nil {
		got.Err = err.Error()
	}
	return got
}

func loadCorpus(t *testing.T) corpusFile {
	t.Helper()
	raw, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "read %s", goldenPath)

	var corpus corpusFile
	require.NoError(t, json.Unmarshal(raw, &corpus), "parse %s", goldenPath)
	require.NotEmpty(t, corpus.Messages, "corpus is empty")
	return corpus
}

func TestCorpusGolden(t *testing.T) {
	corpus := loadCorpus(t)

	if updateGolden {
		// Only Want is regenerated; the origin pins and the provenance recorded
		// with each message are hand-maintained and must survive the rewrite.
		for i := range corpus.Messages {
			corpus.Messages[i].Want = parseCorpusLine(corpus.Messages[i].Line)
		}
		out, err := json.MarshalIndent(corpus, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Clean(goldenPath), append(out, '\n'), 0o644))
		t.Logf("rewrote %s with %d messages", goldenPath, len(corpus.Messages))
		return
	}

	for _, tc := range corpus.Messages {
		t.Run(tc.Integration+"/"+tc.Source, func(t *testing.T) {
			got := parseCorpusLine(tc.Line)
			assert.Equal(t, tc.Want, got,
				"parse of %s changed\nline: %q\nset UPDATE_SYSLOG_CORPUS=1 to adopt",
				tc.Source, tc.Line)
		})
	}
}

// The corpus is only useful as a safety net while it still spans the formats
// the agent claims to handle. These bounds fail if messages are deleted
// wholesale or a framing class disappears.
func TestCorpusGoldenCoverage(t *testing.T) {
	corpus := loadCorpus(t)

	integrations := make(map[string]struct{})
	framings := make(map[string]struct{})
	transports := make(map[string]struct{})
	for _, tc := range corpus.Messages {
		integrations[tc.Integration] = struct{}{}
		framings[tc.Framing] = struct{}{}
		transports[tc.Transport] = struct{}{}
		assert.NotEmpty(t, tc.Line, "%s has no message", tc.Source)
		assert.NotEmpty(t, tc.Source, "%s has no provenance", tc.Integration)
		assert.Contains(t, corpus.Origins, tc.Origin,
			"%s names origin %q, which is not described in the origins object",
			tc.Source, tc.Origin)
	}

	// Provenance is only worth recording if it can be followed back: a
	// repository needs a commit to make its line numbers resolvable, and
	// documentation needs somewhere to read it.
	for name, origin := range corpus.Origins {
		switch origin.Kind {
		case originRepository:
			assert.NotEmpty(t, origin.URL, "origin %s has no url", name)
			assert.Len(t, origin.Commit, 40, "origin %s needs a full commit sha", name)
		case originDocumentation:
			assert.NotEmpty(t, origin.URL, "origin %s has no url", name)
		case originSynthesized:
		default:
			t.Errorf("origin %s has unknown kind %q", name, origin.Kind)
		}
	}

	assert.GreaterOrEqual(t, len(integrations), 22, "integrations covered")
	assert.Subset(t,
		keysOf(framings),
		[]string{"rfc3164_pri", "bsd_nopri", "iso_pri", "iso_nopri",
			"rfc5424", "yearfirst_nopri", "octet_counted"},
		"every framing class the parser handles must stay represented")
	assert.Subset(t, keysOf(transports), []string{"wire", "file"})
}

func keysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
