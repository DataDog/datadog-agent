// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ssi"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newDDITarget(crName, crNamespace string) ssi.DDITarget {
	return ssi.DDITarget{
		CR:             types.NamespacedName{Namespace: crNamespace, Name: crName},
		Enabled:        true,
		TracerVersions: map[string]string{"java": "v1"},
		TracerConfigs:  []corev1.EnvVar{{Name: "DD_SERVICE", Value: "svc"}},
	}
}

func TestAPMTargetStoreGetTarget(t *testing.T) {
	target := ssi.WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
	config := newDDITarget("ddi-web", "default")
	var nilStore *APMTargetStore

	tests := []struct {
		name       string
		store      *APMTargetStore
		setup      func(*APMTargetStore)
		target     ssi.WorkloadTarget
		wantConfig ssi.DDITarget
		wantOK     bool
	}{
		{
			name:       "missing entry",
			store:      NewAPMTargetStore(),
			target:     target,
			wantConfig: ssi.DDITarget{},
			wantOK:     false,
		},
		{
			name:  "existing entry",
			store: NewAPMTargetStore(),
			setup: func(s *APMTargetStore) {
				s.UpsertTarget(target, config)
			},
			target:     target,
			wantConfig: config,
			wantOK:     true,
		},
		{
			name:       "nil store",
			store:      nilStore,
			target:     target,
			wantConfig: ssi.DDITarget{},
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.store
			if tt.setup != nil {
				tt.setup(s)
			}

			got, ok := s.GetTarget(tt.target)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantConfig, got)
		})
	}
}

func TestAPMTargetStoreUpsertTarget(t *testing.T) {
	target := ssi.WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
	config := newDDITarget("ddi-web", "default")
	replacement := config
	replacement.TracerVersions = map[string]string{"python": "v4"}

	tests := []struct {
		name       string
		configs    []ssi.DDITarget
		wantConfig ssi.DDITarget
	}{
		{
			name:       "stores config for target",
			configs:    []ssi.DDITarget{config},
			wantConfig: config,
		},
		{
			name:       "replaces config for target",
			configs:    []ssi.DDITarget{config, replacement},
			wantConfig: replacement,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewAPMTargetStore()
			for _, config := range tt.configs {
				s.UpsertTarget(target, config)
			}

			got, ok := s.GetTarget(target)
			require.True(t, ok)
			require.Equal(t, tt.wantConfig, got)
		})
	}
}

func TestAPMTargetStoreDeleteByCR(t *testing.T) {
	target := ssi.WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
	config := newDDITarget("ddi-web", "default")

	tests := []struct {
		name       string
		deleteCR   types.NamespacedName
		wantConfig ssi.DDITarget
		wantOK     bool
	}{
		{
			name:       "removes config sourced from CR",
			deleteCR:   config.CR,
			wantConfig: ssi.DDITarget{},
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
			s := NewAPMTargetStore()
			s.UpsertTarget(target, config)
			s.DeleteByCR(tt.deleteCR)

			got, ok := s.GetTarget(target)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantConfig, got)
		})
	}
}

func TestAPMTargetStoreConcurrentAccess(_ *testing.T) {
	s := NewAPMTargetStore()
	const workers = 16
	const iterations = 200

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			target := ssi.WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
			config := newDDITarget("ddi", "default")
			for j := range iterations {
				switch j % 3 {
				case 0:
					s.UpsertTarget(target, config)
				case 1:
					_, _ = s.GetTarget(target)
				case 2:
					s.DeleteByCR(config.CR)
				}
			}
		}()
	}
	wg.Wait()
}
