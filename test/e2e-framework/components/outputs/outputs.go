// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package outputs holds the Pulumi-free side of the e2e-framework components:
// the import contract (Importable / JSONImporter) and the *Output structs that
// snapshots and typed environments exchange.
//
// These types used to live inside the Pulumi component packages, which meant
// every consumer of an Output struct — environments, installers, standalone
// drivers, CLIs — transitively linked the whole Pulumi SDK. The Pulumi
// packages now re-export them as aliases, so existing imports keep working;
// Pulumi-free consumers import this package instead.
package outputs

import "encoding/json"

// Importable needs to be implemented by the fully resolved type used outside of Pulumi.
type Importable interface {
	SetKey(string)
	Key() string
	Import(in []byte, obj any) error
}

// JSONImporter is the standard Importable implementation: plain JSON unmarshalling.
type JSONImporter struct {
	key string
}

var _ Importable = &JSONImporter{}

// SetKey sets the resource key this component is imported from.
func (imp *JSONImporter) SetKey(key string) { imp.key = key }

// Key returns the resource key this component is imported from.
func (imp *JSONImporter) Key() string { return imp.key }

// Import unmarshals the raw resource payload into obj.
func (imp *JSONImporter) Import(in []byte, obj any) error { return json.Unmarshal(in, obj) }

// CloudProviderIdentifier identifies the cloud a resource belongs to.
type CloudProviderIdentifier string

const (
	CloudProviderAWS   CloudProviderIdentifier = "aws"
	CloudProviderAzure CloudProviderIdentifier = "azure"
	CloudProviderGCP   CloudProviderIdentifier = "gcp"
)
