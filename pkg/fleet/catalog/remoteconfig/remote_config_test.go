// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/catalog"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestUpdateHandlerAppliesMergedSnapshotAndAcknowledgesUpdates(t *testing.T) {
	first := catalog.Package{Name: "first", Version: "1.0.0", URL: "https://example.com/first"}
	second := catalog.Package{Name: "second", Version: "2.0.0", URL: "https://example.com/second"}
	updates := map[string]state.RawConfig{
		"first":  rawCatalog(t, catalog.Catalog{Packages: []catalog.Package{first}}),
		"second": rawCatalog(t, catalog.Catalog{Packages: []catalog.Package{second}}),
	}

	var applied catalog.Catalog
	statuses := make(map[string]state.ApplyStatus)
	handler := NewUpdateHandler(func(c catalog.Catalog) error {
		applied = c
		return nil
	})
	handler(updates, func(path string, status state.ApplyStatus) {
		statuses[path] = status
	})

	if len(applied.Packages) != 2 || !containsPackage(applied.Packages, first) || !containsPackage(applied.Packages, second) {
		t.Fatalf("applied packages = %#v, want both input packages", applied.Packages)
	}
	wantStatuses := map[string]state.ApplyStatus{
		"first":  {State: state.ApplyStateAcknowledged},
		"second": {State: state.ApplyStateAcknowledged},
	}
	if !reflect.DeepEqual(statuses, wantStatuses) {
		t.Fatalf("apply statuses = %#v, want %#v", statuses, wantStatuses)
	}
}

func TestUpdateHandlerAppliesEmptySnapshot(t *testing.T) {
	called := false
	handler := NewUpdateHandler(func(c catalog.Catalog) error {
		called = true
		if len(c.Packages) != 0 {
			t.Fatalf("applied packages = %#v, want empty snapshot", c.Packages)
		}
		return nil
	})
	handler(map[string]state.RawConfig{}, func(string, state.ApplyStatus) {
		t.Fatal("applyStatus called for an empty update set")
	})

	if !called {
		t.Fatal("apply function was not called")
	}
}

func TestUpdateHandlerRejectsMalformedCatalog(t *testing.T) {
	applied := false
	statuses := make(map[string]state.ApplyStatus)
	handler := NewUpdateHandler(func(catalog.Catalog) error {
		applied = true
		return nil
	})
	handler(map[string]state.RawConfig{
		"invalid": {Config: []byte(`{"packages":`)},
		"valid":   rawCatalog(t, catalog.Catalog{}),
	}, func(path string, status state.ApplyStatus) {
		statuses[path] = status
	})

	if applied {
		t.Fatal("apply function called for a malformed catalog")
	}
	assertErrorStatus(t, statuses, "invalid")
}

func TestUpdateHandlerRejectsInvalidPackage(t *testing.T) {
	applied := false
	statuses := make(map[string]state.ApplyStatus)
	handler := NewUpdateHandler(func(catalog.Catalog) error {
		applied = true
		return nil
	})
	handler(map[string]state.RawConfig{
		"invalid": rawCatalog(t, catalog.Catalog{Packages: []catalog.Package{{
			Name:    "package",
			Version: "1.0.0",
			URL:     "oci://example.com/package:1.0.0",
		}}}),
	}, func(path string, status state.ApplyStatus) {
		statuses[path] = status
	})

	if applied {
		t.Fatal("apply function called for an invalid package")
	}
	assertErrorStatus(t, statuses, "invalid")
}

func TestUpdateHandlerReportsApplyErrorForEveryUpdate(t *testing.T) {
	wantErr := errors.New("could not store catalog")
	updates := map[string]state.RawConfig{
		"first":  rawCatalog(t, catalog.Catalog{}),
		"second": rawCatalog(t, catalog.Catalog{}),
	}
	statuses := make(map[string]state.ApplyStatus)
	handler := NewUpdateHandler(func(catalog.Catalog) error {
		return wantErr
	})
	handler(updates, func(path string, status state.ApplyStatus) {
		statuses[path] = status
	})

	for path := range updates {
		status, ok := statuses[path]
		if !ok {
			t.Fatalf("no apply status reported for %q", path)
		}
		if status.State != state.ApplyStateError || status.Error != wantErr.Error() {
			t.Fatalf("apply status for %q = %#v, want error %q", path, status, wantErr)
		}
	}
}

func TestUpdateHandlerRejectsNilApplyFunction(t *testing.T) {
	statuses := make(map[string]state.ApplyStatus)
	handler := NewUpdateHandler(nil)
	handler(map[string]state.RawConfig{
		"catalog": rawCatalog(t, catalog.Catalog{}),
	}, func(path string, status state.ApplyStatus) {
		statuses[path] = status
	})

	assertErrorStatus(t, statuses, "catalog")
}

func rawCatalog(t *testing.T, c catalog.Catalog) state.RawConfig {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("could not marshal test catalog: %v", err)
	}
	return state.RawConfig{Config: data}
}

func containsPackage(packages []catalog.Package, want catalog.Package) bool {
	for _, pkg := range packages {
		if pkg == want {
			return true
		}
	}
	return false
}

func assertErrorStatus(t *testing.T, statuses map[string]state.ApplyStatus, path string) {
	t.Helper()
	if len(statuses) != 1 {
		t.Fatalf("reported statuses = %#v, want one error status", statuses)
	}
	status, ok := statuses[path]
	if !ok {
		t.Fatalf("no apply status reported for %q", path)
	}
	if status.State != state.ApplyStateError || status.Error == "" {
		t.Fatalf("apply status = %#v, want a non-empty error", status)
	}
}
