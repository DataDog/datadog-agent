// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TaskARNTagName is the name of the tag that carries the ARN of the ECS task the Agent runs in.
const TaskARNTagName = "task_arn"

const (
	taskARNLookupTimeout    = 2 * time.Second
	taskARNLookupRetryDelay = 30 * time.Second
)

// TaskARNTag provides the task_arn:<task ARN> tag of the ECS Fargate task the Agent runs in as a
// sidecar. It's used to identify the task in payloads that don't otherwise carry it.
//
// The ARN is looked up from the ECS task metadata endpoint on first use and cached once found.
// The endpoint can be briefly unavailable when a task starts, so a failed lookup is retried on a
// later call, at most once every taskARNLookupRetryDelay.
type TaskARNTag struct {
	enabled    bool
	getTaskARN func(context.Context) (string, error)
	now        func() time.Time

	mu         sync.Mutex
	tags       []string
	nextLookup time.Time
}

// NewTaskARNTag returns a TaskARNTag for the current environment. Outside of ECS Fargate, its Tags
// method always returns nil.
func NewTaskARNTag() *TaskARNTag {
	return newTaskARNTag(GetOrchestrator() == ECS, func(ctx context.Context) (string, error) {
		return getECSTaskARN(ctx, metadataClient, os.Getenv(metadataURIv4EnvVar))
	}, time.Now)
}

func newTaskARNTag(enabled bool, getTaskARN func(context.Context) (string, error), now func() time.Time) *TaskARNTag {
	return &TaskARNTag{enabled: enabled, getTaskARN: getTaskARN, now: now}
}

// Tags returns []string{"task_arn:<task ARN>"} when running in ECS Fargate and the ARN is known,
// and nil otherwise.
func (t *TaskARNTag) Tags() []string {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.tags != nil || t.now().Before(t.nextLookup) {
		return t.tags
	}

	ctx, cancel := context.WithTimeout(context.Background(), taskARNLookupTimeout)
	defer cancel()
	arn, err := t.getTaskARN(ctx)
	if err == nil && arn == "" {
		err = errors.New("empty task ARN")
	}
	if err != nil {
		t.nextLookup = t.now().Add(taskARNLookupRetryDelay)
		log.Warnf("unable to get the ECS task ARN, retrying in %s: %v", taskARNLookupRetryDelay, err)
		return nil
	}

	t.tags = []string{TaskARNTagName + ":" + arn}
	return t.tags
}

const (
	metadataURIv4EnvVar = "ECS_CONTAINER_METADATA_URI_V4"
	metadataV2TaskURL   = "http://169.254.170.2/v2/metadata"
	maxTaskMetadataSize = 1 << 20
)

// metadataClient never uses a proxy: the task metadata endpoint is link-local.
var metadataClient = &http.Client{Transport: &http.Transport{Proxy: nil}}

// getECSTaskARN reads the task ARN from the ECS task metadata endpoint: v4 when ECS provides its
// URI, otherwise v2.
//
// It queries the endpoint directly instead of using pkg/util/ecs/metadata, whose clients are stubs
// that return an empty task in builds without the docker build tag, such as system-probe.
func getECSTaskARN(ctx context.Context, client *http.Client, metadataURIv4 string) (string, error) {
	url := metadataV2TaskURL
	if metadataURIv4 != "" {
		url = strings.TrimSuffix(metadataURIv4, "/") + "/task"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
	}

	var task struct {
		TaskARN string `json:"TaskARN"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTaskMetadataSize)).Decode(&task); err != nil {
		return "", fmt.Errorf("decoding task metadata from %s: %w", url, err)
	}
	return task.TaskARN, nil
}
