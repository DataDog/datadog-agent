// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestRestartUntilPodErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operation string
		skip      int
	}{
		{name: "find pod", operation: "list"},
		{name: "delete pod", operation: "delete"},
		{name: "find replacement", operation: "list", skip: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := fake.NewClientset(&corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "app-old", Namespace: "test"}})
				wantErr := errors.New("pod operation failed")
				calls := 0
				client.PrependReactor(tc.operation, "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
					calls++
					if calls <= tc.skip {
						return false, nil, nil
					}
					return true, nil, wantErr
				})

				start := time.Now()
				_, err := restartUntil(context.Background(), client, "test", "app", func(*corev1.Pod) bool {
					t.Fatal("readiness must not be checked after a pod-operation error")
					return false
				})
				require.ErrorIs(t, err, wantErr)
				require.Equal(t, tc.skip+1, calls, "pod-operation errors must not be retried")
				require.Zero(t, time.Since(start), "pod-operation errors must return without waiting for a timeout")
			})
		})
	}
}

func TestRestartUntilReplacementTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := fake.NewClientset(&corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "app-old", Namespace: "test"}})
		start := time.Now()
		_, err := restartUntil(context.Background(), client, "test", "app", func(*corev1.Pod) bool {
			t.Fatal("readiness must not be checked without a running replacement")
			return false
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorContains(t, err, "pod app was not recreated in namespace test")
		require.Equal(t, 4*time.Minute, time.Since(start), "report the replacement timeout before the outer admission timeout")
	})
}

func TestRestartUntilAdmissionConverges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := fake.NewClientset(&corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "app-old", Namespace: "test"}})
		pods := corev1.SchemeGroupVersion.WithResource("pods")
		restarts := 0
		client.PrependReactor("delete", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			name := action.(k8stesting.DeleteAction).GetName()
			require.NoError(t, client.Tracker().Delete(pods, "test", name))
			restarts++
			pod := &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{Name: fmt.Sprintf("app-%d", restarts), Namespace: "test"},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "app",
					Env:  []corev1.EnvVar{{Name: "DD_INSTRUMENTATION_INSTALL_TYPE", Value: "k8s_lib_injection"}},
				}}},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}
			if restarts == 2 {
				pod.Spec.Containers[0].Env[0].Value = "k8s_single_step"
			}
			require.NoError(t, client.Tracker().Create(pods, pod, "test"))
			return true, nil, nil
		})

		pod, err := restartUntil(context.Background(), client, "test", "app", hasInstallType("app", "k8s_single_step"))
		require.NoError(t, err)
		require.Equal(t, 2, restarts)
		require.Equal(t, "app-2", pod.Name)
	})
}
