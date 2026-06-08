// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// newInferenceBackend creates an inference backend from a model file.
// Supported formats:
//   - .bin     → raw score lookup table (for testing/replay)
//   - .onnx    → ONNX Runtime (TODO: awaiting onnxruntime-go integration)
//   - .pt      → PyTorch checkpoint via Python subprocess scorer
//   - .scrappy → Native C inference engine (Q8_0 quantized, no Python dependency)
func newInferenceBackend(modelPath string, vocab *scrappyVocab, threshold float64) (scrappyInferenceBackend, error) {
	if strings.HasSuffix(modelPath, ".bin") {
		return newLookupBackend(modelPath)
	}
	if strings.HasSuffix(modelPath, ".onnx") {
		return newONNXRuntimeBackend(modelPath, vocab)
	}
	if strings.HasSuffix(modelPath, ".pt") {
		return newTorchBackend(modelPath, threshold)
	}
	if strings.HasSuffix(modelPath, ".scrappy") {
		return newNativeBackend(modelPath)
	}
	return nil, fmt.Errorf("unsupported model format: %s (expected .onnx, .pt, .scrappy, or .bin)", modelPath)
}

// --- ONNX Runtime backend (production path) ---

// onnxRuntimeBackend wraps ONNX Runtime for Go inference.
//
// The exported model has:
//   - Input:  "input_ids" int64 tensor (1, seq_len)
//   - Output: "logits" float32 tensor (1, seq_len, vocab_size)
//
// We extract logits at the tick_end position, softmax over [normal] and [alert]
// token IDs, and return P([alert]).
type onnxRuntimeBackend struct {
	modelPath string
	normalID  int
	alertID   int
	// TODO: ort.Session from onnxruntime-go
}

func newONNXRuntimeBackend(modelPath string, vocab *scrappyVocab) (*onnxRuntimeBackend, error) {
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model file: %w", err)
	}
	return &onnxRuntimeBackend{
		modelPath: modelPath,
		normalID:  vocab.encode(tokNormal),
		alertID:   vocab.encode(tokAlert),
	}, nil
}

func (b *onnxRuntimeBackend) Score(tokenIDs []int, tickEndPos int) (float64, error) {
	// TODO: implement real ONNX Runtime inference:
	//
	// 1. Create input tensor from tokenIDs (int64, shape [1, len(tokenIDs)])
	// 2. Run session.Run() to get logits tensor (float32, shape [1, seq_len, vocab_size])
	// 3. Extract logits[0][tickEndPos][b.normalID] and logits[0][tickEndPos][b.alertID]
	// 4. Softmax over those two values → P([alert])
	//
	// For now, return 0 (no alert) so the detector pipeline works end-to-end
	// while we finish training and exporting the model.
	return 0.0, nil
}

func (b *onnxRuntimeBackend) Close() error {
	return nil
}

// --- Lookup table backend (for testing/replay) ---

// lookupBackend loads pre-computed per-tick scores from a binary file.
// Format: sequence of float64 values in little-endian, one per tick.
// The detector calls Score() once per tick, consuming scores in order.
type lookupBackend struct {
	scores []float64
	cursor int
}

func newLookupBackend(path string) (*lookupBackend, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lookup scores: %w", err)
	}
	if len(data)%8 != 0 {
		return nil, fmt.Errorf("lookup file size %d is not a multiple of 8", len(data))
	}
	n := len(data) / 8
	scores := make([]float64, n)
	for i := range n {
		bits := binary.LittleEndian.Uint64(data[i*8 : (i+1)*8])
		scores[i] = math.Float64frombits(bits)
	}
	return &lookupBackend{scores: scores}, nil
}

func (b *lookupBackend) Score(_ []int, _ int) (float64, error) {
	if b.cursor >= len(b.scores) {
		return 0.0, nil // past end of lookup table
	}
	score := b.scores[b.cursor]
	b.cursor++
	return score, nil
}

func (b *lookupBackend) Close() error {
	return nil
}

// --- PyTorch subprocess backend (for checkpoint evaluation) ---

// torchBackend launches scrappy_scorer.py as a subprocess and communicates
// via stdin/stdout JSON lines. This enables direct evaluation of .pt
// checkpoints without an ONNX export step.
type torchBackend struct {
	cmd    *exec.Cmd
	stdin  *json.Encoder
	stdout *bufio.Scanner
	mu     sync.Mutex // serializes Score calls

	// LastSalience holds salience entries from the most recent Score call.
	// Non-nil only when the scorer returned salience (i.e. P(alert) >= threshold).
	LastSalience []torchSalienceEntry
}

