// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
)

// ListSecurityProfiles lists the security profiles for the ManagerV2
func (m *ManagerV2) ListSecurityProfiles(_ *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	if !m.config.RuntimeSecurity.SecurityProfileV2Enabled {
		return &api.SecurityProfileListMessage{
			Error: ErrSecurityProfileManagerDisabled.Error(),
		}, ErrSecurityProfileManagerDisabled
	}

	var out api.SecurityProfileListMessage

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, p := range m.profiles {
		msg := p.ToSecurityProfileMessage(m.resolvers.TimeResolver)
		out.Profiles = append(out.Profiles, msg)
	}

	return &out, nil
}

// SaveSecurityProfile saves the requested security profile to disk for the ManagerV2
func (m *ManagerV2) SaveSecurityProfile(params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	if !m.config.RuntimeSecurity.SecurityProfileV2Enabled {
		return &api.SecurityProfileSaveMessage{
			Error: ErrSecurityProfileManagerDisabled.Error(),
		}, ErrSecurityProfileManagerDisabled
	}

	selector, err := cgroupModel.NewWorkloadSelector(params.GetSelector().GetName(), "*")
	if err != nil {
		return &api.SecurityProfileSaveMessage{
			Error: err.Error(),
		}, nil
	}

	m.profilesLock.Lock()
	p := m.profiles[selector]
	m.profilesLock.Unlock()

	if p == nil {
		return &api.SecurityProfileSaveMessage{
			Error: "security profile not found",
		}, nil
	}

	// encode profile
	raw, err := p.EncodeSecurityProfileProtobuf()
	if err != nil {
		return &api.SecurityProfileSaveMessage{
			Error: fmt.Sprintf("couldn't encode security profile in %s format: %v", config.Protobuf, err),
		}, nil
	}

	// write profile to encoded profile to disk
	f, err := os.CreateTemp("/tmp", p.Metadata.Name+"-*.profile")
	if err != nil {
		return nil, fmt.Errorf("couldn't create temporary file: %w", err)
	}
	defer f.Close()

	if _, err = f.Write(raw.Bytes()); err != nil {
		return nil, fmt.Errorf("couldn't write to temporary file: %w", err)
	}

	return &api.SecurityProfileSaveMessage{
		File: f.Name(),
	}, nil
}
