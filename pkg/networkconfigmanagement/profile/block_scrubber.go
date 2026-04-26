// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

package profile

import (
	"bytes"
	"fmt"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// Secrets is a set of redacted bytestrings identified by string keys.
type Secrets map[string][]byte

// Block represents a renderable portion of a config.
type Block interface {
	BlockType() string
	RenderTo(output *bytes.Buffer, secrets Secrets) error
	Render(secrets Secrets) ([]byte, error)
	// For debugging
	String() string
}

// PlainText is just a plain block of bytes.
type PlainText struct {
	Text []byte
}

var _ Block = (*PlainText)(nil)

func (p *PlainText) BlockType() string {
	return "text"
}

func (p *PlainText) String() string {
	return string(p.Text)
}

func (p *PlainText) RenderTo(output *bytes.Buffer, _ Secrets) error {
	_, err := output.Write(p.Text)
	return err
}

func (p *PlainText) Render(secrets Secrets) ([]byte, error) {
	var b bytes.Buffer
	err := p.RenderTo(&b, secrets)
	return b.Bytes(), err
}

// Secret represents a redacted block of bytes, identified by a string ID.
type Secret struct {
	ID string
}

var _ Block = (*Secret)(nil)

func (s *Secret) String() string {
	return "[" + string(s.ID) + "]"
}

func (s *Secret) BlockType() string {
	return "secret"
}

func (s *Secret) RenderTo(buffer *bytes.Buffer, secrets Secrets) error {
	if result, ok := secrets[s.ID]; ok {
		_, err := buffer.Write(result)
		return err
	}
	return fmt.Errorf("unknown secret ID %q", s.ID)
}

func (s *Secret) Render(secrets Secrets) ([]byte, error) {
	var b bytes.Buffer
	err := s.RenderTo(&b, secrets)
	return b.Bytes(), err
}

// Redact generates a Recation from a bytestring and a list of redaction rules.
func Redact(input []byte, rules []RedactionRule) ([]Block, Secrets, error) {
	if len(rules) == 0 {
		return []Block{&PlainText{Text: input}}, nil, nil
	}
	// find a delimiter that doesn't already appear in the input
	delimiter := []byte(uuid.NewString())
	for bytes.Contains(input, delimiter) {
		// the odds of this actually happening are less than the odds of a
		// meteor destroying your datacenter.
		delimiter = []byte(uuid.NewString())
	}
	redactions := Secrets{}
	s := scrubber.New()
	for _, rule := range rules {
		replacer := scrubber.Replacer{
			Regex: rule.Regex,
			ReplFunc: func(b []byte) []byte {
				subs := rule.Regex.FindSubmatchIndex(b)
				var secStart, secEnd int
				for i, name := range rule.Regex.SubexpNames() {
					if name == "secret" {
						secStart = subs[i*2]
						secEnd = subs[i*2+1]
					}
				}
				if secStart == secEnd {
					// TODO maybe log a warning if we match a rule but with no matched secret?
					return b
				}
				if bytes.Contains(b[secStart:secEnd], delimiter) {
					// don't redact a line we've already redacted
					// TODO are there cases where we actually do want to do this?
					// e.g. what if we match "secret 5 .*" but there's also a multiline embedded thing that randomly happens to contain that?
					return b
				}
				secret := rule.Regex.Expand(nil, []byte(`${secret}`), b, subs)
				placeholder := "secret"
				secretID := "secret-1"
				if rule.Replacement != "" {
					placeholder = string(rule.Regex.Expand(nil, []byte(rule.Replacement), b, subs))
					secretID = placeholder
				}
				i := 1
				for _, ok := redactions[secretID]; ok; _, ok = redactions[secretID] {
					i++
					secretID = fmt.Sprintf("%s-%d", placeholder, i)
				}
				// replace the matched secrets block with delimiter+secretID+delimiter
				redacted := bytes.Join([][]byte{b[:secStart], delimiter, []byte(secretID), delimiter, b[secEnd:]}, []byte(""))
				redactions[secretID] = secret
				return redacted
			},
		}
		mode := scrubber.SingleLine
		if rule.Multiline {
			mode = scrubber.MultiLine
		}
		s.AddReplacer(mode, replacer)
	}
	scrubbed, err := s.ScrubBytes(input)
	if err != nil {
		return nil, nil, err
	}
	var blocks []Block
	for i, chunk := range bytes.Split(scrubbed, delimiter[:]) {
		if i%2 == 0 {
			blocks = append(blocks, &PlainText{chunk})
		} else {
			blocks = append(blocks, &Secret{
				ID: string(chunk),
			})
		}
	}
	return blocks, redactions, nil
}

// Assemble renders
func Assemble(blocks []Block, secrets Secrets) ([]byte, error) {
	var buf bytes.Buffer
	for _, b := range blocks {
		if err := b.RenderTo(&buf, secrets); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
