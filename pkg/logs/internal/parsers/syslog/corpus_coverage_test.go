// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdata/corpus.json holds one message for every syslog-capable integration,
// each copied verbatim from that integration's log-pipeline fixtures, together
// with the fields the parser is expected to produce. Its schema, provenance
// rules, and the bazel targets that verify (corpus_golden_test) and adopt
// (corpus_golden) parser changes are documented in generators/corpus, the
// regeneration tool.
//
// The corpus is only useful as a safety net while it still spans the formats
// the agent claims to handle. These bounds fail if messages are deleted
// wholesale or a framing class disappears.

// The subset of the corpus schema this test inspects. The full schema, with
// the "want" fields, lives with the regeneration tool in generators/corpus.
type corpusFile struct {
	Origins  map[string]corpusOrigin `json:"origins"`
	Messages []corpusCase            `json:"messages"`
}

// corpusOrigin describes where a group of messages came from.
type corpusOrigin struct {
	Kind string `json:"kind"`
	URL  string `json:"url,omitempty"`
	// Commit the fixtures were read at. Repository origins only.
	Commit string `json:"commit,omitempty"`
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
	Framing string `json:"framing"`
	Line    string `json:"line"`
}

// Origin kinds. A repository origin is pinned to a commit so line numbers stay
// resolvable; documentation cites a published vendor format; synthesized
// covers messages written here rather than copied from anywhere.
const (
	originRepository    = "repository"
	originDocumentation = "documentation"
	originSynthesized   = "synthesized"
)

const corpusPath = "testdata/corpus.json"

func loadCorpus(t *testing.T) corpusFile {
	t.Helper()
	raw, err := os.ReadFile(corpusPath)
	require.NoError(t, err, "read %s", corpusPath)

	var corpus corpusFile
	require.NoError(t, json.Unmarshal(raw, &corpus), "parse %s", corpusPath)
	require.NotEmpty(t, corpus.Messages, "corpus is empty")
	return corpus
}

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
