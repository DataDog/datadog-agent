// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	assert.Equal(t, 300*time.Millisecond, GetDefaultConfig().timeout)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/id", GetDefaultConfig().containerIDURL)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/project/project-id", GetDefaultConfig().projectIDURL)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/region", GetDefaultConfig().regionURL)
}

func TestGetSingleMetadataMalformedUrl(t *testing.T) {
	assert.Equal(t, "unknown", getSingleMetadata(&http.Client{}, string([]byte("\u007F"))))
}

func TestSingleMetadataTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer ts.Close()
	httpClient := &http.Client{
		Timeout: 1 * time.Nanosecond,
	}
	assert.Equal(t, "unknown", getSingleMetadata(httpClient, ts.URL))
}

func TestSingleMetadataOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("1234"))
	}))
	defer ts.Close()
	assert.Equal(t, "1234", getSingleMetadata(&http.Client{}, ts.URL))
}

func TestGetContainerID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("1234"))
	}))
	defer ts.Close()
	testConfig := &GCPConfig{
		timeout:        1 * time.Second,
		containerIDURL: ts.URL,
	}
	assert.Equal(t, &Info{TagName: "container_id", Value: "1234"}, getContainerID(&http.Client{}, testConfig))
}

func TestGetRegionUnknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("unknown"))
	}))
	defer ts.Close()
	testConfig := &GCPConfig{
		timeout:   1 * time.Second,
		regionURL: ts.URL,
	}
	assert.Equal(t, &Info{TagName: "location", Value: "unknown"}, getRegion(&http.Client{}, testConfig))
}

func TestGetRegionOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("projects/xxx/regions/us-central1"))
	}))
	defer ts.Close()
	testConfig := &GCPConfig{
		timeout:   1 * time.Second,
		regionURL: ts.URL,
	}
	assert.Equal(t, &Info{TagName: "location", Value: "us-central1"}, getRegion(&http.Client{}, testConfig))
}

func TestGetProjectID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("superproject"))
	}))
	defer ts.Close()
	testConfig := &GCPConfig{
		timeout:      1 * time.Second,
		projectIDURL: ts.URL,
	}
	assert.Equal(t, &Info{TagName: "project_id", Value: "superproject"}, getProjectID(&http.Client{}, testConfig))
}

func TestGetMetaDataComplete(t *testing.T) {
	tsProjectID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("superProjectID"))
	}))
	defer tsProjectID.Close()
	tsRegion := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("greatRegion"))
	}))
	defer tsRegion.Close()
	tsContainerID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("acb54"))
	}))
	defer tsContainerID.Close()

	testConfig := &GCPConfig{
		timeout:        1 * time.Second,
		projectIDURL:   tsProjectID.URL,
		regionURL:      tsRegion.URL,
		containerIDURL: tsContainerID.URL,
	}

	metadata := GetMetaData(testConfig)
	assert.Equal(t, &Info{TagName: "container_id", Value: "acb54"}, metadata.ContainerID)
	assert.Equal(t, &Info{TagName: "location", Value: "greatregion"}, metadata.Region)
	assert.Equal(t, &Info{TagName: "project_id", Value: "superprojectid"}, metadata.ProjectID)
}

func TestGetMetaDataIncompleteDueToTimeout(t *testing.T) {
	tsProjectID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("superProjectID"))
	}))
	defer tsProjectID.Close()
	tsRegion := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.Write([]byte("greatRegion"))
	}))
	defer tsRegion.Close()
	tsContainerID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("acb54"))
	}))
	defer tsContainerID.Close()

	testConfig := &GCPConfig{
		timeout:        500 * time.Millisecond,
		projectIDURL:   tsProjectID.URL,
		regionURL:      tsRegion.URL,
		containerIDURL: tsContainerID.URL,
	}

	metadata := GetMetaData(testConfig)
	assert.Equal(t, &Info{TagName: "container_id", Value: "acb54"}, metadata.ContainerID)
	assert.Equal(t, &Info{TagName: "location", Value: "unknown"}, metadata.Region)
	assert.Equal(t, &Info{TagName: "project_id", Value: "superprojectid"}, metadata.ProjectID)
}

func TestTagMap(t *testing.T) {
	metadata := GCPMetadata{
		ProjectID: &Info{
			TagName: "project_id",
			Value:   "myprojectid",
		},
		Region: &Info{
			TagName: "location",
			Value:   "mylocation",
		},
		ContainerID: &Info{
			TagName: "container_id",
			Value:   "f45ab",
		},
	}
	tagMap := metadata.TagMap()
	assert.Equal(t, 3, len(tagMap))
	assert.Equal(t, "myprojectid", tagMap["project_id"])
	assert.Equal(t, "mylocation", tagMap["location"])
	assert.Equal(t, "f45ab", tagMap["container_id"])
}
