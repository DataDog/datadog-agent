// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package enrollment

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFollowerWaitsForWIFReplacement(t *testing.T) {
	for _, tc := range []struct{ name, org, site string }{
		{"API-key identity", "", ""},
		{"different org", "previous-org", "datadoghq.com"},
		{"different site", "test-org", "datadoghq.eu"},
		{"missing org", "", "datadoghq.com"},
		{"missing site", "test-org", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"delegated_auth.org_uuid": "test-org", "site": "datadoghq.com", par.SelfEnroll: true,
			})
			storage := newK8sIdentityStorage(cfg, nil)
			synctest.Test(t, func(t *testing.T) {
				secret := makeTestSecret("default", "identity")
				secret.Data[wifOrgUUIDField] = []byte(tc.org)
				secret.Data[wifSiteField] = []byte(tc.site)
				client := fake.NewSimpleClientset(secret)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var got *corev1.Secret
				var loadErr error
				done := make(chan struct{})
				go func() {
					defer close(done)
					got, loadErr = waitForLeaderAndSecret(ctx, make(chan struct{}), func() bool { return false }, client, "default", "identity", time.Second, storage.acceptsSecret)
				}()
				// A follower must not use a Secret enrolled for different or unknown WIF configuration.
				time.Sleep(2 * time.Second)
				synctest.Wait()
				select {
				case <-done:
					t.Fatal("follower accepted the old identity")
				default:
				}
				replacement := secret.DeepCopy()
				replacement.Data[urnField] = []byte("new-runner")
				replacement.Data[privateKeyField] = []byte("new-key")
				replacement.Data[wifOrgUUIDField] = []byte("test-org")
				replacement.Data[wifSiteField] = []byte("datadoghq.com")
				_, err := client.CoreV1().Secrets("default").Update(ctx, replacement, metav1.UpdateOptions{})
				require.NoError(t, err)
				time.Sleep(time.Second)
				synctest.Wait()
				select {
				case <-done:
					require.NoError(t, loadErr)
					require.NotNil(t, got)
					require.Equal(t, "new-runner", string(got.Data[urnField]))
					require.Equal(t, "new-key", string(got.Data[privateKeyField]))
				default:
					t.Fatal("follower did not load the replacement")
				}
			})
		})
	}
}

func TestWIFReplacementLeaderAndCancellation(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"delegated_auth.org_uuid": "test-org", "site": "datadoghq.com", par.SelfEnroll: true,
	})
	storage := newK8sIdentityStorage(cfg, nil)
	for _, leader := range []bool{true, false} {
		t.Run(map[bool]string{true: "leader", false: "cancelled follower"}[leader], func(t *testing.T) {
			secret := makeTestSecret("default", "identity")
			client := fake.NewSimpleClientset(secret)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !leader {
				cancel()
			}
			got, err := waitForLeaderAndSecret(ctx, make(chan struct{}), func() bool { return leader }, client, "default", "identity", time.Second, storage.acceptsSecret)
			require.Nil(t, got)
			if leader {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			saved, err := client.CoreV1().Secrets("default").Get(context.Background(), "identity", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, secret.Data, saved.Data, "do not delete or modify the old Secret before enrollment")
		})
	}
}
