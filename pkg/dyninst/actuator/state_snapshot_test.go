// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// stateUpdate is a struct that represents the changes to the state after an
// event is processed. It's used to generate the output document.
type stateUpdate struct {
	CurrentlyLoading string         `yaml:"currently_loading,omitempty"`
	QueuedPrograms   string         `yaml:"queued_programs,omitempty"`
	Processes        map[any]string `yaml:"processes,omitempty"`
	Programs         map[int]string `yaml:"programs,omitempty"`
}

func TestSnapshot(t *testing.T) {
	rewrite := false
	if rewriteEnv := os.Getenv("REWRITE"); rewriteEnv != "" {
		if r, err := strconv.ParseBool(rewriteEnv); err == nil && r {
			rewrite = true
		}
	}

	snapshotDir := "testdata/snapshot"
	files, err := filepath.Glob(filepath.Join(snapshotDir, "*.yaml"))
	require.NoError(t, err, "failed to find snapshot files")

	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasPrefix(base, ".") {
			continue
		}
		name := strings.TrimSuffix(base, ".yaml")
		t.Run(name, func(t *testing.T) {
			runSnapshotTest(t, file, rewrite)
		})
	}
}

func runSnapshotTest(t *testing.T, file string, rewrite bool) {
	content, err := os.ReadFile(file)
	require.NoError(t, err, "failed to read snapshot file")

	// Split file into document chunks (first = events, rest = output documents).
	documentChunks, err := splitYAMLDocuments(content)
	require.NoError(t, err, "failed to split YAML documents")
	require.Greater(
		t, len(documentChunks), 0,
		"file must contain at least the events document",
	)

	// Parse only the first document (events) - never deserialize output
	// documents. This is a sanity check to ensure that the file is valid.
	input := documentChunks[0]
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	var eventsNode yaml.Node
	err = decoder.Decode(&eventsNode)
	require.NoError(t, err, "failed to decode events list node")

	// Extract events.
	events, eventNodes, err := parseEventsFromNode(&eventsNode)
	require.NoError(t, err, "failed to parse events")

	// Initialize test state
	expected := documentChunks[1:]
	output := make([][]byte, len(events))
	defer func() {
		if t.Failed() {
			for i, doc := range output {
				t.Logf("output[%d]:\n%s\n---\n", i, string(doc))
			}
		}
	}()

	// Process each event
	s := newState()
	effects := effectRecorder{}
	for i, ev := range events {

		// Create snapshot before handling event
		before := deepCopyState(s)
		if loaded, ok := ev.event.(eventProgramLoaded); ok {
			closeSink := &closeEffectRecorderSink{
				r:         &effects,
				programID: loaded.programID,
			}
			loaded.loaded.sink = closeSink
			ev.event = loaded
		}

		// Handle the event
		effects.effects = effects.effects[:0]
		err = handleEvent(s, &effects, ev.event)
		require.NoError(t, err)
		output[i] = generateEventOutput(t, eventNodes[i], effects, before, s)
		outputString := string(output[i])
		validateState(s, func(err error) {
			t.Errorf("validation failed: %v", err)
		})
		if t.Failed() {
			return
		}
		if rewrite {
			continue
		}

		require.Greater(
			t, len(expected), i, "missing expected output document",
		)
		require.Equal(t, string(expected[i]), outputString)
	}

	if rewrite {
		// Generate complete file: events document + all output documents.
		dir := filepath.Dir(file)
		tmpFile, err := os.CreateTemp(
			dir, fmt.Sprintf(".%s-*.yaml", filepath.Base(file)),
		)
		require.NoError(t, err, "failed to create temporary file")
		defer os.Remove(tmpFile.Name())
		w := bufio.NewWriter(tmpFile)
		write := func(b []byte) {
			_, err := w.Write(b)
			require.NoError(t, err)
		}
		write(input)
		for _, doc := range output {
			write([]byte("---\n"))
			write(doc)
		}
		err = w.Flush()
		require.NoError(t, err)
		err = tmpFile.Close()
		require.NoError(t, err)
		err = os.Rename(tmpFile.Name(), file)
		require.NoError(t, err)
	}
}

func generateEventOutput(
	t *testing.T,
	event *yaml.Node,
	effects effectRecorder,
	before, after *state,
) []byte {
	var buf bytes.Buffer

	outputDoc := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode,
		}},
	}

	// Create content manually to ensure proper multiline formatting.
	var content []*yaml.Node
	content = append(content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "event"},
		event,
	)

	effectNodes, err := effects.yamlNodes()
	require.NoError(t, err)
	if len(effectNodes) > 0 {
		var effectsNode yaml.Node
		require.NoError(t, effectsNode.Encode(effectNodes))
		content = append(content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "effects"},
			&effectsNode,
		)
	}

	update := computeStateUpdate(before, after)
	stateNode := &yaml.Node{}
	require.NoError(t, stateNode.Encode(update))
	content = append(content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "state"},
		stateNode,
	)

	outputDoc.Content[0].Content = content

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	require.NoError(t, encoder.Encode(outputDoc))
	require.NoError(t, encoder.Close())

	return buf.Bytes()
}

