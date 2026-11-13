// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The corpus.json regenerator for pkg/logs/internal/parsers/syslog.
//
// testdata/corpus.json holds one message for every syslog-capable integration,
// each copied verbatim from that integration's log-pipeline fixtures, together
// with the fields the parser is expected to produce. The hand-written tests in
// the parser package pin one sample per parsing decision; the corpus exists for
// the opposite reason: to notice changes nobody thought to assert on. Every
// field of every message is compared, so a message that parses without error
// into the wrong fields — invisible to a targeted test — shows up as a diff.
//
// This tool rewrites only the "want" fields of each message from the current
// parser output; the provenance recorded around them is hand-maintained and
// survives the rewrite.
//
// Review the diff produced by an intentional parser change message by
// message: a field moving from a real value to the nilvalue, or CONTENT
// shifting into a header field, is a regression even when the test would
// otherwise pass once regenerated.
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
// The pins are not a dependency — nothing here fetches them — so they only
// need refreshing when messages are added or rechecked.
//
// A message will not always match its upstream fixture byte for byte.
// Pipeline YAML folds long samples across physical lines, so "source" points
// at the first and "line" holds the joined result; octet-counted samples are
// stored without the RFC 6587 MSG-LEN prefix the framer strips before the
// parser runs.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/syslog"
)

// corpusFields is the parse result recorded for each message. Fields are
// compared exactly, including the nilvalue "-", so a header field silently
// losing its value fails verification.
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

// corpusOrigin describes where a group of messages came from.
type corpusOrigin struct {
	Kind string `json:"kind"`
	URL  string `json:"url,omitempty"`
	// Commit the fixtures were read at. Repository origins only.
	Commit string `json:"commit,omitempty"`
}

// corpusFile is the on-disk shape of corpus.json: where the messages came
// from, then the messages themselves.
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

// parseCorpusLine mirrors the dispatch in the parser: a leading '<' means the
// message arrived with a PRI, anything else is a PRI-less file rendering.
func parseCorpusLine(line string) corpusFields {
	content := []byte(line)
	var parsed syslog.SyslogMessage
	var err error
	if len(content) > 0 && content[0] == '<' {
		parsed, err = syslog.Parse(content)
	} else {
		parsed, err = syslog.ParseBSDLine(content)
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

func main() {
	in := flag.String("in", "", "committed corpus.json to regenerate from")
	out := flag.String("out", "", "path to write the regenerated corpus.json")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "-in and -out are required")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *in, err)
		os.Exit(1)
	}
	var corpus corpusFile
	if err := json.Unmarshal(raw, &corpus); err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", *in, err)
		os.Exit(1)
	}
	if len(corpus.Messages) == 0 {
		fmt.Fprintf(os.Stderr, "parse %s: corpus is empty\n", *in)
		os.Exit(1)
	}

	for i := range corpus.Messages {
		corpus.Messages[i].Want = parseCorpusLine(corpus.Messages[i].Line)
	}

	marshaled, err := json.MarshalIndent(corpus, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, append(marshaled, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("regenerated %s: %d messages\n", *out, len(corpus.Messages))
}
