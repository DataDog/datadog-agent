// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package installers processes the installers_v2.json file
package installers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Arch is the architecture-specific URL for an installer
type Arch struct {
	URL string `json:"url"`
}

// Version contains the architecture-specific URLs for an installer version
// Example: {"x86_64": {...} }
type Version struct {
	Arch map[string]Arch
}

// Product contains the version-specific URLs for an installer product
// Example: {"7.50.0-1": {...} }
type Product struct {
	Version map[string]Version
}

// Installers contains the product-specific URLs for an installer
// Example: {"datadog-agent": {...} }
type Installers struct {
	URL      string
	Products map[string]Product
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (i *Installers) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &i.Products)
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (i *Product) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &i.Version)
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (i *Version) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &i.Arch)
}

func readURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// ListVersions returns a list of available product versions from a installers_v2.json URL
func ListVersions(url string) (*Installers, error) {
	body, err := readURL(url)
	if err != nil {
		return nil, err
	}

	var installers Installers
	installers.URL = url
	err = json.Unmarshal(body, &installers)
	if err != nil {
		return nil, err
	}

	return &installers, nil
}

// GetProductURL returns the URL for a product/version/arch pair from a installers_v2.json URL.
//
// It first tries an exact version match. If that fails, it tries the opposite "-1" suffix
// form (trimming it if present, or appending it if absent). This handles version format
// differences across flavors (e.g. base uses "7.75.0-1", fips uses "7.75.0").
func GetProductURL(url string, product string, version string, arch string) (string, error) {
	versions, err := ListVersions(url)
	if err != nil {
		return "", err
	}

	p, ok := versions.Products[product]
	if !ok {
		return "", fmt.Errorf("product %s not found", product)
	}

	v, ok := p.Version[version]
	if !ok {
		// Some flavors omit the "-1" suffix (e.g. fips uses "7.75.0" while base uses "7.75.0-1").
		// Try the opposite form before giving up.
		var alt string
		if strings.HasSuffix(version, "-1") {
			alt = strings.TrimSuffix(version, "-1")
		} else {
			alt = version + "-1"
		}
		v, ok = p.Version[alt]
		if !ok {
			return "", fmt.Errorf("version %s not found for product %s (also tried %s)", version, product, alt)
		}
	}

	a, ok := v.Arch[arch]
	if !ok {
		return "", fmt.Errorf("arch %s not found", arch)
	}

	return a.URL, nil
}
