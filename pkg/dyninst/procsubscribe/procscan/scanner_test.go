// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"bufio"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
	"github.com/stretchr/testify/require"

	tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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
	t              *testing.T
	scanner        *Scanner
	currentTime    ticks
	processes      map[int32]*testProcess
	goBinaries     map[string]bool
	lastCommand    command
	lastScanResult *scanResult
}

type testProcess struct {
	pid                 int32
	startTime           ticks
	metadataAvailableAt ticks
	metadataError       error
	executable          process.Executable
	executableResolves  bool
	tracerMetadata      tracermetadata.TracerMetadata
}

func newScannerTestState(t *testing.T) *scannerTestState {
	return &scannerTestState{
		t:          t,
		processes:  make(map[int32]*testProcess),
		goBinaries: make(map[string]bool),
	}
}

// testEpoch anchors the tick-denominated timelines to the wall clock that the
// scanner's retry bookkeeping runs on.
var testEpoch = time.Unix(0, 0).UTC()

// ticksToDuration converts the tick-denominated durations used by the test
// timelines into the durations that the constructor takes.
func ticksToDuration(t uint64) time.Duration {
	return time.Duration(t) * time.Second / clkTck
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

// executableInput describes the binary a test process runs. Processes sharing a
// path and inode share a cache entry in the scanner's executable filter.
type executableInput struct {
	Path string `yaml:"path,omitempty"`
	Ino  uint64 `yaml:"ino,omitempty"`
	// GoBinary defaults to true.
	GoBinary *bool `yaml:"go_binary,omitempty"`
	// Unresolvable models a process whose executable cannot be identified,
	// which is what every kernel thread is.
	Unresolvable bool `yaml:"unresolvable,omitempty"`
}

type createProcessCommand struct {
	PID                 int32               `yaml:"pid"`
	StartTime           uint64              `yaml:"start_time"`
	MetadataAvailableAt *uint64             `yaml:"metadata_available_at,omitempty"`
	MetadataError       string              `yaml:"metadata_error,omitempty"`
	Executable          executableInput     `yaml:"executable,omitempty"`
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
	var metadataError error
	switch c.MetadataError {
	case "":
	case "permission":
		metadataError = fmt.Errorf("open fd dir: %w", fs.ErrPermission)
	default:
		return fmt.Errorf("unknown metadata_error %q", c.MetadataError)
	}
	exe := ts.declareExecutable(c.PID, c.Executable)
	ts.processes[c.PID] = &testProcess{
		pid:                 c.PID,
		startTime:           ticks(c.StartTime),
		metadataAvailableAt: metadataAvailableAt,
		metadataError:       metadataError,
		executable:          exe,
		executableResolves:  !c.Executable.Unresolvable,
		tracerMetadata:      c.TracerMetadata.toTracerMetadata(),
	}
	return nil
}

// declareExecutable fills in the defaults for an executable a timeline
// describes and records whether it is a Go binary. Distinct processes sharing a
// path and inode share a cache entry in the scanner's executable filter.
func (ts *scannerTestState) declareExecutable(
	pid int32, in executableInput,
) process.Executable {
	exe := process.Executable{
		Path: in.Path,
		Key:  process.FileKey{FileHandle: process.FileHandle{Ino: in.Ino}},
	}
	if exe.Path == "" {
		exe.Path = fmt.Sprintf("/proc/%d/exe", pid)
	}
	if exe.Key.Ino == 0 {
		exe.Key.Ino = uint64(pid)
	}
	ts.goBinaries[exe.Path] = in.GoBinary == nil || *in.GoBinary
	return exe
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

// initializeCommand builds a fresh Scanner. Issuing it a second time in a
// timeline models an agent restart: the processes stay, the scanner state does
// not.
//
// All of the durations are in ticks to match the timelines.
type initializeCommand struct {
	CurrentTime uint64 `yaml:"current_time"`
	BackoffBase uint64 `yaml:"backoff_base"`
	BackoffCap  uint64 `yaml:"backoff_cap"`
}

func (c *initializeCommand) execute(
	_ *testing.T,
	ts *scannerTestState,
) error {
	ts.currentTime = ticks(c.CurrentTime)
	ts.scanner = newScanner(
		ticksToDuration(c.BackoffBase), ticksToDuration(c.BackoffCap),
		ts.now,
		ts.listPids,
		ts.readStartTime,
		ts.readTracerMetadata,
		ts.resolveExecutable,
		ts.checkGoExecutable,
	)
	return nil
}

type scanCommand struct{}

func (c *scanCommand) execute(_ *testing.T, ts *scannerTestState) error {
	if ts.scanner == nil {
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

func (ts *scannerTestState) now() time.Time {
	return testEpoch.Add(ticksToDuration(uint64(ts.currentTime)))
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
			"process %d does not exist: %w", pid, fs.ErrNotExist,
		)
	}
	if proc.metadataError != nil {
		return tracermetadata.TracerMetadata{}, proc.metadataError
	}
	// Metadata is only available after metadataAvailableAt.
	if ts.currentTime < proc.metadataAvailableAt {
		return tracermetadata.TracerMetadata{}, fmt.Errorf(
			"process %d: %w", pid, kernel.ErrMemFdFileNotFound,
		)
	}
	return proc.tracerMetadata, nil
}

func (ts *scannerTestState) resolveExecutable(
	pid int32,
) (process.Executable, error) {
	proc, ok := ts.processes[pid]
	if !ok || !proc.executableResolves {
		return process.Executable{}, fmt.Errorf(
			"cannot read the executable of process %d: %w", pid, fs.ErrNotExist,
		)
	}
	return proc.executable, nil
}

func (ts *scannerTestState) checkGoExecutable(path string) (bool, error) {
	return ts.goBinaries[path], nil
}

// procSnapshot identifies a process the way the scanner does.
type procSnapshot struct {
	PID       uint32 `yaml:"pid"`
	StartTime uint64 `yaml:"start_time"`
}

// candidateSnapshot exposes the retry schedule of a process that is not
// instrumented yet.
type candidateSnapshot struct {
	PID         uint32 `yaml:"pid"`
	StartTime   uint64 `yaml:"start_time"`
	Attempts    uint32 `yaml:"attempts,omitempty"`
	NextAttempt uint64 `yaml:"next_attempt,omitempty"`
}

type scannerStateSnapshot struct {
	CurrentTime       uint64              `yaml:"current_time"`
	Live              []procSnapshot      `yaml:"live,omitempty,flow"`
	Candidates        []candidateSnapshot `yaml:"candidates,omitempty,flow"`
	ProcessesInProcfs []int32             `yaml:"processes_in_procfs,omitempty,flow"`
}

// Output structures for test commands.
type commandOutput struct {
	Command ast.Node `yaml:"command"`
}

type scanOutput struct {
	Command ast.Node             `yaml:"command"`
	New     []procSnapshot       `yaml:"new,omitempty,flow"`
	Removed []int                `yaml:"removed,omitempty,flow"`
	State   scannerStateSnapshot `yaml:"state"`
}

func (ts *scannerTestState) cloneState() *scannerStateSnapshot {
	s := ts.scanner
	s.mu.Lock()
	defer s.mu.Unlock()
	live := make([]procSnapshot, 0, len(s.mu.live))
	for pid, startTime := range s.mu.live {
		live = append(live, procSnapshot{
			PID: pid, StartTime: uint64(startTime),
		})
	}
	slices.SortFunc(live, func(a, b procSnapshot) int {
		return cmp.Compare(a.PID, b.PID)
	})

	candidates := make([]candidateSnapshot, 0, len(s.candidates))
	for pid, c := range s.candidates {
		candidates = append(candidates, candidateSnapshot{
			PID:         pid,
			StartTime:   uint64(c.startTime),
			Attempts:    c.attempts,
			NextAttempt: uint64(durationToTicks(c.nextAttempt.Sub(testEpoch))),
		})
	}
	slices.SortFunc(candidates, func(a, b candidateSnapshot) int {
		return cmp.Compare(a.PID, b.PID)
	})

	pids := make([]int32, 0, len(ts.processes))
	for pid := range ts.processes {
		pids = append(pids, pid)
	}
	slices.Sort(pids)

	return &scannerStateSnapshot{
		CurrentTime:       uint64(ts.currentTime),
		Live:              live,
		Candidates:        candidates,
		ProcessesInProcfs: pids,
	}
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
			State:   *ts.cloneState(),
		}

		if len(ts.lastScanResult.New) > 0 {
			scanOut.New = make([]procSnapshot, 0, len(ts.lastScanResult.New))
			for _, dp := range ts.lastScanResult.New {
				scanOut.New = append(scanOut.New, procSnapshot{
					PID: dp.PID, StartTime: dp.StartTimeTicks,
				})
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
			// Scan reports exits in map order; sort for a stable snapshot.
			slices.Sort(scanOut.Removed)
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
