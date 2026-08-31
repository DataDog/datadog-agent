// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const parTaskTTL = 5 * time.Minute

// parServerState holds the in-memory task queue and result map for PAR e2e tests.
// The Private Action Runner polls /api/v2/on-prem-management-service/workflow-tasks/dequeue
// to receive tasks; tests use /fakeintake/par/* control endpoints to enqueue tasks and
// read back results without needing a real OPMS backend.
type parServerState struct {
	mu           sync.Mutex
	queue        []parQueuedTask
	results      map[string]*api.PARTaskResult
	dequeueCalls int // counts how many times PAR has called the dequeue endpoint

	// Signing identity registered via /fakeintake/par/signing-key. Left zero-valued,
	// dequeued tasks carry an unsigned envelope.
	signingKeyID string
	signingKey   ed25519.PrivateKey
	orgID        int64
	runnerID     string
	connectionID string
}

type parQueuedTask struct {
	TaskID    string                 `json:"task_id"`
	ActionFQN string                 `json:"action_fqn"`
	Inputs    map[string]interface{} `json:"inputs"`
}

// --- PAR-facing handlers (called by the agent) ---

func (fi *Server) handlePARDequeue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fi.par.mu.Lock()
	defer fi.par.mu.Unlock()

	fi.par.dequeueCalls++

	if len(fi.par.queue) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	task := fi.par.queue[0]
	fi.par.queue = fi.par.queue[1:]

	bundleID, actionName := parSplitFQN(task.ActionFQN)
	remoteAction, actionInputs := parRemoteActionFromInputs(task.Inputs)
	inputs, err := parStruct(actionInputs)
	if err != nil {
		http.Error(w, "invalid task inputs", http.StatusBadRequest)
		return
	}

	pbTask := &privateactionspb.PrivateActionTask{
		ActionName:     actionName,
		BundleId:       bundleID,
		OrgId:          fi.par.orgID,
		TaskId:         task.TaskID,
		Inputs:         inputs,
		ExpirationTime: timestamppb.New(time.Now().Add(parTaskTTL)),
	}
	if fi.par.runnerID != "" {
		pbTask.ConnectionInfo = &privateactionspb.ConnectionInfo{
			ConnectionId:    fi.par.connectionID,
			CredentialsType: privateactionspb.CredentialsType_TOKEN_AUTH,
			RunnerId:        fi.par.runnerID,
		}
	}
	if remoteAction != nil {
		pbTask.SystemInputs = &privateactionspb.SystemInputs{
			Input: &privateactionspb.SystemInputs_RemoteAction{
				RemoteAction: remoteAction,
			},
		}
	}

	signedTaskData, err := proto.Marshal(pbTask)
	if err != nil {
		http.Error(w, "failed to marshal task", http.StatusInternalServerError)
		return
	}

	envelope := map[string]interface{}{"data": signedTaskData}
	if fi.par.signingKey != nil {
		hashedPayload := sha256.Sum256(signedTaskData)
		envelope["hash_type"] = int32(privateactionspb.HashType_SHA256)
		envelope["signatures"] = []map[string]interface{}{
			{
				"key_type":  int32(privateactionspb.KeyType_ED25519),
				"key_id":    fi.par.signingKeyID,
				"signature": ed25519.Sign(fi.par.signingKey, hashedPayload[:]),
			},
		}
	}

	attributes := map[string]interface{}{
		"name":            actionName,
		"bundle_id":       bundleID,
		"task_id":         task.TaskID,
		"job_id":          task.TaskID,
		"org_id":          fi.par.orgID,
		"inputs":          actionInputs,
		"signed_envelope": envelope,
	}
	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"id":         task.TaskID,
			"type":       "task",
			"attributes": attributes,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func parRemoteActionFromInputs(inputs map[string]interface{}) (*privateactionspb.RemoteAction, map[string]interface{}) {
	actionInputs := make(map[string]interface{}, len(inputs))
	for key, value := range inputs {
		actionInputs[key] = value
	}

	// rshell policy fields are delivered in the signed task fields in production
	// (resolved from execution policies by the backend). The runner reads them
	// from system_inputs.remote_action, not inputs. Surface any flat test inputs
	// in the serialized task payload so fakeintake behaves like a real
	// backend-signed task.
	remoteAction := &privateactionspb.RemoteAction{}
	hasRemoteAction := false
	if v, ok := inputs["allowedCommands"]; ok {
		if commands, ok := parStringSlice(v); ok {
			remoteAction.AllowedCommands = commands
			hasRemoteAction = true
			delete(actionInputs, "allowedCommands")
		}
	}
	if v, ok := inputs["allowedPaths"]; ok {
		if paths, ok := parStringSlice(v); ok {
			remoteAction.AllowedPaths = paths
			hasRemoteAction = true
			delete(actionInputs, "allowedPaths")
		}
	}
	if v, ok := inputs["systemServices"]; ok {
		if services, ok := parSystemServices(v); ok {
			remoteAction.SystemServices = services
			hasRemoteAction = true
			delete(actionInputs, "systemServices")
		}
	}
	if !hasRemoteAction {
		return nil, actionInputs
	}
	return remoteAction, actionInputs
}

