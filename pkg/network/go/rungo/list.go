// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package rungo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// This URL contains a list of all Go versions
// https://pkg.go.dev/golang.org/x/website/internal/dl
const goVersionListURL string = "https://go.dev/dl/?mode=json&include=all"

type goRelease struct {
	Version string `json:"version"`
}

// ListGoVersions gets a list of all current Go versions by downloading the Golang download page
// and scanning it for Go versions.
// Includes beta and RC versions, as well as normal point releases.
// See https://golang.org/dl (all versions are listed under "Archived versions")
// or https://go.dev/dl/?mode=json&include=all
func ListGoVersions(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", goVersionListURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error constructing GET request to %s: %w", goVersionListURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making HTTP request to %s: %w", goVersionListURL, err)
	}

	defer resp.Body.Close()

	// Parse the body as a slice of shallow release structs
	// The full shape is at:
	// https://pkg.go.dev/golang.org/x/website/internal/dl#Release
	// but we're only interested in the version field.
	releases := []goRelease{}
	err = json.NewDecoder(resp.Body).Decode(&releases)
	if err != nil {
		return nil, fmt.Errorf("error decoding response from %s as JSON: %w", goVersionListURL, err)
	}

	// Flatten the list of structs to a list of versions,
	// and remove the leading "go" from the version strings.
	versions := make([]string, len(releases))
	for i := range releases {
		versions[i] = strings.TrimPrefix(releases[i].Version, "go")
	}

	return versions, nil
}
