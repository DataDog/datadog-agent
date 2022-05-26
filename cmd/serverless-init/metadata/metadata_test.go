// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	assert.Equal(t, 300*time.Millisecond, GetDefaultConfig().timeout)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/id", GetDefaultConfig().ContainerIDUrl)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/project/project-id", GetDefaultConfig().ProjectIDUrl)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/region", GetDefaultConfig().RegionUrl)
}

func TestGetSingleMetadataMalformedUrl(t *testing.T) {
	testConfig := &Config{
		timeout:        1 * time.Millisecond,
		ContainerIDUrl: string([]byte("\u007F")),
	}
	assert.Equal(t, "unknown", getSingleMetadata(testConfig.ContainerIDUrl, testConfig.timeout))
}

func TestSingleMedataTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer ts.Close()
	testConfig := &Config{
		timeout:        1 * time.Millisecond,
		ContainerIDUrl: ts.URL,
	}
	assert.Equal(t, "unknown", getSingleMetadata(testConfig.ContainerIDUrl, testConfig.timeout))
}

func TestSingleMedataOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("1234"))
	}))
	defer ts.Close()
	testConfig := &Config{
		timeout:        1 * time.Second,
		ContainerIDUrl: ts.URL,
	}
	assert.Equal(t, "1234", getSingleMetadata(testConfig.ContainerIDUrl, testConfig.timeout))
}

func TestGetContainerID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("1234"))
	}))
	defer ts.Close()
	testConfig := &Config{
		timeout:        1 * time.Second,
		ContainerIDUrl: ts.URL,
	}
	assert.Equal(t, &MetadataInfo{tagName: "containerid", value: "1234"}, getContainerID(testConfig))
}

func TestGetRegion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("superRegion"))
	}))
	defer ts.Close()
	testConfig := &Config{
		timeout:   1 * time.Second,
		RegionUrl: ts.URL,
	}
	assert.Equal(t, &MetadataInfo{tagName: "region", value: "superregion"}, getRegion(testConfig))
}

func TestGetProjectID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("superproject"))
	}))
	defer ts.Close()
	testConfig := &Config{
		timeout:      1 * time.Second,
		ProjectIDUrl: ts.URL,
	}
	assert.Equal(t, &MetadataInfo{tagName: "projectid", value: "superproject"}, getProjectID(testConfig))
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

	testConfig := &Config{
		timeout:        1 * time.Second,
		ProjectIDUrl:   tsProjectID.URL,
		RegionUrl:      tsRegion.URL,
		ContainerIDUrl: tsContainerID.URL,
	}

	metadata := GetMetaData(testConfig)
	assert.Equal(t, &MetadataInfo{tagName: "containerid", value: "acb54"}, metadata.ContainerID)
	assert.Equal(t, &MetadataInfo{tagName: "region", value: "greatregion"}, metadata.Region)
	assert.Equal(t, &MetadataInfo{tagName: "projectid", value: "superprojectid"}, metadata.ProjectID)
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

	testConfig := &Config{
		timeout:        500 * time.Millisecond,
		ProjectIDUrl:   tsProjectID.URL,
		RegionUrl:      tsRegion.URL,
		ContainerIDUrl: tsContainerID.URL,
	}

	metadata := GetMetaData(testConfig)
	assert.Equal(t, &MetadataInfo{tagName: "containerid", value: "acb54"}, metadata.ContainerID)
	assert.Nil(t, metadata.Region)
	assert.Equal(t, &MetadataInfo{tagName: "projectid", value: "superprojectid"}, metadata.ProjectID)
}

func TestTagMap(t *testing.T) {
	metadata := Metadata{
		ProjectID: &MetadataInfo{
			tagName: "projectid",
			value:   "myprojectid",
		},
		Region: &MetadataInfo{
			tagName: "region",
			value:   "myregion",
		},
		ContainerID: &MetadataInfo{
			tagName: "containerid",
			value:   "f45ab",
		},
	}
	tagMap := metadata.TagMap()
	assert.Equal(t, 3, len(tagMap))
	assert.Equal(t, "myprojectid", tagMap["projectid"])
	assert.Equal(t, "myregion", tagMap["region"])
	assert.Equal(t, "f45ab", tagMap["containerid"])
}
