// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTaskARN = "arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"

func TestTaskARNTagDisabled(t *testing.T) {
	called := false
	tag := newTaskARNTag(false, func(context.Context) (string, error) {
		called = true
		return testTaskARN, nil
	}, time.Now)

	assert.Nil(t, tag.Tags())
	assert.False(t, called)
}

func TestTaskARNTagCachesTheARN(t *testing.T) {
	calls := 0
	tag := newTaskARNTag(true, func(context.Context) (string, error) {
		calls++
		return testTaskARN, nil
	}, time.Now)

	assert.Equal(t, []string{"task_arn:" + testTaskARN}, tag.Tags())
	assert.Equal(t, []string{"task_arn:" + testTaskARN}, tag.Tags())
	assert.Equal(t, 1, calls)
}

func TestTaskARNTagRetriesAfterDelay(t *testing.T) {
	now := time.Unix(1000, 0)
	calls := 0
	tag := newTaskARNTag(true, func(context.Context) (string, error) {
		calls++
		switch calls {
		case 1:
			return "", errors.New("metadata endpoint not ready")
		case 2:
			return "", nil
		default:
			return testTaskARN, nil
		}
	}, func() time.Time { return now })

	assert.Nil(t, tag.Tags(), "lookup error")
	assert.Nil(t, tag.Tags(), "no new lookup before the retry delay")
	assert.Equal(t, 1, calls)

	now = now.Add(taskARNLookupRetryDelay)
	assert.Nil(t, tag.Tags(), "empty ARN")
	assert.Equal(t, 2, calls)

	now = now.Add(taskARNLookupRetryDelay)
	assert.Equal(t, []string{"task_arn:" + testTaskARN}, tag.Tags())
	assert.Equal(t, 3, calls)
}

func TestGetECSTaskARNFromV4Endpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v4/abc/task", r.URL.Path)
		_, _ = w.Write([]byte(`{"Cluster":"cluster","TaskARN":"` + testTaskARN + `","Containers":[]}`))
	}))
	defer srv.Close()

	arn, err := getECSTaskARN(context.Background(), srv.Client(), srv.URL+"/v4/abc/")
	require.NoError(t, err)
	assert.Equal(t, testTaskARN, arn)
}

func TestGetECSTaskARNErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantARN string
		wantErr bool
	}{
		{name: "non-200", status: http.StatusInternalServerError, wantErr: true},
		{name: "invalid JSON", status: http.StatusOK, body: "not json", wantErr: true},
		{name: "no ARN", status: http.StatusOK, body: `{"Cluster":"cluster"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			arn, err := getECSTaskARN(context.Background(), srv.Client(), srv.URL)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantARN, arn)
		})
	}
}
