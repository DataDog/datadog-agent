// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// startTime is a deterministic epoch to ensure that absolute timestamps in
// snapshot documents are stable across test runs.
var startTime = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestBatcherSnapshot(t *testing.T) {
	snapshotDir := filepath.Join("testdata", "snapshot")
	files, err := filepath.Glob(filepath.Join(snapshotDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "no snapshot *.yaml files found in %s", snapshotDir)

	envRewrite := false
	if v := os.Getenv("REWRITE"); v != "" {
		if b, _ := strconv.ParseBool(v); b {
			envRewrite = true
		}
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".yaml")
		t.Run(name, func(t *testing.T) {
			runSnapshotFile(t, file, envRewrite)
		})
	}
}

func runSnapshotFile(t *testing.T, file string, envRewrite bool) {
	content, err := os.ReadFile(file)
	require.NoError(t, err)

	docs, err := splitYAMLDocuments(content)
	require.NoError(t, err)
	require.Greater(t, len(docs), 0)

	// first document – parse with yaml.Node to preserve custom tags
	var root yaml.Node
	require.NoError(t, yaml.Unmarshal(docs[0], &root))
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		t.Fatalf("invalid header document structure")
	}
	headerMap := root.Content[0]
	var cfg batcherConfig
	var eventsNode *yaml.Node
	// walk mapping nodes
	for i := 0; i < len(headerMap.Content); i += 2 {
		key := headerMap.Content[i].Value
		val := headerMap.Content[i+1]
		switch key {
		case "config":
			var c struct {
				MaxItems     int `yaml:"max_items"`
				MaxSizeBytes int `yaml:"max_size_bytes"`
				IdleFlushMs  int `yaml:"idle_flush_ms"`
				MaxBufferMs  int `yaml:"max_buffer_ms"`
			}
			require.NoError(t, val.Decode(&c))
			cfg = batcherConfig{
				maxBatchItems:     c.MaxItems,
				maxBatchSizeBytes: c.MaxSizeBytes,
				maxBufferDuration: time.Duration(c.MaxBufferMs) * time.Millisecond,
			}
		case "events":
			eventsNode = val
		}
	}
	require.NotNil(t, eventsNode, "events list not found in header document")
	require.Equal(t, yaml.SequenceNode, eventsNode.Kind, "events should be sequence")

	events, err := parseBatcherEvents(eventsNode.Content)
	require.NoError(t, err)

	state := newBatcherState(cfg)
	virtualNow := time.Duration(0)

	var outputs [][]byte
	var lastMetrics = state.metrics.Stats()
	lastReset := 0
	var lastErr error

	// If the test fails, print the generated output documents so that developers
	// can copy-paste them into the snapshot file when diagnosing differences.
	defer func() {
		if t.Failed() {
			for i, doc := range outputs {
				if len(doc) == 0 {
					continue
				}
				t.Logf("generated output[%d]:\n%s\n---", i, strings.TrimSpace(string(doc)))
			}
		}
	}()

	var nextFlush time.Time
	for i, yEv := range events {
		if lastErr != nil {
			t.Fatalf("unexpected error (can only occur on last event): %v", lastErr)
		}

		if yEv.advance > 0 {
			virtualNow += yEv.advance
		} else if _, isTimerFired := yEv.event.(timerFiredEvent); isTimerFired {
			virtualNow += nextFlush.Sub(startTime)
		}

		// No immediate logging; if the test fails we will dump the generated output
		// documents in the deferred reporter above.

		goNow := startTime.Add(virtualNow)

		eff := &snapshotEffects{
			now:        virtualNow,
			prevResetP: &lastReset,
			nextFlush:  &nextFlush,
		}
		errEvent := state.handleEvent(yEv.event, goNow, eff)
		lastErr = errEvent

		doc := buildOutputDoc(
			yEv.node, eff.nodes, state, lastMetrics, virtualNow, nextFlush,
		)
		outputs = append(outputs, doc)
		lastMetrics = state.metrics.Stats()

		if len(docs) > i+1 {
			if !envRewrite {
				require.Equal(t, string(docs[i+1]), string(doc))
			}
		} else {
			// No expected doc recorded yet; treat as rewrite session for this file.
			envRewrite = true
		}
	}

	// validate expected error if header has error regex
	if errRegexNode := findChildByKey(headerMap, "error"); errRegexNode != nil {
		pattern := errRegexNode.Value
		if !regexp.MustCompile(pattern).MatchString(fmt.Sprint(lastErr)) {
			t.Fatalf("expected error matching %s, got %v", pattern, lastErr)
		}
	} else if lastErr != nil {
		t.Fatalf("unexpected error: %v", lastErr)
	}

	if envRewrite {
		// write back file: header + generated
		var buf bytes.Buffer
		buf.Write(docs[0])
		for _, out := range outputs {
			buf.WriteString("---\n")
			buf.Write(out)
		}
		require.NoError(t, os.WriteFile(file, buf.Bytes(), fs.FileMode(0644)))
	}
}

