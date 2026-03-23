// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	fallbackStateConfigMapName = "spot-scheduler-state"
	disabledUntilKey           = "spot-disabled-until"
)

// fallbackStore persists and retrieves the spot-disabled-until timestamp.
type fallbackStore interface {
	// store persists the disabled-until timestamp.
	store(ctx context.Context, until time.Time) error
	// read retrieves the disabled-until timestamp. Returns zero time if not set.
	read(ctx context.Context) (time.Time, error)
}

type configMapFallbackStore struct {
	cmClient corev1client.ConfigMapInterface
	cmName   string
}

func newConfigMapFallbackStore(client kubernetes.Interface, namespace string) *configMapFallbackStore {
	return &configMapFallbackStore{
		cmClient: client.CoreV1().ConfigMaps(namespace),
		cmName:   fallbackStateConfigMapName,
	}
}

func (s *configMapFallbackStore) store(ctx context.Context, until time.Time) error {
	data := map[string]string{disabledUntilKey: until.Format(time.RFC3339)}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: fallbackStateConfigMapName},
		Data:       data,
	}
	_, err := s.cmClient.Update(ctx, cm, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		_, err = s.cmClient.Create(ctx, cm, metav1.CreateOptions{})
	}
	return err
}

func (s *configMapFallbackStore) read(ctx context.Context) (time.Time, error) {
	cm, err := s.cmClient.Get(ctx, s.cmName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	val := cm.Data[disabledUntilKey]
	if val == "" {
		return time.Time{}, nil
	}
	ts, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return time.Time{}, err
	}
	return ts, nil
}
