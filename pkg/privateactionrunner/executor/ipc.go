// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"github.com/gogo/protobuf/proto"
)

const (
	// ProtocolVersion is the version of the local executor IPC protocol.
	ProtocolVersion uint32 = 1

	statusPath   = "/v1/status"
	submitPath   = "/v1/submit"
	shutdownPath = "/v1/shutdown"

	contentTypeProtobuf = "application/x-protobuf"
)

// SocketPath returns the configured local executor endpoint path.
func SocketPath(cfg model.Reader) string {
	runPath := cfg.GetString("run_path")
	if runPath == "" {
		runPath = filepath.Dir(pkgconfigsetup.DefaultDDAgentBin)
	}
	return defaultSocketPath(runPath)
}

// Listen creates the platform-local listener for the executor IPC server.
func Listen(address string) (net.Listener, error) {
	return listen(address)
}

func newHTTPClient(address string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dial(ctx, address, timeout)
			},
		},
	}
}

func postProto(ctx context.Context, client *http.Client, authToken string, path string, req proto.Message, resp proto.Message) error {
	body, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://private-action-runner-executor"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", contentTypeProtobuf)
	if authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+authToken)
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("executor returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}
	if err := proto.Unmarshal(respBody, resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

func writeProto(w http.ResponseWriter, msg proto.Message) {
	body, err := proto.Marshal(msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentTypeProtobuf)
	_, _ = w.Write(body)
}

func readProto(r *http.Request, msg proto.Message) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return proto.Unmarshal(body, msg)
}

func statusResponse(active int32, version string) *executorpb.StatusResponse {
	return &executorpb.StatusResponse{
		ProtocolVersion: ProtocolVersion,
		Ready:           true,
		ActiveTasks:     active,
		Version:         version,
	}
}