// computeStateUpdate compares two states and returns state update data showing
// changes.
func computeStateUpdate(before, after *state) *stateUpdate {
	update := &stateUpdate{}

	{
		var beforeLoading, afterLoading any
		if before.currentlyLoading != nil {
			beforeLoading = int(before.currentlyLoading.id)
		}
		if after.currentlyLoading != nil {
			afterLoading = int(after.currentlyLoading.id)
		}
		if beforeLoading != afterLoading {
			update.CurrentlyLoading = fmt.Sprintf(
				"%v -> %v", beforeLoading, afterLoading,
			)
		} else {
			update.CurrentlyLoading = fmt.Sprintf("%v", afterLoading)
		}
	}
	getQueuedProgramIDs := func(s *state) []int {
		var ids []int
		for p := range s.queuedLoading.items() {
			ids = append(ids, int(p.id))
		}
		return ids
	}
	{
		beforeQueued := getQueuedProgramIDs(before)
		afterQueued := getQueuedProgramIDs(after)
		if !slices.Equal(beforeQueued, afterQueued) {
			transition := fmt.Sprintf("%v -> %v", beforeQueued, afterQueued)
			update.QueuedPrograms = transition
		} else {
			current := fmt.Sprintf("%v", afterQueued)
			update.QueuedPrograms = current
		}
	}
	{
		before, after := before.processes, after.processes
		allIDs := make(map[processKey]bool)
		for id := range before {
			allIDs[id] = true
		}
		for id := range after {
			allIDs[id] = true
		}

		for id := range allIDs {
			beforeProc := before[id]
			afterProc := after[id]
			var key any
			if id.tenantID != 0 {
				key = fmt.Sprintf("t%d:%d", id.tenantID, id.PID)
			} else {
				key = int(id.PID)
			}

			var beforeState, afterState any
			if beforeProc != nil {
				if beforeProc.currentProgram != 0 {
					beforeState = fmt.Sprintf(
						"%s (prog %d)",
						beforeProc.state.String(), beforeProc.currentProgram,
					)
				} else {
					beforeState = beforeProc.state.String()
				}
			}
			if afterProc != nil {
				if afterProc.currentProgram != 0 {
					afterState = fmt.Sprintf(
						"%s (prog %d)",
						afterProc.state.String(), afterProc.currentProgram,
					)
				} else {
					afterState = afterProc.state.String()
				}
			}
			if update.Processes == nil {
				update.Processes = make(map[any]string)
			}
			if beforeState != afterState {
				update.Processes[key] = fmt.Sprintf(
					"%v -> %v", beforeState, afterState,
				)
			} else {
				update.Processes[key] = fmt.Sprintf("%v", afterState)
			}
		}
	}

	{
		before, after := before.programs, after.programs
		allIDs := make(map[ir.ProgramID]bool)
		for id := range before {
			allIDs[id] = true
		}
		for id := range after {
			allIDs[id] = true
		}

		for id := range allIDs {
			beforeProg := before[id]
			afterProg := after[id]
			key := int(id)

			var beforeState, afterState any

			if beforeProg != nil {
				beforeState = fmt.Sprintf(
					"%s (proc %d)",
					beforeProg.state.String(), beforeProg.PID,
				)
			}
			if afterProg != nil {
				afterState = fmt.Sprintf(
					"%s (proc %d)",
					afterProg.state.String(), afterProg.PID,
				)
			}

			if update.Programs == nil {
				update.Programs = make(map[int]string)
			}

			if beforeState != afterState {
				update.Programs[key] = fmt.Sprintf(
					"%v -> %v", beforeState, afterState,
				)
			} else {
				update.Programs[key] = fmt.Sprintf("%v", afterState)
			}
		}

	}

	return update
}

func clearComments(node *yaml.Node) {
	node.FootComment = ""
	node.HeadComment = ""
	node.LineComment = ""
	for _, child := range node.Content {
		if child != nil {
			clearComments(child)
		}
	}
}

func parseEventsFromNode(
	eventsNode *yaml.Node,
) ([]yamlEvent, []*yaml.Node, error) {
	if eventsNode.Kind != yaml.DocumentNode || len(eventsNode.Content) != 1 {
		return nil, nil, fmt.Errorf(
			"expected document with single content node",
		)
	}

	listNode := eventsNode.Content[0]
	if listNode.Kind != yaml.SequenceNode {
		return nil, nil, fmt.Errorf("expected sequence node for events list")
	}

	events := make([]yamlEvent, len(listNode.Content))
	for i, eventNode := range listNode.Content {
		clearComments(eventNode)
		if err := eventNode.Decode(&events[i]); err != nil {
			return nil, nil, fmt.Errorf("failed to decode event: %w", err)
		}
	}

	return events, listNode.Content, nil
}

func splitYAMLDocuments(content []byte) ([][]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Split(bufio.ScanLines)

	var documents [][]byte
	var currentDocument []byte
	for scanner.Scan() {
		if bytes.HasPrefix(scanner.Bytes(), []byte("---")) {
			documents = append(documents, currentDocument)
			currentDocument = nil
		} else {
			currentDocument = append(currentDocument, scanner.Bytes()...)
			currentDocument = append(currentDocument, '\n')
		}
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	if len(currentDocument) > 0 {
		documents = append(documents, currentDocument)
	}
	return documents, nil
}
