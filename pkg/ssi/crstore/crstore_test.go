// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package crstore

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newAPMConfig(crName, crNamespace string) APMConfig {
	return APMConfig{
		CR:             types.NamespacedName{Namespace: crNamespace, Name: crName},
		Enabled:        true,
		TracerVersions: map[string]string{"java": "v1"},
		TracerConfigs:  []corev1.EnvVar{{Name: "DD_SERVICE", Value: "svc"}},
	}
}

func TestStoreGetAPM(t *testing.T) {
	target := WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
	config := newAPMConfig("ddi-web", "default")

	tests := []struct {
		name       string
		setup      func(*Store)
		target     WorkloadTarget
		wantConfig APMConfig
		wantOK     bool
	}{
		{
			name:       "missing entry",
			target:     target,
			wantConfig: APMConfig{},
			wantOK:     false,
		},
		{
			name: "existing entry",
			setup: func(s *Store) {
				s.UpsertAPM(target, config)
			},
			target:     target,
			wantConfig: config,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			if tt.setup != nil {
				tt.setup(s)
			}

			got, ok := s.GetAPM(tt.target)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantConfig, got)
		})
	}
}

func TestStoreUpsertAPM(t *testing.T) {
	target := WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
	config := newAPMConfig("ddi-web", "default")
	replacement := config
	replacement.TracerVersions = map[string]string{"python": "v4"}

	tests := []struct {
		name       string
		configs    []APMConfig
		wantConfig APMConfig
	}{
		{
			name:       "stores config for target",
			configs:    []APMConfig{config},
			wantConfig: config,
		},
		{
			name:       "replaces config for target",
			configs:    []APMConfig{config, replacement},
			wantConfig: replacement,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			for _, config := range tt.configs {
				s.UpsertAPM(target, config)
			}

			got, ok := s.GetAPM(target)
			require.True(t, ok)
			require.Equal(t, tt.wantConfig, got)
		})
	}
}

func TestStoreDeleteByCR(t *testing.T) {
	target := WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
	config := newAPMConfig("ddi-web", "default")

	tests := []struct {
		name       string
		deleteCR   types.NamespacedName
		wantConfig APMConfig
		wantOK     bool
	}{
		{
			name:       "removes config sourced from CR",
			deleteCR:   config.CR,
			wantConfig: APMConfig{},
			wantOK:     false,
		},
		{
			name:       "ignores wrong CR",
			deleteCR:   types.NamespacedName{Namespace: "default", Name: "wrong"},
			wantConfig: config,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			s.UpsertAPM(target, config)
			s.DeleteByCR(tt.deleteCR)

			got, ok := s.GetAPM(target)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantConfig, got)
		})
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := New()
	const workers = 16
	const iterations = 200

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			target := WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
			config := newAPMConfig("ddi", "default")
			for j := range iterations {
				switch j % 3 {
				case 0:
					s.UpsertAPM(target, config)
				case 1:
					_, _ = s.GetAPM(target)
				case 2:
					s.DeleteByCR(config.CR)
				}
			}
		}()
	}
	wg.Wait()
}
