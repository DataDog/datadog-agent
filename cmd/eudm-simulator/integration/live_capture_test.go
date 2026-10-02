// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib

package integration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processapi "github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// These are independent synthetic producer processes using real serializer,
// submitter, forwarder, and session code. They do not satisfy the installed-Agent
// platform gate or prove delivery to an operator's configured backend.
type producerHello struct {
	URL string
	PID int
}

type producerAction struct {
	Name    string
	Control tc.Control
}

type producerResult struct {
	Status    tc.Status
	PID       int
	Digests   []string
	Committed bool
}

type captureBatch struct {
	Status  tc.Status   `json:"status"`
	Records []tc.Record `json:"records"`
}

type subprocessProducer struct {
	url, token string
	pid        int
	stream     tc.Stream
	client     *http.Client
	identity   tc.Identity
}

func TestLiveCaptureProducerHelper(t *testing.T) {
	role := os.Getenv("EUDM_TEST_PRODUCER_ROLE")
	if role == "" {
		return
	}
	if role != "core-agent" && role != "process-agent" {
		t.Fatal("invalid helper role")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	recorder := output.NewRecorder()
	pipeline, err := output.New(ctx, nil, "synthetic-delivery-key", recorder, output.Options{MetricProtocol: "v1"})
	if err != nil {
		t.Fatal("cannot construct synthetic delivery pipeline")
	}
	defer pipeline.Close()
	manager := tc.NewManager(role, "7.85.0-fixture", fixtureCommit)
	defer manager.Close()
	stream := tc.Metrics
	if role == "process-agent" {
		stream = tc.Processes
	} else if selected := tc.Stream(os.Getenv("EUDM_TEST_PRODUCER_STREAM")); selected != "" {
		if selected != tc.AgentInventory && selected != tc.HostInventory && selected != tc.HostSystemInfo {
			t.Fatal("invalid synthetic inventory stream")
		}
		stream = selected
	}
	capability := tc.Capability{Stream: stream, Cadence: 15 * time.Second}
	if stream == tc.Metrics {
		capability.MetricSchedules = []tc.MetricSchedule{{Family: "cpu", Cadence: 15 * time.Second}}
	} else if stream == tc.AgentInventory || stream == tc.HostInventory || stream == tc.HostSystemInfo {
		capability.Cadence = syntheticInventoryCadence(stream)
	}
	if err := manager.Register(capability); err != nil {
		t.Fatal("cannot register synthetic producer")
	}
	// No output has been submitted. The subsequent production queue admission
	// synchronizes the submitter's first observation with this initialization.
	pipeline.Serializer.LiveCapture, pipeline.Serializer.LiveCaptureCadence = manager, 15*time.Second
	pipeline.Submitter.CaptureManager = manager
	pipeline.Submitter.SetCaptureCadence(checks.ProcessCheckName, 15*time.Second)
	var pending *tc.Reservation
	var actions sync.Mutex
	token := os.Getenv("EUDM_TEST_PRODUCER_TOKEN")
	if token == "" {
		t.Fatal("missing synthetic IPC token")
	}
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	api, err := manager.Handler(authenticate)
	if err != nil {
		t.Fatal("cannot construct synthetic session API")
	}
	stopping := make(chan struct{})
	mux := http.NewServeMux()
	mux.Handle("/capture/", http.StripPrefix("/capture", api))
	mux.Handle("POST /test", authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actions.Lock()
		defer actions.Unlock()
		var action producerAction
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&action) != nil {
			http.Error(w, "invalid test action", http.StatusBadRequest)
			return
		}
		result := producerResult{PID: os.Getpid()}
		var actionErr error
		switch action.Name {
		case "submit":
			if stream == tc.Metrics {
				actionErr = pipeline.Serializer.SendIterableSeries(capture.NewSeriesSource([]*metrics.Serie{{Name: "system.cpu.user", Host: "synthetic-device", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1234567890.25, Value: 5}}}}))
			} else if stream == tc.AgentInventory || stream == tc.HostInventory || stream == tc.HostSystemInfo {
				actionErr = pipeline.Serializer.SendMetadata(&syntheticInventorySubmission{stream: stream, inventory: syntheticInventory(stream)})
			} else {
				bodies := make([]model.MessageBody, 2)
				for i := range bodies {
					bodies[i] = &model.CollectorProc{HostName: "synthetic-device", GroupId: 71, GroupSize: 2, Processes: []*model.Process{{Pid: int32(42 + i), Command: &model.Command{Comm: "synthetic-process"}}}}
				}
				actionErr = pipeline.Group(r.Context(), time.Now(), checks.ProcessCheckName, "synthetic-device", bodies)
			}
			if actionErr == nil {
				actionErr = pipeline.Wait(r.Context())
			}
			for _, ref := range recorder.Drain() {
				digest := sha256.Sum256(append([]byte(ref.Path+"\x00"), ref.Body...))
				result.Digests = append(result.Digests, hex.EncodeToString(digest[:]))
			}
		case "fill":
			// Hold exactly the production limit so the next real submission's
			// observation fails capture without changing that submission.
			for range tc.MaxRecords {
				reservation := manager.Begin(stream, time.Now(), time.Second, 4096)
				if reservation == nil || !reservation.Commit(syntheticQueuedPayload(stream)) {
					actionErr = errors.New("cannot fill capture queue")
					break
				}
			}
		case "worker-failure":
			manager.Observe(stream, time.Now(), time.Second, 4096, func() tc.Payload {
				panic("synthetic capture copy failure")
			})
		case "reserve":
			if pending != nil {
				actionErr = errors.New("pending copy already exists")
			} else {
				pending = manager.Begin(stream, time.Now(), time.Second, 4096)
				if pending == nil {
					actionErr = errors.New("cannot reserve pending copy")
				}
			}
		case "commit":
			result.Committed = pending.Commit(syntheticQueuedPayload(stream))
			pending = nil
		case "fail":
			actionErr = manager.Fail(action.Control)
		case "close":
			manager.Close()
		case "exit":
			pending.Discard()
			pending = nil
			close(stopping)
		default:
			actionErr = errors.New("unknown test action")
		}
		if actionErr != nil {
			http.Error(w, "synthetic producer action failed", http.StatusConflict)
			return
		}
		result.Status = manager.Status()
		_ = json.NewEncoder(w).Encode(result)
	})))
	server := httptest.NewServer(mux)
	defer server.Close()
	if err := json.NewEncoder(os.Stdout).Encode(producerHello{URL: server.URL, PID: os.Getpid()}); err != nil {
		t.Fatal("cannot announce synthetic producer")
	}
	select {
	case <-stopping:
	case <-ctx.Done():
		t.Fatal("synthetic producer timed out")
	}
}

