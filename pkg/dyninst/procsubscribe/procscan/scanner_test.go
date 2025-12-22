// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
)

// TestScannerSnapshot runs snapshot tests for the Scanner using YAML-defined
// command sequences.
func TestScannerSnapshot(t *testing.T) {
	rewrite := false
	if rewriteEnv := os.Getenv("REWRITE"); rewriteEnv != "" {
		if r, err := strconv.ParseBool(rewriteEnv); err == nil && r {
			rewrite = true
		}
	}

	snapshotDir := "testdata/scanner"
	files, err := filepath.Glob(filepath.Join(snapshotDir, "*.yaml"))
	require.NoError(t, err, "failed to find snapshot files")

	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasPrefix(base, ".") {
			continue
		}
		name := strings.TrimSuffix(base, ".yaml")
		t.Run(name, func(t *testing.T) {
			runScannerSnapshotTest(t, file, rewrite)
		})
	}
}

func runScannerSnapshotTest(t *testing.T, file string, rewrite bool) {
	content, err := os.ReadFile(file)
	require.NoError(t, err, "failed to read snapshot file")

	// Split file into document chunks (first = commands, rest = output
	// documents).
	documentChunks, err := splitYAMLDocuments(content)
	require.NoError(t, err, "failed to split YAML documents")
	require.Greater(
		t, len(documentChunks), 0,
		"file must contain at least the commands document",
	)

	// Parse commands from first document using AST to preserve nodes.
	input := documentChunks[0]
	tokens := lexer.Tokenize(string(input))
	astFile, err := parser.Parse(tokens, 0)
	require.NoError(t, err, "failed to parse commands")

	commands, commandNodes, err := parseCommandsFromAST(astFile)
	require.NoError(t, err, "failed to extract commands")

	// Initialize test state.
	expected := documentChunks[1:]
	outputs := make([][]byte, len(commands))
	defer func() {
		if t.Failed() {
			for i, doc := range outputs {
				t.Logf("output[%d]:\n%s\n---\n", i, string(doc))
			}
		}
	}()

	// Process each command.
	testState := newScannerTestState(t)
	for i, cmd := range commands {
		testState.lastCommand = cmd
		testState.lastScanResult = nil
		err = cmd.execute(t, testState)
		require.NoError(t, err, "failed to execute command %d: %v", i, cmd)

		outputs[i] = testState.generateOutput(t, commandNodes[i])

		if rewrite {
			continue
		}

		require.Greater(
			t, len(expected), i, "missing expected output document",
		)
		require.Equal(t, string(expected[i]), string(outputs[i]))
	}

	if rewrite {
		// Generate complete file: commands document + all output documents.
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
		for _, doc := range outputs {
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

// scannerTestState manages the test state for scanner tests.
type scannerTestState struct {
	t                  *testing.T
	scanner            *Scanner
	currentTime        ticks
	processes          map[int32]*testProcess
	executables        map[int32]process.Executable
	processDelaysTicks []ticks
	lastCommand        command
	lastScanResult     *scanResult
	initialized        bool
	firstOutput        bool
}

type testProcess struct {
	pid                 int32
	startTime           ticks
	metadataAvailableAt ticks
	tracerMetadata      tracermetadata.TracerMetadata
}

func newScannerTestState(t *testing.T) *scannerTestState {
	const defaultProcessDelay = 100 // 100 ticks
	ts := &scannerTestState{
		t:                  t,
		currentTime:        0,
		processes:          make(map[int32]*testProcess),
		executables:        make(map[int32]process.Executable),
		processDelaysTicks: []ticks{defaultProcessDelay},
		firstOutput:        true,
	}

	ts.scanner = newScanner(
		[]timeWindow{{startDelay: defaultProcessDelay}},
		func() (ticks, error) { return ts.currentTime, nil },
		ts.listPids,
		ts.readStartTime,
		ts.readTracerMetadata,
		ts.resolveExecutable,
	)

	return ts
}

// command is the interface that all test commands implement.
type command interface {
	execute(t *testing.T, ts *scannerTestState) error
}

// Reusable structures for YAML parsing.
type tracerMetadataInput struct {
	SchemaVersion uint8  `yaml:"schema_version"`
	RuntimeID     string `yaml:"runtime_id"`
	Language      string `yaml:"language"`
	TracerVersion string `yaml:"tracer_version"`
	Hostname      string `yaml:"hostname"`
	Service       string `yaml:"service"`
	Env           string `yaml:"env"`
	Version       string `yaml:"version"`
	ProcessTags   string `yaml:"process_tags"`
	ContainerID   string `yaml:"container_id"`
}

func (t *tracerMetadataInput) toTracerMetadata() tracermetadata.TracerMetadata {
	return tracermetadata.TracerMetadata{
		SchemaVersion:  t.SchemaVersion,
		RuntimeID:      t.RuntimeID,
		TracerLanguage: t.Language,
		TracerVersion:  t.TracerVersion,
		Hostname:       t.Hostname,
		ServiceName:    t.Service,
		ServiceEnv:     t.Env,
		ServiceVersion: t.Version,
		ProcessTags:    t.ProcessTags,
		ContainerID:    t.ContainerID,
	}
}

type createProcessCommand struct {
	PID                 int32               `yaml:"pid"`
	StartTime           uint64              `yaml:"start_time"`
	MetadataAvailableAt *uint64             `yaml:"metadata_available_at,omitempty"`
	TracerMetadata      tracerMetadataInput `yaml:"tracer_metadata"`
}

func (c *createProcessCommand) execute(
	_ *testing.T,
	ts *scannerTestState,
) error {
	if _, exists := ts.processes[c.PID]; exists {
		return fmt.Errorf("process %d already exists", c.PID)
	}
	metadataAvailableAt := ticks(c.StartTime)
	if c.MetadataAvailableAt != nil {
		metadataAvailableAt = ticks(*c.MetadataAvailableAt)
	}
	ts.processes[c.PID] = &testProcess{
		pid:                 c.PID,
		startTime:           ticks(c.StartTime),
		metadataAvailableAt: metadataAvailableAt,
		tracerMetadata:      c.TracerMetadata.toTracerMetadata(),
	}
	ts.executables[c.PID] = process.Executable{
		Path: fmt.Sprintf("/proc/%d/exe", c.PID),
	}
	return nil
}

type removeProcessCommand struct {
	PID int32 `yaml:"pid"`
}

func (c *removeProcessCommand) execute(
	_ *testing.T,
	ts *scannerTestState,
) error {
	if _, exists := ts.processes[c.PID]; !exists {
		return fmt.Errorf("process %d does not exist", c.PID)
	}
	delete(ts.processes, c.PID)
	delete(ts.executables, c.PID)
	return nil
}

type advanceTimeCommand struct {
	To uint64 `yaml:"to"`
	By uint64 `yaml:"by"`
}

func (c *advanceTimeCommand) execute(
	_ *testing.T,
	ts *scannerTestState,
) error {
	if c.To > 0 {
		ts.currentTime = ticks(c.To)
	} else if c.By > 0 {
		ts.currentTime += ticks(c.By)
	}
	return nil
}

type initializeCommand struct {
	CurrentTime   uint64   `yaml:"current_time"`
	ProcessDelays []uint64 `yaml:"process_delays"`
}

func (c *initializeCommand) execute(
	_ *testing.T,
	ts *scannerTestState,
) error {
	ts.currentTime = ticks(c.CurrentTime)

	// Build time windows from process delays.
	ts.processDelaysTicks = make([]ticks, len(c.ProcessDelays))
	windows := make([]timeWindow, len(c.ProcessDelays))
	for i, delay := range c.ProcessDelays {
		ts.processDelaysTicks[i] = ticks(delay)
		windows[i] = timeWindow{startDelay: ticks(delay)}
	}
	ts.scanner.windows = windows

	ts.initialized = true
	return nil
}

type scanCommand struct{}

func (c *scanCommand) execute(_ *testing.T, ts *scannerTestState) error {
	if !ts.initialized {
		return errors.New(
			"scanner not initialized: use !initialize command first",
		)
	}
	discovered, removed, err := ts.scanner.Scan()
	if err != nil {
		return err
	}
	ts.lastScanResult = &scanResult{
		New:     discovered,
		Removed: removed,
	}
	return nil
}

func (ts *scannerTestState) listPids() iter.Seq2[uint32, error] {
	return func(yield func(uint32, error) bool) {
		pids := make([]int32, 0, len(ts.processes))
		for pid := range ts.processes {
			pids = append(pids, pid)
		}
		slices.Sort(pids)
		for _, pid := range pids {
			if !yield(uint32(pid), nil) {
				return
			}
		}
	}
}

func (ts *scannerTestState) readStartTime(pid int32) (ticks, error) {
	proc, ok := ts.processes[pid]
	if !ok {
		return 0, fmt.Errorf("process %d does not exist", pid)
	}
	return proc.startTime, nil
}

func (ts *scannerTestState) readTracerMetadata(
	pid int32,
) (tracermetadata.TracerMetadata, error) {
	proc, ok := ts.processes[pid]
	if !ok {
		return tracermetadata.TracerMetadata{}, fmt.Errorf(
			"process %d does not exist", pid,
		)
	}
	// Metadata is only available after metadataAvailableAt.
	if ts.currentTime < proc.metadataAvailableAt {
		return tracermetadata.TracerMetadata{}, fmt.Errorf(
			"metadata not yet available for process %d", pid,
		)
	}
	return proc.tracerMetadata, nil
}

func (ts *scannerTestState) resolveExecutable(
	pid int32,
) (process.Executable, error) {
	exe, ok := ts.executables[pid]
	if !ok {
		return process.Executable{}, fmt.Errorf("process %d does not exist", pid)
	}
	return exe, nil
}

type scannerStateSnapshot struct {
	CurrentTime       uint64   `yaml:"current_time"`
	LastScan          uint64   `yaml:"last_scan"`
	ProcessDelays     []uint64 `yaml:"process_delays,omitempty,flow"`
	Live              []int32  `yaml:"live,omitempty,flow"`
	ProcessesInProcfs []int32  `yaml:"processes_in_procfs,omitempty,flow"`
}

// Output structures for test commands.
type commandOutput struct {
	Command ast.Node `yaml:"command"`
}

type scanOutput struct {
	Command ast.Node             `yaml:"command"`
	New     []int                `yaml:"new,omitempty,flow"`
	Removed []int                `yaml:"removed,omitempty,flow"`
	State   scannerStateSnapshot `yaml:"state"`
}

func (ts *scannerTestState) cloneState(
	includeProcessDelays bool,
) *scannerStateSnapshot {
	ts.scanner.mu.Lock()
	defer ts.scanner.mu.Unlock()
	live := make([]int32, 0)
	ts.scanner.mu.live.Ascend(func(pid uint32) bool {
		live = append(live, int32(pid))
		return true
	})
	slices.Sort(live)

	pids := make([]int32, 0, len(ts.processes))
	for pid := range ts.processes {
		pids = append(pids, pid)
	}
	slices.Sort(pids)

	snapshot := &scannerStateSnapshot{
		CurrentTime:       uint64(ts.currentTime),
		LastScan:          uint64(ts.scanner.lastScan),
		Live:              live,
		ProcessesInProcfs: pids,
	}

	if includeProcessDelays {
		snapshot.ProcessDelays = make([]uint64, len(ts.processDelaysTicks))
		for i, d := range ts.processDelaysTicks {
			snapshot.ProcessDelays[i] = uint64(d)
		}
	}

	return snapshot
}

func (ts *scannerTestState) generateOutput(
	t *testing.T,
	cmdNode ast.Node,
) []byte {
	var outputStruct any

	// For scan commands, output includes results and state.
	if ts.lastScanResult != nil {
		scanOut := scanOutput{
			Command: cmdNode,
			State:   *ts.cloneState(ts.firstOutput),
		}

		// Mark that we've generated the first output.
		if ts.firstOutput {
			ts.firstOutput = false
		}

		// Format new processes (just PIDs).
		if len(ts.lastScanResult.New) > 0 {
			scanOut.New = make([]int, 0, len(ts.lastScanResult.New))
			for _, dp := range ts.lastScanResult.New {
				scanOut.New = append(scanOut.New, int(dp.PID))
			}
		}

		// Format removed processes (just PIDs).
		if len(ts.lastScanResult.Removed) > 0 {
			scanOut.Removed = make(
				[]int,
				0,
				len(ts.lastScanResult.Removed),
			)
			for _, pid := range ts.lastScanResult.Removed {
				scanOut.Removed = append(scanOut.Removed, int(pid))
			}
		}

		outputStruct = scanOut
	} else {
		// For non-scan commands, just output the command.
		outputStruct = commandOutput{
			Command: cmdNode,
		}
	}

	bytes, err := yaml.MarshalWithOptions(
		outputStruct,
		yaml.Indent(2),
	)
	require.NoError(t, err)
	return bytes
}

type scanResult struct {
	New     []DiscoveredProcess
	Removed []ProcessID
}

func parseCommandsFromAST(
	file *ast.File,
) ([]command, []ast.Node, error) {
	if len(file.Docs) == 0 {
		return nil, nil, errors.New("no documents in file")
	}

	doc := file.Docs[0]
	if doc.Body == nil {
		return nil, nil, errors.New("empty document")
	}

	// The body should be a sequence.
	seq, ok := doc.Body.(*ast.SequenceNode)
	if !ok {
		return nil, nil, fmt.Errorf(
			"expected sequence node, got %T", doc.Body,
		)
	}

	commands := make([]command, 0, len(seq.Values))
	nodes := make([]ast.Node, 0, len(seq.Values))
	for _, val := range seq.Values {
		nodes = append(nodes, val)
		cmd, err := parseCommand(val)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode command: %w", err)
		}
		commands = append(commands, cmd)
	}

	return commands, nodes, nil
}

func parseCommand(node ast.Node) (command, error) {
	// Extract the command type from the YAML tag.
	var cmdType string

	// Tags in goccy/go-yaml are stored in tokens.
	// The tag token appears just before the content token.
	tok := node.GetToken()
	if tok != nil {
		// Check if this token itself is a tag.
		if tok.Type == token.TagType {
			cmdType = strings.TrimPrefix(tok.Value, "!")
		} else {
			// Walk backwards to find a tag token.
			for t := tok.Prev; t != nil; t = t.Prev {
				if t.Type == token.TagType {
					cmdType = strings.TrimPrefix(t.Value, "!")
					break
				}
				// Stop if we hit a sequence entry marker.
				if t.Type == token.SequenceEntryType {
					break
				}
			}
		}
	}

	if cmdType == "" {
		return nil, fmt.Errorf(
			"command missing type tag (token type: %v)", tok.Type,
		)
	}

	// Convert the AST node to a Go value, which strips the tag.
	var dataValue any
	if err := yaml.NodeToValue(node, &dataValue); err != nil {
		return nil, fmt.Errorf("failed to convert node to value: %w", err)
	}

	// Marshal the value to YAML bytes (now without the tag).
	dataBytes, err := yaml.Marshal(dataValue)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal node data: %w", err)
	}

	switch cmdType {
	case "create-process":
		var cmd createProcessCommand
		if err := yaml.Unmarshal(dataBytes, &cmd); err != nil {
			return nil, fmt.Errorf(
				"failed to decode create-process: %w", err,
			)
		}
		return &cmd, nil

	case "remove-process":
		var cmd removeProcessCommand
		if err := yaml.Unmarshal(dataBytes, &cmd); err != nil {
			return nil, fmt.Errorf(
				"failed to decode remove-process: %w", err,
			)
		}
		return &cmd, nil

	case "advance-time":
		var cmd advanceTimeCommand
		if err := yaml.Unmarshal(dataBytes, &cmd); err != nil {
			return nil, fmt.Errorf(
				"failed to decode advance-time: %w", err,
			)
		}
		return &cmd, nil

	case "scan":
		return &scanCommand{}, nil

	case "initialize":
		var cmd initializeCommand
		if err := yaml.Unmarshal(dataBytes, &cmd); err != nil {
			return nil, fmt.Errorf(
				"failed to decode initialize: %w", err,
			)
		}
		return &cmd, nil

	default:
		return nil, fmt.Errorf("unknown command type: %s", cmdType)
	}
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

// It verifies that Scan drives cache mark-and-sweep and avoids stale start
// times on PID reuse.
func TestScannerStartTimeCachePurgedOnMissingPIDAndPIDReuse(t *testing.T) {
	now := ticks(1000)

	type procInfo struct {
		startTime ticks
	}
	procs := map[int32]procInfo{
		// With delay=0 the window is [lastWatermark, now]. The first scan starts
		// at lastWatermark=0, so ensure the start time is <= now.
		1: {startTime: 900},
	}

	listPids := func() iter.Seq2[uint32, error] {
		return func(yield func(uint32, error) bool) {
			pids := make([]int32, 0, len(procs))
			for pid := range procs {
				pids = append(pids, pid)
			}
			slices.Sort(pids)
			for _, pid := range pids {
				if !yield(uint32(pid), nil) {
					return
				}
			}
		}
	}

	readCalls := 0
	readStartTime := func(pid int32) (ticks, error) {
		readCalls++
		info, ok := procs[pid]
		if !ok {
			return 0, fmt.Errorf("process %d does not exist", pid)
		}
		return info.startTime, nil
	}

	s := newScanner(
		[]timeWindow{{startDelay: 0}},
		func() (ticks, error) { return now, nil },
		listPids,
		readStartTime,
		func(int32) (tracermetadata.TracerMetadata, error) {
			return tracermetadata.TracerMetadata{TracerLanguage: "go"}, nil
		},
		func(pid int32) (process.Executable, error) {
			return process.Executable{Path: fmt.Sprintf("/proc/%d/exe", pid)}, nil
		},
	)
	s.startTimeCache.maxSize = 8

	discovered, removed, err := s.Scan()
	require.NoError(t, err)
	require.Empty(t, removed)
	require.Len(t, discovered, 1)
	require.Equal(t, uint32(1), discovered[0].PID)
	require.Equal(t, uint64(900), discovered[0].StartTimeTicks)

	// End-of-scan sweep should have kept the entry and cleared the seen marker.
	start, ok := s.startTimeCache.entries[1]
	require.True(t, ok)
	require.Equal(t, ticks(900), start)

	// Process exits (PID no longer present in procfs).
	delete(procs, 1)
	now = 1100
	discovered, removed, err = s.Scan()
	require.NoError(t, err)
	require.Empty(t, discovered)
	require.Equal(t, []ProcessID{1}, removed)
	_, ok = s.startTimeCache.entries[1]
	require.False(t, ok)

	// PID is reused. Cache must not resurrect a stale start time.
	procs[1] = procInfo{startTime: 1150}
	now = 1200
	discovered, removed, err = s.Scan()
	require.NoError(t, err)
	require.Empty(t, removed)
	require.Len(t, discovered, 1)
	require.Equal(t, uint32(1), discovered[0].PID)
	require.Equal(t, uint64(1150), discovered[0].StartTimeTicks)
	require.GreaterOrEqual(t, readCalls, 2)
}
