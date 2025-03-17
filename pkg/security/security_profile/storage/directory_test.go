// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package storage holds files related to storages for security profiles
package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

type profileWithSelector struct {
	profile  *profile.Profile
	selector cgroupModel.WorkloadSelector
}

func newProfileWithSelector(name string, imageName string, imageTag string) *profileWithSelector {
	pws := &profileWithSelector{}
	pws.selector.Image = imageName
	pws.selector.Tag = imageTag
	pws.profile = profile.New(profile.WithWorkloadSelector(pws.selector))
	pws.profile.Metadata.Name = name
	return pws
}

func TestDirectory(t *testing.T) {
	testDir := t.TempDir()
	const maxDirEntries = 2

	storageRequests := []config.StorageRequest{
		{
			Type:            config.LocalStorage,
			Format:          config.JSON,
			OutputDirectory: testDir,
		},
		{
			Type:            config.LocalStorage,
			Format:          config.Profile,
			OutputDirectory: testDir,
		},
	}

	profile1 := newProfileWithSelector("profile1", "image1", "tag1")
	profile2 := newProfileWithSelector("profile2", "image2", "tag2")
	profile3 := newProfileWithSelector("profile3", "image3", "tag3")

	d, err := NewDirectory(testDir, maxDirEntries)
	require.NoError(t, err)

	t.Run("persist", func(t *testing.T) {
		// Check that profiles can be persisted
		for _, p := range []*profileWithSelector{profile1, profile2, profile3} {
			for _, sr := range storageRequests {
				b, err := p.profile.Encode(sr.Format)
				require.NoErrorf(t, err, "failed to encode profile %s", p.profile.Metadata.Name)
				err = d.Persist(sr, p.profile, b)
				require.NoErrorf(t, err, "failed to persist profile %s", p.profile.Metadata.Name)
			}
		}
	})

	t.Run("load", func(t *testing.T) {
		// Check that profiles can be loaded
		for _, p := range []*profileWithSelector{profile2, profile3} {
			// Check that files for profile2 and profile3 exist in the directory
			for _, sr := range storageRequests {
				expectedPath := filepath.Join(testDir, p.profile.Metadata.Name+"."+sr.Format.String())
				fi, err := os.Stat(expectedPath)
				require.NoErrorf(t, err, "failed to stat file %s", expectedPath)
				assert.Greater(t, fi.Size(), int64(0), "file %s is empty", expectedPath)
			}

			// Check that profile2 and profile3 can be loaded
			loaded := profile.New()
			ok, err := d.Load(&p.selector, loaded)
			require.NoErrorf(t, err, "failed to load profile %s", p.profile.Metadata.Name)
			require.True(t, ok, "failed to load profile %s", p.profile.Metadata.Name)
			require.NotNilf(t, loaded.GetWorkloadSelector(), "failed to get selector for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.selector.Image, loaded.GetWorkloadSelector().Image, "failed to match image for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.selector.Tag, loaded.GetWorkloadSelector().Tag, "failed to match tag for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.profile.Metadata.Name, loaded.Metadata.Name, "failed to match name for profile %s", p.profile.Metadata.Name)
		}
	})

	t.Run("eviction", func(t *testing.T) {
		for _, p := range []*profileWithSelector{profile1} {
			// Check that profile1 cannot be loaded
			loaded := profile.New()
			ok, err := d.Load(&p.selector, loaded)
			require.NoErrorf(t, err, "no profile should not return any error %s", p.profile.Metadata.Name)
			assert.Falsef(t, ok, "profile should not be loaded %s", p.profile.Metadata.Name)

			// Check that profile1 files were removed from the directory
			for _, sr := range storageRequests {
				expectedPath := filepath.Join(testDir, p.profile.Metadata.Name+"."+sr.Format.String())
				_, err := os.Stat(expectedPath)
				assert.ErrorIsf(t, err, os.ErrNotExist, "file %s should not exist", expectedPath)
			}
		}
	})

	t.Run("wildcard", func(t *testing.T) {
		// Check that profile2 and profile3 can be loaded through a wildcard selector
		for _, p := range []*profileWithSelector{profile2, profile3} {
			loaded := profile.New()
			wildCardSelector := cgroupModel.WorkloadSelector{Image: p.selector.Image, Tag: "*"}
			ok, err := d.Load(&wildCardSelector, loaded)
			require.NoErrorf(t, err, "failed to load profile %s", p.profile.Metadata.Name)
			require.True(t, ok, "failed to load profile %s", p.profile.Metadata.Name)
			require.NotNilf(t, loaded.GetWorkloadSelector(), "failed to get selector for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.selector.Image, loaded.GetWorkloadSelector().Image, "failed to match image for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.selector.Tag, loaded.GetWorkloadSelector().Tag, "failed to match tag for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.profile.Metadata.Name, loaded.Metadata.Name, "failed to match name for profile %s", p.profile.Metadata.Name)
		}
	})

	t.Run("startup", func(t *testing.T) {
		// Check that profiles can be loaded at startup
		d1, err := NewDirectory(testDir, maxDirEntries)
		require.NoError(t, err)
		for _, p := range []*profileWithSelector{profile2, profile3} {
			// Check that profile2 and profile3 can be loaded
			loaded := profile.New()
			ok, err := d1.Load(&p.selector, loaded)
			require.NoErrorf(t, err, "failed to load profile %s", p.profile.Metadata.Name)
			require.True(t, ok, "failed to load profile %s", p.profile.Metadata.Name)
			require.NotNilf(t, loaded.GetWorkloadSelector(), "failed to get selector for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.selector.Image, loaded.GetWorkloadSelector().Image, "failed to match image for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.selector.Tag, loaded.GetWorkloadSelector().Tag, "failed to match tag for profile %s", p.profile.Metadata.Name)
			assert.Equalf(t, p.profile.Metadata.Name, loaded.Metadata.Name, "failed to match name for profile %s", p.profile.Metadata.Name)
		}
	})
}