func syntheticQueuedPayload(stream tc.Stream) tc.Payload {
	if stream == tc.Metrics {
		return tc.Payload{Series: []tc.Series{{Name: "system.cpu.user", Ordinal: 1, Host: "synthetic-device", Points: []tc.Point{{Timestamp: 1, Value: 5}}}}}
	}
	if stream == tc.AgentInventory || stream == tc.HostInventory || stream == tc.HostSystemInfo {
		return tc.Payload{Inventory: syntheticInventory(stream)}
	}
	body, err := processapi.EncodePayload(&model.CollectorProc{HostName: "synthetic-device", GroupId: 72, GroupSize: 1})
	if err != nil {
		panic("cannot encode synthetic pending group")
	}
	return tc.Payload{Chunks: []tc.Chunk{{Body: body, Headers: map[string]string{headers.HostHeader: "synthetic-device", headers.RequestIDHeader: "16384"}}}}
}

func startSubprocessProducer(t *testing.T, role string, inventoryStream ...tc.Stream) *subprocessProducer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLiveCaptureProducerHelper$")
	token := rand.Text()
	command.Env = append(os.Environ(), "EUDM_TEST_PRODUCER_ROLE="+role, "EUDM_TEST_PRODUCER_TOKEN="+token)
	if len(inventoryStream) != 0 {
		command.Env = append(command.Env, "EUDM_TEST_PRODUCER_STREAM="+string(inventoryStream[0]))
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal("cannot attach synthetic producer output")
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal("cannot start synthetic producer process")
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	producer := &subprocessProducer{token: token, stream: tc.Metrics, client: &http.Client{Timeout: 8 * time.Second}}
	if role == "process-agent" {
		producer.stream = tc.Processes
	} else if len(inventoryStream) != 0 {
		producer.stream = inventoryStream[0]
	}
	t.Cleanup(func() {
		defer cancel()
		if producer.url != "" {
			_, _ = producer.request("/test", producerAction{Name: "exit"}, new(producerResult), true)
		}
		select {
		case err := <-exited:
			if err != nil {
				t.Error("synthetic producer exited with a test or race failure")
			}
		case <-time.After(8 * time.Second):
			cancel()
			<-exited
			t.Error("synthetic producer did not shut down cleanly")
		}
	})
	hello := make(chan producerHello, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var announcement producerHello
			if json.Unmarshal(scanner.Bytes(), &announcement) == nil && announcement.URL != "" {
				hello <- announcement
				_, _ = io.Copy(io.Discard, stdout)
				return
			}
		}
		close(hello)
	}()
	select {
	case announcement := <-hello:
		if announcement.URL == "" || announcement.PID == os.Getpid() || announcement.PID <= 0 {
			t.Fatal("synthetic producer did not start as an independent process")
		}
		producer.url, producer.pid = announcement.URL, announcement.PID
	case <-time.After(20 * time.Second):
		t.Fatal("synthetic producer readiness timed out")
	}
	status := producer.status(t)
	if status.Producer.Role != role || status.State != "" || len(status.Capabilities) != 1 || status.Capabilities[0].Stream != producer.stream {
		t.Fatal("unexpected synthetic producer readiness")
	}
	producer.identity = status.Producer
	return producer
}

