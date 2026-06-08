// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package eventbuf

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// TestBufferSnapshot runs each *.yaml file under testdata/snapshot as a
// scripted history against eventbuf.Buffer and verifies the resulting
// Readys and state match the recorded documents. Set REWRITE=1 to
// regenerate the expected outputs after changing Buffer behavior.
//
// File layout: one header document (config + op sequence) followed by one
// output document per op. Ops are tagged YAML mappings:
//
//	!add-fragment {key, side, seq, final, expect_return, bytes}
//	!note-return-lost {key}
//	!note-partial {key, side, last_seq}
//	!discard {key}
//	!evict-stale {max_idle}
//	!close {}
//
// See any of the existing testdata files for examples.
func TestBufferSnapshot(t *testing.T) {
	snapshotDir := filepath.Join("testdata", "snapshot")
	files, err := filepath.Glob(filepath.Join(snapshotDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "no snapshot files in %s", snapshotDir)

	envRewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".yaml")
		t.Run(name, func(t *testing.T) {
			runSnapshotFile(t, file, envRewrite)
		})
	}
}

type snapshotConfig struct {
	BudgetBytes int `yaml:"budget_bytes,omitempty"`
}

func runSnapshotFile(t *testing.T, file string, envRewrite bool) {
	content, err := os.ReadFile(file)
	require.NoError(t, err)

	docs, err := splitYAMLDocuments(content)
	require.NoError(t, err)
	require.NotEmpty(t, docs, "file contains no documents")

	// Parse header doc.
	var header struct {
		Config snapshotConfig `yaml:"config"`
		Ops    []yaml.Node    `yaml:"ops"`
	}
	require.NoError(t, yaml.Unmarshal(docs[0], &header),
		"failed to parse header document")

	// An unspecified budget_bytes means "effectively unlimited" — use a
	// large ceiling rather than a distinct no-budget code path.
	budgetBytes := header.Config.BudgetBytes
	if budgetBytes == 0 {
		budgetBytes = 1 << 30
	}
	budget := NewBudget(budgetBytes)
	b := NewBuffer(budget)

	// Keep a side-map from short key identifier (e.g. "k1") to eventbuf.Key,
	// so tests can refer to invocations by short name. First use of a name
	// fixes its Key; subsequent uses reuse it.
	keyMap := map[string]Key{}
	nextKeyID := uint64(1)
	keyFor := func(name string) Key {
		if k, ok := keyMap[name]; ok {
			return k
		}
		k := Key{
			Goid:           nextKeyID,
			StackByteDepth: 100,
			ProbeID:        uint32(nextKeyID),
			EntryKtime:     nextKeyID * 1000,
		}
		nextKeyID++
		keyMap[name] = k
		return k
	}
	// For printing, invert the keyMap to get a name from a Key.
	keyName := func(k Key) string {
		for n, kk := range keyMap {
			if kk == k {
				return n
			}
		}
		return fmt.Sprintf("{%d,%d,%d,%d}", k.Goid, k.StackByteDepth, k.ProbeID, k.EntryKtime)
	}

	var outputs [][]byte
	defer func() {
		if t.Failed() {
			for i, doc := range outputs {
				t.Logf("generated output[%d]:\n%s\n---", i, strings.TrimSpace(string(doc)))
			}
		}
	}()

	for i, opNode := range header.Ops {
		var budgetEvictions []Ready
		var readys []Ready

		tag := strings.TrimPrefix(opNode.Tag, "!")
		switch tag {
		case "add-fragment":
			var a struct {
				Key          string `yaml:"key"`
				Side         string `yaml:"side"`
				Seq          uint16 `yaml:"seq"`
				Final        bool   `yaml:"final"`
				ExpectReturn bool   `yaml:"expect_return"`
				Bytes        int    `yaml:"bytes"`
			}
			require.NoError(t, opNode.Decode(&a),
				"op %d: decode add-fragment", i)
			side, err := parseSide(a.Side)
			require.NoError(t, err, "op %d", i)
			msg := newTestMessage(a.Bytes)
			r, done := b.AddFragment(
				keyFor(a.Key), msg, side, a.Seq, a.Final, a.ExpectReturn,
			)
			if done {
				readys = append(readys, r)
			}
			budgetEvictions = b.TakePendingBudgetEvictions()
		case "note-return-lost":
			var a struct {
				Key string `yaml:"key"`
			}
			require.NoError(t, opNode.Decode(&a))
			r, done := b.NoteReturnLost(keyFor(a.Key))
			if done {
				readys = append(readys, r)
			}
		case "note-partial":
			var a struct {
				Key     string `yaml:"key"`
				Side    string `yaml:"side"`
				LastSeq uint16 `yaml:"last_seq"`
			}
			require.NoError(t, opNode.Decode(&a))
			side, err := parseSide(a.Side)
			require.NoError(t, err)
			r, done := b.NotePartial(keyFor(a.Key), side, a.LastSeq)
			if done {
				readys = append(readys, r)
			}
		case "discard":
			var a struct {
				Key string `yaml:"key"`
			}
			require.NoError(t, opNode.Decode(&a))
			b.Discard(keyFor(a.Key))
		case "evict-older-than":
			var a struct {
				Cutoff uint64 `yaml:"cutoff"`
			}
			require.NoError(t, opNode.Decode(&a))
			readys = b.EvictOlderThan(a.Cutoff)
		case "close":
			readys = b.Close()
		default:
			t.Fatalf("op %d: unknown tag %q", i, opNode.Tag)
		}

		// Always release the Ready message lists after recording them. The
		// snapshot only captures counts + flags; releasing here mirrors the
		// sink's ownership contract.
		releaseReadys := func(rs []Ready) {
			for _, r := range rs {
				if r.Entry != nil {
					r.Entry.Release()
				}
				if r.Return != nil {
					r.Return.Release()
				}
			}
		}

		doc := buildSnapshotOutput(opNode, readys, budgetEvictions, b, keyName)
		outputs = append(outputs, doc)

		releaseReadys(readys)
		releaseReadys(budgetEvictions)

		if len(docs) > i+1 && !envRewrite {
			require.Equal(t, string(docs[i+1]), string(doc),
				"mismatch at op %d (%s)", i, tag)
		}
	}

	if envRewrite {
		var buf bytes.Buffer
		buf.Write(docs[0])
		for _, out := range outputs {
			buf.WriteString("---\n")
			buf.Write(out)
		}
		require.NoError(t, os.WriteFile(file, buf.Bytes(), fs.FileMode(0644)))
	}
}

