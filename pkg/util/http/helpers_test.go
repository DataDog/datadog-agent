// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test ok"))
	}))
	defer ts.Close()

	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	res, err := Get(context.Background(), ts.URL, nil, 5*time.Second, c)

	require.NoError(t, err)
	assert.Equal(t, "test ok", res)
}

func TestGetHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "value", r.Header.Get("header"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test ok"))
	}))
	defer ts.Close()

	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	res, err := Get(context.Background(), ts.URL, map[string]string{"header": "value"}, 5*time.Second, c)

	require.NoError(t, err)
	assert.Equal(t, "test ok", res)
}

func TestGetTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test ok"))
	}))
	defer ts.Close()

	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	_, err := Get(context.Background(), ts.URL, map[string]string{"header": "value"}, 100*time.Millisecond, c)

	require.Error(t, err)
}

func TestGetError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("test ok"))
	}))
	defer ts.Close()

	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	_, err := Get(context.Background(), ts.URL, map[string]string{"header": "value"}, 5*time.Second, c)

	require.Error(t, err)
}

func TestSetJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	err := errors.New("some error")
	errorCode := http.StatusInternalServerError

	SetJSONError(w, err, errorCode)

	res := w.Result()
	defer res.Body.Close()

	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))

	// Verify the response body
	expectedBody := "{\"error\":\"some error\"}\n"

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.EqualValues(t, []byte(expectedBody), body)

	// Verify the response status code
	assert.Equal(t, errorCode, res.StatusCode)
}
