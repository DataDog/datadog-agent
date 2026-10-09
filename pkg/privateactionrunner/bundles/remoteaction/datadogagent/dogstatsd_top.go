// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed by Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_datadogagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/aggregator/contexttop"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

const (
	defaultDogstatsdTopNumMetrics = 10
	defaultDogstatsdTopNumTags    = 5
	maxDogstatsdTopNumMetrics     = 50
	maxDogstatsdTopNumTags        = 20
	dogstatsdTopSourceDump        = "dump"
	dogstatsdTopOutputLimit       = 10 * 1024 * 1024
	dogstatsdTopStderrLimit       = 16 * 1024
)

var errDogstatsdTopOutputLimit = errors.New("agent CLI output limit exceeded")

type dogstatsdTopCommand func(context.Context, string, ...string) (string, error)

// GetDogstatsdTopHandler returns the metrics with the most active DogStatsD contexts.
type GetDogstatsdTopHandler struct {
	ipcClient   ipc.HTTPClient
	agentBinary string
	runCommand  dogstatsdTopCommand
	coordinator *dogstatsdTopCoordinator
}

// NewGetDogstatsdTopHandler creates a new GetDogstatsdTopHandler.
func NewGetDogstatsdTopHandler(client ipc.HTTPClient) *GetDogstatsdTopHandler {
	return &GetDogstatsdTopHandler{
		ipcClient:   client,
		agentBinary: defaultpaths.GetDefaultDDAgentBin(),
		runCommand:  runDogstatsdTopCommand,
		coordinator: newDogstatsdTopCoordinator(),
	}
}

// GetDogstatsdTopInputs defines the options for the DogStatsD context summary.
type GetDogstatsdTopInputs struct {
	NumMetrics *int   `json:"num_metrics,omitempty"`
	NumTags    *int   `json:"num_tags,omitempty"`
	Source     string `json:"source,omitempty"`
}

type dogstatsdTopOptions struct {
	numMetrics int
	numTags    int
}

type dogstatsdContextsDumpInfo struct {
	Path               string `json:"path"`
	Size               int64  `json:"size"`
	ModifiedAtUnixNano int64  `json:"modified_at_unix_nano"`
}

type dogstatsdTopResponse struct {
	Source string `json:"source"`
	contexttop.Result
}

type dogstatsdTopKey struct {
	path               string
	size               int64
	modifiedAtUnixNano int64
	numMetrics         int
	numTags            int
}

// Run executes the getDogstatsdTop action.
func (h *GetDogstatsdTopHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("getDogstatsdTop: IPC client is not available")
	}

	inputs, err := types.ExtractInputs[GetDogstatsdTopInputs](task)
	if err != nil {
		return nil, fmt.Errorf("getDogstatsdTop: failed to parse inputs: %w", err)
	}
	options, err := normalizeDogstatsdTopInputs(inputs)
	if err != nil {
		return nil, fmt.Errorf("getDogstatsdTop: %w", err)
	}

	dumpInfo, err := h.getDogstatsdContextsDumpInfo(ctx)
	if err != nil {
		return nil, err
	}

	key := dogstatsdTopKey{
		path:               dumpInfo.Path,
		size:               dumpInfo.Size,
		modifiedAtUnixNano: dumpInfo.ModifiedAtUnixNano,
		numMetrics:         options.numMetrics,
		numTags:            options.numTags,
	}
	result, err := h.coordinator.Do(ctx, key, func(operationCtx context.Context) (dogstatsdTopResponse, error) {
		return h.runDogstatsdTop(operationCtx, dumpInfo.Path, options)
	})
	if err != nil {
		return nil, fmt.Errorf("getDogstatsdTop: %w", err)
	}
	return result, nil
}

func normalizeDogstatsdTopInputs(inputs GetDogstatsdTopInputs) (dogstatsdTopOptions, error) {
	if inputs.Source == "" {
		inputs.Source = dogstatsdTopSourceDump
	}
	if inputs.Source != dogstatsdTopSourceDump {
		return dogstatsdTopOptions{}, fmt.Errorf("source must be %q", dogstatsdTopSourceDump)
	}

	options := dogstatsdTopOptions{
		numMetrics: defaultDogstatsdTopNumMetrics,
		numTags:    defaultDogstatsdTopNumTags,
	}
	if inputs.NumMetrics != nil {
		options.numMetrics = *inputs.NumMetrics
	}
	if inputs.NumTags != nil {
		options.numTags = *inputs.NumTags
	}
	if options.numMetrics < 1 || options.numMetrics > maxDogstatsdTopNumMetrics {
		return dogstatsdTopOptions{}, fmt.Errorf("num_metrics must be between 1 and %d", maxDogstatsdTopNumMetrics)
	}
	if options.numTags < 1 || options.numTags > maxDogstatsdTopNumTags {
		return dogstatsdTopOptions{}, fmt.Errorf("num_tags must be between 1 and %d", maxDogstatsdTopNumTags)
	}
	return options, nil
}