func parseSide(s string) (Side, error) {
	switch s {
	case "entry":
		return Entry, nil
	case "return":
		return Return, nil
	default:
		return 0, fmt.Errorf("invalid side %q", s)
	}
}

// buildSnapshotOutput renders one op's outcome as a YAML document. The
// shape is:
//
//	op: !add-fragment {...}        # the input op, verbatim
//	readys:                        # only if any were emitted
//	  - {key: k1, entry_fragments: 2, ..., entry_truncated: true}
//	budget_evictions:              # only if any (post-AddFragment)
//	  - {...}
//	state:
//	  tree_size: 3
//	  bytes: 128
//	  budget_used: 128
func buildSnapshotOutput(
	opNode yaml.Node,
	readys []Ready,
	budgetEvictions []Ready,
	b *Buffer,
	keyName func(Key) string,
) []byte {
	var doc yaml.Node
	doc.Kind = yaml.DocumentNode
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	doc.Content = []*yaml.Node{mapping}

	// op (verbatim input node, with comments stripped so per-op inline
	// comments in the header don't propagate into the generated output).
	opCopy := opNode
	stripComments(&opCopy)
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "op"},
		&opCopy,
	)

	if len(readys) > 0 {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "readys"},
			readysNode(readys, keyName),
		)
	}
	if len(budgetEvictions) > 0 {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "budget_evictions"},
			readysNode(budgetEvictions, keyName),
		)
	}

	stateMap := map[string]any{
		"tree_size":   b.Len(),
		"bytes":       b.Bytes(),
		"budget_used": b.budget.Used(),
	}
	var stateNode yaml.Node
	_ = stateNode.Encode(stateMap)
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "state"},
		&stateNode,
	)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(&doc)
	_ = enc.Close()
	return buf.Bytes()
}

// readysNode returns a YAML sequence describing a slice of Readys. Each
// Ready is rendered in flow style on one line for readability.
func readysNode(readys []Ready, keyName func(Key) string) *yaml.Node {
	// Stable ordering: by key-name.
	sorted := append([]Ready(nil), readys...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return keyName(sorted[i].Key) < keyName(sorted[j].Key)
	})
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, r := range sorted {
		entryFrags := 0
		entryBytes := 0
		if r.Entry != nil {
			for ev := range r.Entry.Fragments() {
				entryFrags++
				entryBytes += len(ev)
			}
		}
		returnFrags := 0
		returnBytes := 0
		if r.Return != nil {
			for ev := range r.Return.Fragments() {
				returnFrags++
				returnBytes += len(ev)
			}
		}

		m := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		m.Content = append(m.Content,
			scalar("key"), scalar(keyName(r.Key)),
			scalar("entry_frags"), scalar(strconv.Itoa(entryFrags)),
			scalar("entry_bytes"), scalar(strconv.Itoa(entryBytes)),
			scalar("return_frags"), scalar(strconv.Itoa(returnFrags)),
			scalar("return_bytes"), scalar(strconv.Itoa(returnBytes)),
		)
		if r.EntryTruncated {
			m.Content = append(m.Content, scalar("entry_truncated"), scalar("true"))
		}
		if r.ReturnTruncated {
			m.Content = append(m.Content, scalar("return_truncated"), scalar("true"))
		}
		if r.ReturnLost {
			m.Content = append(m.Content, scalar("return_lost"), scalar("true"))
		}
		seq.Content = append(seq.Content, m)
	}
	return seq
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
}

// stripComments recursively clears HeadComment / LineComment / FootComment
// on a yaml.Node so that comments present in the source (e.g. inline
// annotations on ops in the header) don't bleed into generated output docs.
func stripComments(n *yaml.Node) {
	if n == nil {
		return
	}
	n.HeadComment = ""
	n.LineComment = ""
	n.FootComment = ""
	for _, c := range n.Content {
		stripComments(c)
	}
}

// splitYAMLDocuments copied from the uploader snapshot test.
func splitYAMLDocuments(content []byte) ([][]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	scanner.Split(bufio.ScanLines)

	var documents [][]byte
	var current []byte
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.HasPrefix(line, []byte("---")) {
			documents = append(documents, current)
			current = nil
		} else {
			current = append(current, line...)
			current = append(current, '\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(current) > 0 {
		documents = append(documents, current)
	}
	return documents, nil
}
