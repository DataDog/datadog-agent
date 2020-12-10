// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package mock

// Registry does nothing
type Registry struct {
	offset      string
	tailingMode string
	configID    string
	identifier  string
}

// NewRegistry returns a new registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// GetOffset returns the offset.
func (r *Registry) GetOffset(identifier string) string {
	return r.offset
}

// SetOffset sets the offset.
func (r *Registry) SetOffset(offset string) {
	r.offset = offset
}

// GetTailingMode returns the tailing mode.
func (r *Registry) GetTailingMode(identifier string) string {
	return r.tailingMode
}

// SetTailingMode sets the tailing mode.
func (r *Registry) SetTailingMode(tailingMode string) {
	r.tailingMode = tailingMode
}

// SetConfigID allow only one identifier to update an offset
func (r *Registry) SetConfigID(identifier, configID string) {
	r.configID = configID
	r.identifier = identifier
}

// GetConfigID get the config identifier
func (r *Registry) GetConfigID() string {
	return r.configID
}

// GetIdentifier get the config identifier
func (r *Registry) GetIdentifier() string {
	return r.identifier
}
