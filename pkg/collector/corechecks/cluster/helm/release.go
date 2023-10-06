// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

// The "release" struct and the related ones, are a simplified version of the
// ones found in the Helm lib. Ref:
// https://github.com/helm/helm/blob/v3.8.0/pkg/release/release.go#L22
//
// Defining the structs here allows us to avoid importing Helm as a dependency.
// If in the future we need to support other storage backends or more advanced
// functionality, we'll need to rethink if the trade-off of not importing the
// Helm lib is still worth it.

type release struct {
	Name  string `json:"name,omitempty"`
	Info  *info  `json:"info,omitempty"`
	Chart *chart `json:"chart,omitempty"`
	// Config is the set of extra Values added to the chart.
	// These values override the default values inside of the chart.
	Config    map[string]interface{} `json:"config,omitempty"`
	Version   int                    `json:"version,omitempty"`
	Namespace string                 `json:"namespace,omitempty"`
}

type namespacedName string
type revision int

func (rel *release) namespacedName() namespacedName {
	return namespacedName(fmt.Sprintf("%s/%s", rel.Namespace, rel.Name))
}

func (rel *release) revision() revision {
	return revision(rel.Version)
}

type info struct {
	Status string `json:"status,omitempty"`
}

type chart struct {
	Metadata *metadata `json:"metadata"`
	// Values are default config for this chart.
	Values map[string]interface{} `json:"values"`
}

type metadata struct {
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
	AppVersion string `json:"appVersion,omitempty"`
}

// Note: the decodeRelease function has been copied from the helm library.
// It's private, so we can't reuse it.
// Ref: https://github.com/helm/helm/blob/v3.8.0/pkg/storage/driver/util.go#L56

var b64 = base64.StdEncoding
var magicGzip = []byte{0x1f, 0x8b, 0x08}

// decodeRelease decodes the bytes of data into a release
// type. Data must contain a base64 encoded gzipped string of a
// valid release, otherwise an error is returned.
func decodeRelease(data string) (*release, error) {
	// base64 decode string
	b, err := b64.DecodeString(data)
	if err != nil {
		return nil, err
	}
	if len(b) < 4 {
		// Avoid panic if b[0:3] cannot be accessed
		return nil, fmt.Errorf("The byte array is too short (expected at least 4 characters, got %s instead): it cannot contain a Helm release", fmt.Sprint(len(b)))
	}
	// For backwards compatibility with releases that were stored before
	// compression was introduced we skip decompression if the
	// gzip magic header is not found
	if bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b2, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}

	var rls release
	// unmarshal release object bytes
	if err := json.Unmarshal(b, &rls); err != nil {
		return nil, err
	}
	return &rls, nil
}

// getConfigValue returns the string value of a config param provided as a
// dot-separated key (for example "agents.image.tag"). It returns an error if
// the config param is not set.
func (rel *release) getConfigValue(dotSeparatedKey string) (string, error) {
	// We need to check rel.Config overrides what's set in rel.Chart.Values, so
	// we need to check rel.Config first.
	value, err := getValue(rel.Config, dotSeparatedKey)
	if err != nil {
		if rel.Chart == nil {
			return "", fmt.Errorf("not found")
		}

		value, err = getValue(rel.Chart.Values, dotSeparatedKey)
		if err != nil {
			return "", fmt.Errorf("not found")
		}
	}

	return value, nil
}