type event interface {
	isEvent()
}

type enqueueEvent struct {
	data json.RawMessage
}

func (enqueueEvent) isEvent() {}

type timerFiredEvent struct{}

func (timerFiredEvent) isEvent() {}

type batchOutcomeEvent sendResult

func (batchOutcomeEvent) isEvent() {}

type stopEvent struct{}

func (stopEvent) isEvent() {}

// handleEvent processes an event and calls methods on the effects interface.
func (s *batcherState) handleEvent(e event, now time.Time, eff effects) error {
	switch ev := e.(type) {
	case enqueueEvent:
		s.handleEnqueueEvent(ev.data, now, eff)
		return nil
	case timerFiredEvent:
		return s.handleTimerFiredEvent(eff)
	case batchOutcomeEvent:
		return s.handleBatchOutcomeEvent(sendResult(ev), eff)
	case stopEvent:
		s.handleStopEvent(eff)
		return nil
	default:
		return fmt.Errorf("unsupported event type %T", ev)
	}
}

type enqueueYAML struct {
	Value string `yaml:"value"`
}

type timerFiredYAML struct {
	Advance *string `yaml:"advance,omitempty"`
}

type batchOutcomeYAML struct {
	ID      uint64 `yaml:"id"`
	Success bool   `yaml:"success"`
}

type enqueueBytesYAML struct {
	Size int `yaml:"size"`
}

// yamlBatchEvent wraps an uploader event to provide YAML (un)marshaling. It
// also keeps the virtual time advance specified by the YAML document.
type yamlBatchEvent struct {
	event   event
	advance time.Duration // how much virtual time to advance before the event
}

// MarshalYAML encodes the event using a custom tag matching the event type.
// It is currently only used when regenerating snapshot files while REWRITE is
// set, but implementing it makes round-trip tests possible.
func (y yamlBatchEvent) MarshalYAML() (any, error) {
	encode := func(tag string, v any) (*yaml.Node, error) {
		n := &yaml.Node{}
		if err := n.Encode(v); err != nil {
			return nil, err
		}
		n.Tag = tag
		return n, nil
	}

	switch ev := y.event.(type) {
	case enqueueEvent:
		var v enqueueYAML
		_ = json.Unmarshal(ev.data, &v.Value)
		return encode("!enqueue", v)
	case timerFiredEvent:
		if y.advance > 0 {
			advance := y.advance.String()
			return encode("!timer-fired", timerFiredYAML{Advance: &advance})
		}
		return encode("!timer-fired", timerFiredYAML{})
	case batchOutcomeEvent:
		return encode("!batch-outcome", batchOutcomeYAML{ID: uint64(ev.id), Success: ev.err == nil})
	case stopEvent:
		return encode("!stop", map[string]any{})
	default:
		return nil, fmt.Errorf("unsupported event type %T", ev)
	}
}

// UnmarshalYAML decodes an event using its YAML tag. It performs minimal
// validation and records any explicit virtual-time advance found in the YAML
// body.
func (y *yamlBatchEvent) UnmarshalYAML(node *yaml.Node) error {
	tag := strings.TrimPrefix(node.Tag, "!")
	switch tag {
	case "enqueue":
		var v enqueueYAML
		if err := node.Decode(&v); err != nil {
			return err
		}
		y.event = enqueueEvent{data: json.RawMessage([]byte(fmt.Sprintf("\"%s\"", v.Value)))}
		y.advance = 0
	case "enqueue-bytes":
		var v enqueueBytesYAML
		if err := node.Decode(&v); err != nil {
			return err
		}
		raw := bytes.Repeat([]byte{'x'}, v.Size)
		quoted := append([]byte{'"'}, append(raw, '"')...)
		y.event = enqueueEvent{data: json.RawMessage(quoted)}
		y.advance = 0
	case "timer-fired":
		var v timerFiredYAML
		if err := node.Decode(&v); err != nil {
			return err
		}
		if v.Advance != nil {
			var err error
			y.advance, err = time.ParseDuration(*v.Advance)
			if err != nil {
				return err
			}
		}

		y.event = timerFiredEvent{}
	case "batch-outcome":
		var v batchOutcomeYAML
		if err := node.Decode(&v); err != nil {
			return err
		}
		var err error
		if !v.Success {
			err = fmt.Errorf("failed")
		}
		y.event = batchOutcomeEvent{id: batchID(v.ID), err: err}
		y.advance = 0
	case "stop":
		y.event = stopEvent{}
		y.advance = 0
	default:
		return fmt.Errorf("unknown event tag %s", tag)
	}
	return nil
}

// capturedEvent is the internal representation used by the snapshot runner.
// It contains the parsed event, the corresponding YAML node (for pretty
// printing), and the virtual-time advance that must happen before the event.
type capturedEvent struct {
	node    *yaml.Node
	event   event
	advance time.Duration // how much to advance virtual time before the event
}

type snapshotEffects struct {
	now        time.Duration // virtual time when effects recorded
	prevResetP *int          // pointer to last reset abs ms value (ms)
	nodes      []*yaml.Node
	nextFlush  *time.Time
}

