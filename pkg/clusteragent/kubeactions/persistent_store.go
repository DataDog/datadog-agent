// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// ConfigMapName is the name of the ConfigMap used to store action execution history
	ConfigMapName = "datadog-kubeactions-state"
	// ConfigMapNamespace is the namespace where the ConfigMap is stored
	ConfigMapNamespace = "default" // TODO: Make this configurable
	// ConfigMapDataKey is the key in the ConfigMap data
	ConfigMapDataKey = "executed-actions"
	// PersistInterval is how often we persist to ConfigMap
	PersistInterval = 30 * time.Second
)

// PersistentActionStore extends ActionStore with persistence to a Kubernetes ConfigMap
type PersistentActionStore struct {
	*ActionStore
	clientset kubernetes.Interface
	namespace string
	ticker    *time.Ticker
	stopCh    chan struct{}
	mu        sync.Mutex
	dirty     bool // tracks if there are unsaved changes
}

// NewPersistentActionStore creates a new PersistentActionStore
func NewPersistentActionStore(ctx context.Context, clientset kubernetes.Interface, namespace string) (*PersistentActionStore, error) {
	store := &PersistentActionStore{
		ActionStore: NewActionStore(),
		clientset:   clientset,
		namespace:   namespace,
		stopCh:      make(chan struct{}),
	}

	// Load existing state from ConfigMap
	if err := store.load(ctx); err != nil {
		log.Warnf("Failed to load action state from ConfigMap: %v", err)
		// Continue anyway with empty state
	}

	// Start background persistence goroutine
	store.ticker = time.NewTicker(PersistInterval)
	go store.persistLoop(ctx)

	log.Infof("Created persistent action store with ConfigMap %s/%s", namespace, ConfigMapName)
	return store, nil
}

// MarkExecuted marks an action as executed and flags for persistence
func (s *PersistentActionStore) MarkExecuted(key ActionKey, status, message string, timestamp int64) {
	s.ActionStore.MarkExecuted(key, status, message, timestamp)
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}

// persistLoop periodically persists the action store to ConfigMap
func (s *PersistentActionStore) persistLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Final persist before shutdown
			if err := s.persist(ctx); err != nil {
				log.Errorf("Failed to persist action state on shutdown: %v", err)
			}
			return
		case <-s.stopCh:
			return
		case <-s.ticker.C:
			s.mu.Lock()
			dirty := s.dirty
			s.mu.Unlock()

			if dirty {
				if err := s.persist(ctx); err != nil {
					log.Errorf("Failed to persist action state: %v", err)
				} else {
					s.mu.Lock()
					s.dirty = false
					s.mu.Unlock()
				}
			}
		}
	}
}

// persist saves the current action store state to ConfigMap
func (s *PersistentActionStore) persist(ctx context.Context) error {
	records := s.GetAll()

	// Serialize to JSON
	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("failed to marshal action records: %w", err)
	}

	// Get or create ConfigMap
	cm, err := s.clientset.CoreV1().ConfigMaps(s.namespace).Get(ctx, ConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new ConfigMap
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapName,
					Namespace: s.namespace,
				},
				Data: map[string]string{
					ConfigMapDataKey: string(data),
				},
			}
			_, err = s.clientset.CoreV1().ConfigMaps(s.namespace).Create(ctx, cm, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create ConfigMap: %w", err)
			}
			log.Debugf("Created ConfigMap %s/%s for action state", s.namespace, ConfigMapName)
			return nil
		}
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Update existing ConfigMap
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[ConfigMapDataKey] = string(data)

	_, err = s.clientset.CoreV1().ConfigMaps(s.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	log.Debugf("Persisted %d action records to ConfigMap", len(records))
	return nil
}

// load restores the action store state from ConfigMap
func (s *PersistentActionStore) load(ctx context.Context) error {
	cm, err := s.clientset.CoreV1().ConfigMaps(s.namespace).Get(ctx, ConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Debugf("ConfigMap %s/%s not found, starting with empty state", s.namespace, ConfigMapName)
			return nil
		}
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	data, ok := cm.Data[ConfigMapDataKey]
	if !ok || data == "" {
		log.Debugf("No action data in ConfigMap, starting with empty state")
		return nil
	}

	// Deserialize from JSON
	var records []ActionRecord
	if err := json.Unmarshal([]byte(data), &records); err != nil {
		return fmt.Errorf("failed to unmarshal action records: %w", err)
	}

	// Restore to store
	for _, record := range records {
		s.ActionStore.MarkExecuted(record.Key, record.Status, record.Message, record.Timestamp)
	}

	log.Infof("Loaded %d action records from ConfigMap", len(records))
	return nil
}

// Stop stops the persistence goroutine
func (s *PersistentActionStore) Stop(ctx context.Context) error {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)

	// Final persist
	return s.persist(ctx)
}