func (p *subprocessProducer) request(path string, value, result any, authenticated bool) (int, error) {
	method := http.MethodGet
	var body io.Reader
	if value != nil {
		method = http.MethodPost
		data, err := json.Marshal(value)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, p.url+path, body)
	if err != nil {
		return 0, err
	}
	if authenticated {
		request.Header.Set("Authorization", "Bearer "+p.token)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return response.StatusCode, nil
	}
	return response.StatusCode, json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(result)
}

func (p *subprocessProducer) call(t *testing.T, path string, value, result any) {
	t.Helper()
	if code, err := p.request(path, value, result, true); err != nil || code != http.StatusOK {
		t.Fatalf("synthetic producer API %s failed (HTTP %d)", path, code)
	}
}

func (p *subprocessProducer) status(t *testing.T) tc.Status {
	t.Helper()
	var status tc.Status
	p.call(t, "/capture/status", nil, &status)
	if p.identity.InstanceID != "" && status.Producer != p.identity {
		t.Fatal("producer process identity changed during capture")
	}
	return status
}

func (p *subprocessProducer) action(t *testing.T, name string) producerResult {
	t.Helper()
	var result producerResult
	p.call(t, "/test", producerAction{Name: name}, &result)
	if result.PID != p.pid || result.Status.Producer != p.identity {
		t.Fatal("capture action changed the running producer process")
	}
	return result
}

func (p *subprocessProducer) arm(t *testing.T, control tc.Control) {
	t.Helper()
	var status tc.Status
	p.call(t, "/capture/prepare", tc.PrepareRequest{Control: control, Streams: []tc.Stream{p.stream}}, &status)
	if status.State != tc.Prepared || status.FinalSequence != 0 {
		t.Fatal("producer did not prepare an empty session")
	}
	p.call(t, "/capture/activate", control, &status)
	if status.State != tc.Active || status.ActivatedAt.IsZero() {
		t.Fatal("producer did not acknowledge activation")
	}
}

