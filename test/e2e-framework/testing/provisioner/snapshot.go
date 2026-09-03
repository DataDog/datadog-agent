// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package provisioner

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// A snapshot is the on-disk currency of a provisioned environment: a single JSON
// object whose top-level keys are resource names, each value being the raw JSON
// payload of that resource. Keys prefixed with "_" are metadata. The format is
// the one read back by provisioners.StaticStackProvisioner, so an environment
// saved with WriteSnapshotFile can be re-attached to a typed environment with
// no Pulumi interaction.
//
// Example:
//
//	{
//	  "_source": "local-kind",
//	  "_created": "2026-09-02T10:00:00Z",
//	  "kubernetesCluster": { "clusterName": "kind", "kubeConfig": "..." },
//	  "fakeIntake":          { "host": "192.168.1.10", "scheme": "http", "port": 30080, "url": "..." }
//	}

// SnapshotMetadataPrefix is the prefix of snapshot keys that carry metadata
// instead of resources.
const SnapshotMetadataPrefix = "_"

// WriteSnapshotFile writes resources and metadata to path as a snapshot file.
// meta keys are stored as-is; keys not starting with "_" are rejected to keep
// the resource namespace clean.
func WriteSnapshotFile(path string, resources RawResources, meta map[string]any) error {
	out := make(map[string]json.RawMessage, len(resources)+len(meta))
	for k, v := range resources {
		if strings.HasPrefix(k, SnapshotMetadataPrefix) {
			return fmt.Errorf("resource name %q must not start with %q", k, SnapshotMetadataPrefix)
		}
		if !json.Valid(v) {
			return fmt.Errorf("resource %q is not valid JSON", k)
		}
		out[k] = json.RawMessage(v)
	}
	for k, v := range meta {
		if !strings.HasPrefix(k, SnapshotMetadataPrefix) {
			k = SnapshotMetadataPrefix + k
		}
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encoding metadata %q: %w", k, err)
		}
		out[k] = encoded
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadSnapshotFile reads a snapshot file into its raw resources and metadata.
func ReadSnapshotFile(path string) (resources RawResources, meta map[string]json.RawMessage, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(data, &topLevel); err != nil {
		return nil, nil, fmt.Errorf("parsing snapshot %s: %w", path, err)
	}
	resources = RawResources{}
	meta = map[string]json.RawMessage{}
	for k, v := range topLevel {
		if strings.HasPrefix(k, SnapshotMetadataPrefix) {
			meta[k] = v
		} else {
			resources[k] = v
		}
	}
	return resources, meta, nil
}

// ReadSnapshotResource decodes the resource stored under key in the snapshot at
// path into target.
func ReadSnapshotResource(path, key string, target any) error {
	resources, _, err := ReadSnapshotFile(path)
	if err != nil {
		return err
	}
	raw, ok := resources[key]
	if !ok {
		return fmt.Errorf("snapshot %s has no %q resource", path, key)
	}
	return json.Unmarshal(raw, target)
}