func (h *GetDogstatsdTopHandler) getDogstatsdContextsDumpInfo(ctx context.Context) (dogstatsdContextsDumpInfo, error) {
	base, err := agentBaseURL()
	if err != nil {
		return dogstatsdContextsDumpInfo{}, fmt.Errorf("getDogstatsdTop: %w", err)
	}
	resp, err := h.ipcClient.Get(base+"/agent/dogstatsd-contexts-dump", ipchttp.WithContext(ctx))
	if err != nil {
		if msg := strings.TrimSpace(string(resp)); msg != "" {
			return dogstatsdContextsDumpInfo{}, fmt.Errorf("getDogstatsdTop: request to agent failed: %s", msg)
		}
		return dogstatsdContextsDumpInfo{}, fmt.Errorf("getDogstatsdTop: request to agent failed: %w", err)
	}

	var result dogstatsdContextsDumpInfo
	if err := json.Unmarshal(resp, &result); err != nil {
		return dogstatsdContextsDumpInfo{}, fmt.Errorf("getDogstatsdTop: invalid response from agent: %w", err)
	}
	if strings.TrimSpace(result.Path) == "" {
		return dogstatsdContextsDumpInfo{}, errors.New("getDogstatsdTop: agent returned an empty dump path")
	}
	if result.Size < 0 {
		return dogstatsdContextsDumpInfo{}, errors.New("getDogstatsdTop: agent returned an invalid dump size")
	}
	return result, nil
}

func (h *GetDogstatsdTopHandler) runDogstatsdTop(
	ctx context.Context,
	path string,
	options dogstatsdTopOptions,
) (dogstatsdTopResponse, error) {
	output, err := h.runCommand(
		ctx,
		h.agentBinary,
		"dogstatsd",
		"top",
		"--path", path,
		"--json",
		"-m", strconv.Itoa(options.numMetrics),
		"-t", strconv.Itoa(options.numTags),
	)
	if err != nil {
		return dogstatsdTopResponse{}, err
	}

	decoder := json.NewDecoder(strings.NewReader(output))
	var result contexttop.Result
	if err := decoder.Decode(&result); err != nil {
		return dogstatsdTopResponse{}, fmt.Errorf("invalid JSON from agent CLI: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return dogstatsdTopResponse{}, errors.New("invalid JSON from agent CLI: multiple values")
		}
		return dogstatsdTopResponse{}, fmt.Errorf("invalid JSON from agent CLI: %w", err)
	}

	return dogstatsdTopResponse{Source: dogstatsdTopSourceDump, Result: result}, nil
}

func runDogstatsdTopCommand(ctx context.Context, binary string, args ...string) (string, error) {
	stdout, stderr := parutil.NewLimitedStdoutStderrWritersPair(dogstatsdTopOutputLimit)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()

	if stdout.LimitReached() || stderr.LimitReached() {
		return "", fmt.Errorf("agent CLI output exceeded %d bytes: %w", dogstatsdTopOutputLimit, errDogstatsdTopOutputLimit)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("agent CLI execution canceled: %w", ctxErr)
	}
	if err != nil {
		if errorOutput := dogstatsdTopErrorOutput(stderr.String()); errorOutput != "" {
			return "", fmt.Errorf("agent CLI failed: %w; stderr: %s", err, errorOutput)
		}
		return "", fmt.Errorf("agent CLI failed: %w", err)
	}
	return stdout.String(), nil
}

func dogstatsdTopErrorOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= dogstatsdTopStderrLimit {
		return output
	}
	return "[truncated] " + output[len(output)-dogstatsdTopStderrLimit:]
}

type dogstatsdTopCoordinator struct {
	mu        sync.Mutex
	flights   map[dogstatsdTopKey]*dogstatsdTopFlight
	childSlot chan struct{}
}

type dogstatsdTopFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	result  dogstatsdTopResponse
	err     error
}

func newDogstatsdTopCoordinator() *dogstatsdTopCoordinator {
	return &dogstatsdTopCoordinator{
		flights:   make(map[dogstatsdTopKey]*dogstatsdTopFlight),
		childSlot: make(chan struct{}, 1),
	}
}

func (c *dogstatsdTopCoordinator) Do(
	ctx context.Context,
	key dogstatsdTopKey,
	operation func(context.Context) (dogstatsdTopResponse, error),
) (dogstatsdTopResponse, error) {
	c.mu.Lock()
	flight := c.flights[key]
	if flight != nil {
		flight.waiters++
	} else {
		operationCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		flight = &dogstatsdTopFlight{
			done:    make(chan struct{}),
			cancel:  cancel,
			waiters: 1,
		}
		c.flights[key] = flight
		go c.run(key, flight, operationCtx, operation)
	}
	c.mu.Unlock()

	select {
	case <-flight.done:
		return flight.result, flight.err
	case <-ctx.Done():
		c.cancelWaiter(key, flight)
		return dogstatsdTopResponse{}, ctx.Err()
	}
}

func (c *dogstatsdTopCoordinator) run(
	key dogstatsdTopKey,
	flight *dogstatsdTopFlight,
	ctx context.Context,
	operation func(context.Context) (dogstatsdTopResponse, error),
) {
	select {
	case c.childSlot <- struct{}{}:
		if err := ctx.Err(); err != nil {
			flight.err = err
		} else {
			flight.result, flight.err = operation(ctx)
		}
		<-c.childSlot
	case <-ctx.Done():
		flight.err = ctx.Err()
	}

	c.mu.Lock()
	if c.flights[key] == flight {
		delete(c.flights, key)
	}
	close(flight.done)
	c.mu.Unlock()
	flight.cancel()
}

func (c *dogstatsdTopCoordinator) cancelWaiter(key dogstatsdTopKey, flight *dogstatsdTopFlight) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.flights[key] != flight {
		return
	}
	flight.waiters--
	if flight.waiters == 0 {
		delete(c.flights, key)
		flight.cancel()
	}
}