func (p *subprocessProducer) stopAndDrain(t *testing.T, control tc.Control, expected int) {
	t.Helper()
	var stopped tc.Status
	p.call(t, "/capture/stop", control, &stopped)
	var cursor uint64
	for rounds := 0; ; rounds++ {
		if rounds > tc.MaxRecords {
			t.Fatal("producer drain did not reach its final acknowledgement")
		}
		var batch captureBatch
		p.call(t, "/capture/records", tc.ReadRequest{Control: control, Cursor: cursor}, &batch)
		for _, record := range batch.Records {
			if record.Sequence != cursor+1 || record.Producer != p.identity || record.SessionID != control.SessionID || record.Stream != p.stream || record.ObservedAt.After(stopped.StoppedAt) {
				t.Fatal("cross-process record lost ordering or session boundaries")
			}
			cursor = record.Sequence
		}
		if batch.Status.State == tc.Stopped {
			if cursor != uint64(expected) || batch.Status.FinalSequence != cursor || batch.Status.Acknowledged != cursor {
				t.Fatal("producer stopped without complete acknowledgement")
			}
			break
		}
	}
	p.call(t, "/capture/stop", control, &stopped)
	if stopped.State != tc.Stopped {
		t.Fatal("final stopped acknowledgement missing")
	}
}

func assertSubmissionMatches(t *testing.T, p *subprocessProducer, baseline []string) {
	t.Helper()
	result := p.action(t, "submit")
	if len(result.Digests) == 0 || !slices.Equal(result.Digests, baseline) {
		t.Fatal("capture changed normal Agent submission bodies")
	}
}

// The complete production body deliberately includes a private configuration
// value. Only the separate allowlisted projection can cross the capture API.
type syntheticInventorySubmission struct {
	stream    tc.Stream
	inventory *tc.Inventory
}

func (p *syntheticInventorySubmission) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Hostname   string                     `json:"hostname"`
		UUID       string                     `json:"uuid"`
		Timestamp  int64                      `json:"timestamp"`
		Agent      *tc.AgentInventoryMetadata `json:"agent_metadata,omitempty"`
		Host       *tc.HostInventoryMetadata  `json:"host_metadata,omitempty"`
		SystemInfo *tc.HostSystemInfoMetadata `json:"host_system_info_metadata,omitempty"`
		Config     string                     `json:"configuration"`
	}{p.inventory.Hostname, p.inventory.UUID, p.inventory.Timestamp, p.inventory.Agent, p.inventory.Host, p.inventory.SystemInfo, "synthetic-private-inventory-config"})
}

func (p *syntheticInventorySubmission) CaptureInventoryStream() tc.Stream { return p.stream }
func (p *syntheticInventorySubmission) CaptureInventorySchedule() (time.Time, time.Duration) {
	return time.Now(), syntheticInventoryCadence(p.stream)
}
func (p *syntheticInventorySubmission) CaptureInventorySize() int64 {
	return tc.InventorySize(p.inventory)
}
func (p *syntheticInventorySubmission) CopyCaptureInventory() *tc.Inventory {
	return tc.CloneInventory(p.inventory)
}

func syntheticInventoryCadence(stream tc.Stream) time.Duration {
	if stream == tc.HostSystemInfo {
		return time.Hour
	}
	return 10 * time.Minute
}

func syntheticInventory(stream tc.Stream) *tc.Inventory {
	inventory := &tc.Inventory{Hostname: "synthetic-device", UUID: "synthetic-inventory-uuid", Timestamp: 1234567890250000000}
	switch stream {
	case tc.AgentInventory:
		inventory.Agent = &tc.AgentInventoryMetadata{AgentVersion: "7.85.0-fixture", Flavor: "agent", InfrastructureMode: "end_user_device", AgentStartupTimeMS: 1234567800000}
	case tc.HostInventory:
		inventory.Host = &tc.HostInventoryMetadata{OS: "darwin", CPUArchitecture: "arm64", CPULogicalProcessors: 4, MemoryTotalKb: 16 << 20, AgentVersion: "7.85.0-fixture"}
	case tc.HostSystemInfo:
		inventory.SystemInfo = &tc.HostSystemInfoMetadata{Manufacturer: "Apple Inc.", ModelName: "fixture-model", ChassisType: "Laptop", SerialNumber: "synthetic-device-serial"}
	}
	return inventory
}