func (se *snapshotEffects) sendBatch(id batchID, batch []json.RawMessage) {
	if len(batch) == 0 {
		return // nothing to record
	}
	n := &yaml.Node{Tag: "!send-batch", Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	n.Content = []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "id"}, {Kind: yaml.ScalarNode, Value: fmt.Sprintf("%d", id)},
		{Kind: yaml.ScalarNode, Value: "items"}, {Kind: yaml.ScalarNode, Value: fmt.Sprintf("%d", len(batch))},
		{Kind: yaml.ScalarNode, Value: "bytes"}, {Kind: yaml.ScalarNode, Value: fmt.Sprintf("%d", batchSize(batch))},
	}
	se.nodes = append(se.nodes, n)
}

func batchSize(b []json.RawMessage) int {
	var n int
	for _, m := range b {
		n += len(m)
	}
	return n
}

func (se *snapshotEffects) resetTimer(ts time.Time) {

	absMs := int(ts.Sub(startTime).Milliseconds())
	prev := *se.prevResetP
	deltaMs := absMs - prev
	*se.prevResetP = absMs
	n := &yaml.Node{Tag: "!reset-timer", Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	n.Content = []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "ts"}, {Kind: yaml.ScalarNode, Value: fmt.Sprintf("%dms (%+dms)", absMs, deltaMs)},
	}
	se.nodes = append(se.nodes, n)
	*se.nextFlush = ts
}

func (se *snapshotEffects) clearTimer() {
	// Emit an empty mapping under the !reset-timer tag because the timer has been stopped.
	*se.prevResetP = 0
	n := &yaml.Node{Tag: "!reset-timer", Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	// The mapping is intentionally left empty.
	se.nodes = append(se.nodes, n)
	*se.nextFlush = time.Time{}
}

func parseBatcherEvents(nodes []*yaml.Node) ([]capturedEvent, error) {
	var events []capturedEvent
	for _, n := range nodes {
		var ybe yamlBatchEvent
		if err := n.Decode(&ybe); err != nil {
			return nil, err
		}
		events = append(events, capturedEvent{node: n, event: ybe.event, advance: ybe.advance})
	}
	return events, nil
}

// buildOutputDoc constructs the YAML document for one step.
func buildOutputDoc(
	eventNode *yaml.Node,
	effectNodes []*yaml.Node,
	s *batcherState,
	lastMetrics map[string]int64,
	virtualNow time.Duration,
	nextFlush time.Time,
) []byte {
	var doc yaml.Node
	doc.Kind = yaml.DocumentNode
	mapping := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "now"},
		{Kind: yaml.ScalarNode, Value: virtualNow.String()},
	}}
	doc.Content = []*yaml.Node{mapping}
	if !nextFlush.IsZero() {
		mapping.Content = append(
			mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "next_flush"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: nextFlush.Sub(startTime).String()},
		)
	}

	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "event"}, eventNode)
	if len(effectNodes) > 0 {
		effectsSeq := &yaml.Node{Kind: yaml.SequenceNode}
		effectsSeq.Content = effectNodes
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "effects"}, effectsSeq)
	}

	// state (batch length + deadlines)
	stateMap := map[string]any{}
	if len(s.buffer) > 0 {
		stateMap["batch_len"] = len(s.buffer)
	} else {
		stateMap["batch_len"] = 0
	}
	stateMap["current_size"] = s.bufferBytes
	stateMap["timer_set"] = s.timerSet
	if len(s.inFlight) > 0 {
		var ids []int
		for id := range s.inFlight {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		stateMap["inflight"] = ids
	}
	if len(stateMap) > 0 {
		var n yaml.Node
		_ = n.Encode(stateMap)
		// Switch the inflight sequence to flow style so that it is rendered on a single line.
		if len(s.inFlight) > 0 {
			// n is the mapping node produced by Encode above. Walk its content to find
			// the "inflight" key and tweak its associated sequence node style.
			for i := 0; i < len(n.Content); i += 2 {
				keyNode, valNode := n.Content[i], n.Content[i+1]
				if keyNode.Value == "inflight" && valNode.Kind == yaml.SequenceNode {
					valNode.Style = yaml.FlowStyle
					break
				}
			}
		}
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "state"}, &n)
	}

	// Metrics diff:
	cur := s.metrics.Stats()
	changed := map[string]string{}
	for k, v := range cur {
		delta := v - lastMetrics[k]
		if delta != 0 {
			changed[k] = fmt.Sprintf("%d (+%d)", v, delta)
		}
	}
	if len(changed) > 0 {
		var n yaml.Node
		_ = n.Encode(changed)
		mapping.Content = append(
			mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "metrics"},
			&n,
		)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(&doc)
	_ = enc.Close()
	return buf.Bytes()
}

// splitYAMLDocuments copied (simplified) from actuator tests
func splitYAMLDocuments(content []byte) ([][]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
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

func findChildByKey(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