type torchScoreRequest struct {
	TokenIDs  []int   `json:"token_ids,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
	Reset     bool    `json:"reset,omitempty"`
}

type torchSalienceEntry struct {
	TokenID int     `json:"token_id"`
	Weight  float64 `json:"weight"`
}

type torchScoreResponse struct {
	Score    float64              `json:"score"`
	Error    string               `json:"error,omitempty"`
	Ready    bool                 `json:"ready,omitempty"`
	Reset    bool                 `json:"reset,omitempty"`
	Salience []torchSalienceEntry `json:"salience,omitempty"`
}

func newTorchBackend(checkpointPath string, threshold float64) (*torchBackend, error) {
	if _, err := os.Stat(checkpointPath); err != nil {
		return nil, fmt.Errorf("checkpoint file: %w", err)
	}

	// Find the scorer script next to this Go source, or in the same dir as the checkpoint.
	scorerScript := findScorerScript(checkpointPath)
	if scorerScript == "" {
		return nil, fmt.Errorf("scrappy_scorer.py not found (looked next to checkpoint and in observer/impl)")
	}

	// Use the scrappy venv Python if available, otherwise system python3.
	pythonBin := findPython()

	cmd := exec.Command(pythonBin, scorerScript, checkpointPath,
		"--threshold", fmt.Sprintf("%.4f", threshold))
	cmd.Stderr = os.Stderr

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start scorer subprocess: %w", err)
	}

	encoder := json.NewEncoder(stdinPipe)
	scanner := bufio.NewScanner(stdoutPipe)
	// Allow large JSON lines (token sequences can be big).
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	// Wait for handshake.
	if !scanner.Scan() {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("scorer subprocess did not send handshake")
	}

	var handshake torchScoreResponse
	if err := json.Unmarshal(scanner.Bytes(), &handshake); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("parse handshake: %w", err)
	}
	if !handshake.Ready {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("scorer not ready: %s", handshake.Error)
	}

	fmt.Fprintf(os.Stderr, "scrappy_inference: torch backend ready (checkpoint=%s)\n", filepath.Base(checkpointPath))

	return &torchBackend{
		cmd:    cmd,
		stdin:  encoder,
		stdout: scanner,
	}, nil
}

// Score sends the tick's tokens to the stateful scorer subprocess.
// The scorer processes tokens incrementally via model.step(), maintaining
// SSM/SWA state across calls. Only the new tick tokens are sent — the
// tokenIDs parameter is the current tick only, not the full context.
// The tickEndPos parameter is ignored (kept for interface compatibility).
func (b *torchBackend) Score(tokenIDs []int, _ int) (float64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	req := torchScoreRequest{TokenIDs: tokenIDs}
	if err := b.stdin.Encode(req); err != nil {
		return 0, fmt.Errorf("send to scorer: %w", err)
	}

	if !b.stdout.Scan() {
		return 0, fmt.Errorf("scorer subprocess closed unexpectedly")
	}

	var resp torchScoreResponse
	if err := json.Unmarshal(b.stdout.Bytes(), &resp); err != nil {
		return 0, fmt.Errorf("parse scorer response: %w", err)
	}
	if resp.Error != "" {
		return 0, fmt.Errorf("scorer error: %s", resp.Error)
	}

	b.LastSalience = resp.Salience
	return resp.Score, nil
}

func (b *torchBackend) Close() error {
	if b.cmd != nil && b.cmd.Process != nil {
		// Close stdin to signal the scorer to exit gracefully.
		if closer, ok := b.cmd.Stdin.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		return b.cmd.Wait()
	}
	return nil
}

// --- Native C inference backend (.scrappy format) ---

// newNativeBackend launches the native scrappy-infer binary as a subprocess.
// Same JSON-RPC protocol as the torch backend — drop-in replacement.
func newNativeBackend(modelPath string) (*torchBackend, error) {
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model file: %w", err)
	}

	inferBin := findNativeBinary(modelPath)
	if inferBin == "" {
		return nil, fmt.Errorf("scrappy-infer binary not found (looked next to model and in scrappy/native/build)")
	}

	cmd := exec.Command(inferBin, modelPath)
	cmd.Stderr = os.Stderr

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start native inference: %w", err)
	}

	encoder := json.NewEncoder(stdinPipe)
	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	// Wait for handshake.
	if !scanner.Scan() {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("native binary did not send handshake")
	}

	var handshake torchScoreResponse
	if err := json.Unmarshal(scanner.Bytes(), &handshake); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("parse handshake: %w", err)
	}
	if !handshake.Ready {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("native binary not ready: %s", handshake.Error)
	}

	fmt.Fprintf(os.Stderr, "scrappy_inference: native backend ready (model=%s, binary=%s)\n",
		filepath.Base(modelPath), filepath.Base(inferBin))

	return &torchBackend{
		cmd:    cmd,
		stdin:  encoder,
		stdout: scanner,
	}, nil
}

// findNativeBinary locates the scrappy-infer binary.
func findNativeBinary(modelPath string) string {
	home := os.Getenv("HOME")
	candidates := []string{
		// Next to the model file.
		filepath.Join(filepath.Dir(modelPath), "scrappy-infer"),
		// In the scrappy native bin directory (release build).
		filepath.Join(home, "dd", "scrappy", "native", "bin", "scrappy-infer"),
		// In the scrappy opt-native build directory (optimized).
		filepath.Join(home, "dd", "scrappy", "opt-native", "build", "scrappy-infer"),
		// In the scrappy native build directory.
		filepath.Join(home, "dd", "scrappy", "native", "build", "scrappy-infer"),
		filepath.Join(home, "go", "src", "github.com", "DataDog", "scrappy", "native", "bin", "scrappy-infer"),
		filepath.Join(home, "go", "src", "github.com", "DataDog", "scrappy", "native", "build", "scrappy-infer"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// findScorerScript locates scrappy_scorer.py.
func findScorerScript(checkpointPath string) string {
	home := os.Getenv("HOME")
	candidates := []string{
		// Next to Go source (development).
		filepath.Join(observerImplDir(), "scrappy_scorer.py"),
		// Next to checkpoint.
		filepath.Join(filepath.Dir(checkpointPath), "scrappy_scorer.py"),
		// Eval directory (TL sandbox layout).
		filepath.Join(home, "eval", "scrappy_scorer.py"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// observerImplDir returns the directory containing the observer impl Go source.
func observerImplDir() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(os.Getenv("HOME"), "go")
	}
	return filepath.Join(gopath, "src", "github.com", "DataDog", "datadog-agent", "comp", "observer", "impl")
}

// findPython returns the path to a Python interpreter with torch installed.
// Checks known venv locations, then falls back to system python3.
func findPython() string {
	home := os.Getenv("HOME")
	candidates := []string{
		filepath.Join(home, "dd", "scrappy", ".venv-eval", "bin", "python3"),
		filepath.Join(home, "venv", "bin", "python3"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "python3"
}