func TestLiveCaptureInventoryAcrossProcessBoundary(t *testing.T) {
	for _, stream := range []tc.Stream{tc.AgentInventory, tc.HostInventory, tc.HostSystemInfo} {
		t.Run(string(stream), func(t *testing.T) {
			producer := startSubprocessProducer(t, "core-agent", stream)
			baseline := producer.action(t, "submit").Digests
			if len(baseline) != 1 || producer.status(t).FinalSequence != 0 {
				t.Fatal("disabled inventory capture retained output or lost normal delivery")
			}
			control := tc.Control{ProtocolVersion: tc.ProtocolVersion, SessionID: rand.Text()}
			producer.arm(t, control)
			assertSubmissionMatches(t, producer, baseline)
			var batch captureBatch
			producer.call(t, "/capture/records", tc.ReadRequest{Control: control}, &batch)
			if len(batch.Records) != 1 {
				t.Fatal("inventory submission did not yield exactly one capture cycle")
			}
			record := batch.Records[0]
			if record.Stream != stream || record.Payload.Inventory == nil || record.Cadence != syntheticInventoryCadence(stream) || len(record.Payload.Routes) == 0 {
				t.Fatal("inventory capture lost its envelope, schedule or observed route")
			}
			for _, route := range record.Payload.Routes {
				if route.Endpoint != "/api/v1/metadata" || route.Protocol != "inventory-v1" {
					t.Fatal("inventory capture observed an unexpected delivery route")
				}
			}
			encoded, err := json.Marshal(record)
			if err != nil || bytes.Contains(encoded, []byte("synthetic-private-inventory-config")) || bytes.Contains(encoded, []byte("configuration")) {
				t.Fatal("inventory capture exposed production configuration")
			}
			producer.stopAndDrain(t, control, 1)
			assertSubmissionMatches(t, producer, baseline)
			control.SessionID = rand.Text()
			producer.arm(t, control)
			producer.action(t, "fill")
			assertSubmissionMatches(t, producer, baseline)
			if status := producer.status(t); status.State != tc.Failed || status.Drops != 1 {
				t.Fatal("inventory capture overflow did not fail only the capture session")
			}
		})
	}
}