func parSystemServices(value interface{}) (map[string]*structpb.ListValue, bool) {
	values, ok := value.(map[string]interface{})
	if !ok {
		return nil, false
	}
	policy, err := structpb.NewStruct(values)
	if err != nil {
		return nil, false
	}

	services := make(map[string]*structpb.ListValue, len(policy.Fields))
	for service, value := range policy.Fields {
		actions := value.GetListValue()
		if actions == nil {
			return nil, false
		}
		services[service] = actions
	}
	return services, true
}

func parStringSlice(value interface{}) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return values, true
	case []interface{}:
		strings := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok {
				strings = append(strings, s)
			}
		}
		return strings, true
	default:
		return nil, false
	}
}

func parStruct(inputs map[string]interface{}) (*structpb.Struct, error) {
	normalized := make(map[string]interface{}, len(inputs))
	for key, value := range inputs {
		normalized[key] = parStructValue(value)
	}
	return structpb.NewStruct(normalized)
}

func parStructValue(value interface{}) interface{} {
	switch v := value.(type) {
	case []string:
		values := make([]interface{}, 0, len(v))
		for _, value := range v {
			values = append(values, value)
		}
		return values
	case []interface{}:
		values := make([]interface{}, 0, len(v))
		for _, value := range v {
			values = append(values, parStructValue(value))
		}
		return values
	case map[string]interface{}:
		values := make(map[string]interface{}, len(v))
		for key, value := range v {
			values[key] = parStructValue(value)
		}
		return values
	default:
		return value
	}
}

func (fi *Server) handlePARPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var req struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				TaskID  string `json:"task_id"`
				Payload struct {
					Outputs      map[string]interface{} `json:"outputs"`
					ErrorCode    int                    `json:"error_code"`
					ErrorDetails string                 `json:"error_details"`
				} `json:"payload"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	result := &api.PARTaskResult{
		TaskID:       req.Data.Attributes.TaskID,
		Success:      req.Data.ID == "succeed_task",
		Outputs:      req.Data.Attributes.Payload.Outputs,
		ErrorCode:    req.Data.Attributes.Payload.ErrorCode,
		ErrorDetails: req.Data.Attributes.Payload.ErrorDetails,
	}

	fi.par.mu.Lock()
	fi.par.results[result.TaskID] = result
	fi.par.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (fi *Server) handlePARHealthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"type": "healthCheckResponse",
			"id":   "fakeintake-par",
			"attributes": map[string]interface{}{
				"id": "fake-runner",
			},
		},
	})
}

func (fi *Server) handlePARHeartbeat(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// --- Control handlers (called by the test process) ---

func (fi *Server) handlePAREnqueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var task parQueuedTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	fi.par.mu.Lock()
	fi.par.queue = append(fi.par.queue, task)
	fi.par.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (fi *Server) handlePARResult(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("taskID")
	if taskID == "" {
		http.Error(w, "taskID query param required", http.StatusBadRequest)
		return
	}

	fi.par.mu.Lock()
	result, ok := fi.par.results[taskID]
	fi.par.mu.Unlock()

	if !ok {
		http.Error(w, "no result yet", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// handlePARSetSigningKey registers a signing identity used by handlePARDequeue.
func (fi *Server) handlePARSetSigningKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		KeyID        string `json:"key_id"`
		PrivateKey   []byte `json:"private_key"`
		OrgID        int64  `json:"org_id"`
		RunnerID     string `json:"runner_id"`
		ConnectionID string `json:"connection_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.KeyID == "" || req.RunnerID == "" || req.ConnectionID == "" || len(req.PrivateKey) != ed25519.PrivateKeySize {
		http.Error(w, "invalid signing configuration", http.StatusBadRequest)
		return
	}

	fi.par.mu.Lock()
	fi.par.signingKeyID = req.KeyID
	fi.par.signingKey = ed25519.PrivateKey(req.PrivateKey)
	fi.par.orgID = req.OrgID
	fi.par.runnerID = req.RunnerID
	fi.par.connectionID = req.ConnectionID
	fi.par.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (fi *Server) handlePARFlush(w http.ResponseWriter, _ *http.Request) {
	fi.par.mu.Lock()
	fi.par.queue = nil
	fi.par.results = make(map[string]*api.PARTaskResult)
	fi.par.dequeueCalls = 0
	fi.par.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (fi *Server) handlePARStats(w http.ResponseWriter, _ *http.Request) {
	fi.par.mu.Lock()
	calls := fi.par.dequeueCalls
	fi.par.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"dequeue_calls": calls})
}

// parSplitFQN splits "com.foo.bar.actionName" into ("com.foo.bar", "actionName").
func parSplitFQN(fqn string) (string, string) {
	idx := strings.LastIndex(fqn, ".")
	if idx < 0 {
		return fqn, ""
	}
	return fqn[:idx], fqn[idx+1:]
}
