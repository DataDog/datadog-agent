// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build functionaltests

package tests

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	"github.com/xeipuuv/gojsonschema"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/tests/schemas"
)

// AssetLoader schema loader from asset
type AssetFileSystem struct{}

// NewAssetFileSystem returns a new asset http.FileSystem
func NewAssetFileSystem() *AssetFileSystem {
	return &AssetFileSystem{}
}

// Open implements the http.FileSystem interface
func (a *AssetFileSystem) Open(name string) (http.File, error) {
	content, err := schemas.Asset(name)
	if err != nil {
		return nil, err
	}
	return &AssetFile{Reader: bytes.NewReader(content)}, nil
}

// AssetFile implements to File interface
type AssetFile struct {
	*bytes.Reader
}

// Close implements the http.FileSystem interface
func (f *AssetFile) Close() error {
	return nil
}

// Close implements the http.FileSystem interface
func (f *AssetFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

// Stat implements the http.FileSystem interface
func (f *AssetFile) Stat() (os.FileInfo, error) {
	return nil, nil
}

func validateSchema(t *testing.T, event *sprobe.Event, path string) bool {
	fs := NewAssetFileSystem()

	documentLoader := gojsonschema.NewStringLoader(event.String())
	schemaLoader := gojsonschema.NewReferenceLoaderFileSystem(path, fs)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Errorf("%s", desc)
		}
		return false
	}

	return true
}

func validateExecSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///exec.schema.json")
}

func validateOpenSchema(t *testing.T, event *sprobe.Event) bool {
	return validateSchema(t, event, "file:///open.schema.json")
}