func TestLiveCaptureSeparateProducerProcesses(t *testing.T) {
	core, process := startSubprocessProducer(t, "core-agent"), startSubprocessProducer(t, "process-agent")
	if core.pid == process.pid || core.identity.InstanceID == process.identity.InstanceID {
		t.Fatal("producer inventory does not represent separate running processes")
	}
	for _, producer := range []*subprocessProducer{core, process} {
		t.Run(producer.identity.Role, func(t *testing.T) {
			baseline := producer.action(t, "submit").Digests
			if len(baseline) == 0 || producer.status(t).FinalSequence != 0 {
				t.Fatal("disabled capture retained output or lost normal delivery")
			}
			control := tc.Control{ProtocolVersion: tc.ProtocolVersion, SessionID: rand.Text()}
			for _, operation := range []string{"capabilities", "status", "prepare", "activate", "heartbeat", "records", "stop"} {
				var value any = control
				if operation == "capabilities" || operation == "status" {
					value = nil
				}
				code, err := producer.request("/capture/"+operation, value, new(tc.Status), false)
				if err != nil || code != http.StatusUnauthorized {
					t.Fatal("producer accepted an unauthenticated operation")
				}
			}
			producer.arm(t, control)
			assertSubmissionMatches(t, producer, baseline)
			var first, repeat captureBatch
			producer.call(t, "/capture/records", tc.ReadRequest{Control: control}, &first)
			producer.call(t, "/capture/records", tc.ReadRequest{Control: control}, &repeat)
			if len(first.Records) != 1 || len(repeat.Records) != 1 || first.Records[0].CycleID != repeat.Records[0].CycleID || first.Records[0].Sequence != repeat.Records[0].Sequence {
				t.Fatal("repeated IPC reads manufactured collection cycles")
			}
			if producer.stream == tc.Processes {
				messages, err := processapi.DecodeCaptureGroup(&first.Records[0])
				if err != nil || len(messages) != 2 {
					t.Fatal("captured process group is incomplete or unordered")
				}
			}
			producer.stopAndDrain(t, control, 1)
			assertSubmissionMatches(t, producer, baseline)
			if producer.status(t).FinalSequence != 1 {
				t.Fatal("stopped session admitted later normal output")
			}

			for _, failure := range []string{"fill", "worker-failure"} {
				control.SessionID = rand.Text()
				producer.arm(t, control)
				producer.action(t, failure)
				assertSubmissionMatches(t, producer, baseline)
				failed := producer.status(t)
				if failed.State != tc.Failed || failed.Failures != 1 || (failure == "fill" && failed.Drops != 1) {
					t.Fatal("capture failure was not isolated and accounted")
				}
			}

			control.SessionID = rand.Text()
			producer.arm(t, control) // Also proves failed queues released retained data.
			producer.action(t, "reserve")
			stopped := make(chan error, 1)
			go func() {
				code, err := producer.request("/capture/stop", control, new(tc.Status), true)
				if err == nil && code != http.StatusOK {
					err = errors.New("stop failed")
				}
				stopped <- err
			}()
			deadline := time.Now().Add(3 * time.Second)
			for producer.status(t).State != tc.Stopping {
				if time.Now().After(deadline) {
					t.Fatal("stop did not close admission")
				}
			}
			select {
			case <-stopped:
				t.Fatal("stop acknowledged before reserved copy completed")
			default:
			}
			if !producer.action(t, "commit").Committed {
				t.Fatal("stop discarded an accepted pending copy")
			}
			if err := <-stopped; err != nil {
				t.Fatal("stop failed after pending copy completed")
			}
			producer.stopAndDrain(t, control, 1)

			control.SessionID = rand.Text()
			producer.arm(t, control)
			producer.action(t, "reserve")
			var result producerResult
			producer.call(t, "/test", producerAction{Name: "fail", Control: control}, &result)
			if producer.action(t, "commit").Committed {
				t.Fatal("failed session retained late copied output")
			}
			old := control
			control.SessionID = rand.Text()
			producer.arm(t, control)
			code, err := producer.request("/test", producerAction{Name: "fail", Control: old}, &result, true)
			if err != nil || code != http.StatusConflict || producer.status(t).State != tc.Active {
				t.Fatal("obsolete callback affected successor session")
			}
			assertSubmissionMatches(t, producer, baseline)
			producer.stopAndDrain(t, control, 1)
			producer.action(t, "close")
			assertSubmissionMatches(t, producer, baseline)
			if len(producer.status(t).Capabilities) != 0 {
				t.Fatal("shutdown left capture capabilities running")
			}
		})
	}
}

func TestLiveCaptureCoordinatorLossExpiresRealLease(t *testing.T) {
	producers := []*subprocessProducer{startSubprocessProducer(t, "core-agent"), startSubprocessProducer(t, "process-agent")}
	control := tc.Control{ProtocolVersion: tc.ProtocolVersion, SessionID: rand.Text()}
	baselines := make([][]string, len(producers))
	for i, producer := range producers {
		baselines[i] = producer.action(t, "submit").Digests
		producer.arm(t, control)
		assertSubmissionMatches(t, producer, baselines[i])
	}
	// Deliberately omit heartbeats: this integration gate exercises the actual
	// production 30-second lease across OS processes, not a mocked unit clock.
	deadline := time.NewTimer(tc.LeaseDuration + 10*time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	for {
		allFailed := true
		for _, producer := range producers {
			allFailed = producer.status(t).State == tc.Failed && allFailed
		}
		if allFailed {
			break
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatal("coordinator loss did not disarm producers after the real lease")
		}
	}
	for i, producer := range producers {
		assertSubmissionMatches(t, producer, baselines[i])
		control.SessionID = rand.Text()
		producer.arm(t, control) // A successor requires all old raw data released.
		producer.stopAndDrain(t, control, 0)
	}
}
