// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package compliance

import (
	"context"
	"sync"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// ReflectorStore maintains a filtered cache of RoleBindings and
// ClusterRoleBindings that reference "default" ServiceAccounts.
//
// This store is used to optimize compliance rule cis-kubernetes-1.5.1-5.1.5
// which needs to check if the default ServiceAccounts are used. Instead of
// fetching all objects every check interval, which can cause memory spikes in
// large clusters, we use reflectors to watch for changes and only store
// bindings that reference default ServiceAccounts.
type ReflectorStore struct {
	roleBindingStore        *roleBindingStore
	clusterRoleBindingStore *clusterRoleBindingStore

	roleBindingReflector        *cache.Reflector
	clusterRoleBindingReflector *cache.Reflector
}

// NewReflectorStore creates a new store with reflectors watching RoleBindings
// and ClusterRoleBindings. Call Run() to start the reflectors.
func NewReflectorStore(client kubernetes.Interface) *ReflectorStore {
	store := &ReflectorStore{
		roleBindingStore:        newRoleBindingStore(),
		clusterRoleBindingStore: newClusterRoleBindingStore(),
	}

	roleBindingListWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			return client.RbacV1().RoleBindings(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return client.RbacV1().RoleBindings(metav1.NamespaceAll).Watch(ctx, options)
		},
	}
	store.roleBindingReflector = cache.NewReflector(roleBindingListWatch, &rbacv1.RoleBinding{}, store.roleBindingStore, 0)

	clusterRoleBindingListWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			return client.RbacV1().ClusterRoleBindings().List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return client.RbacV1().ClusterRoleBindings().Watch(ctx, options)
		},
	}
	store.clusterRoleBindingReflector = cache.NewReflector(clusterRoleBindingListWatch, &rbacv1.ClusterRoleBinding{}, store.clusterRoleBindingStore, 0)

	return store
}

// Run starts the reflectors.
func (s *ReflectorStore) Run(stopCh <-chan struct{}) {
	go s.roleBindingReflector.Run(stopCh)
	go s.clusterRoleBindingReflector.Run(stopCh)
}

// HasSynced returns true if all reflectors have synced.
func (s *ReflectorStore) HasSynced() bool {
	return s.roleBindingStore.HasSynced() && s.clusterRoleBindingStore.HasSynced()
}

// GetRoleBindings returns stored RoleBindings that reference default ServiceAccounts.
func (s *ReflectorStore) GetRoleBindings() []*rbacv1.RoleBinding {
	return s.roleBindingStore.GetFiltered()
}

// GetClusterRoleBindings returns stored ClusterRoleBindings that reference default ServiceAccounts.
func (s *ReflectorStore) GetClusterRoleBindings() []*rbacv1.ClusterRoleBinding {
	return s.clusterRoleBindingStore.GetFiltered()
}

// referencesDefaultServiceAccount returns true if any subject in the list is a
// ServiceAccount named "default".
func referencesDefaultServiceAccount(subjects []rbacv1.Subject) bool {
	for _, subject := range subjects {
		if subject.Kind == rbacv1.ServiceAccountKind && subject.Name == "default" {
			return true
		}
	}
	return false
}

type roleBindingStore struct {
	mu        sync.RWMutex
	items     map[string]*rbacv1.RoleBinding
	hasSynced bool
}

func newRoleBindingStore() *roleBindingStore {
	return &roleBindingStore{
		items: make(map[string]*rbacv1.RoleBinding),
	}
}

func (s *roleBindingStore) HasSynced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasSynced
}

func (s *roleBindingStore) GetFiltered() []*rbacv1.RoleBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*rbacv1.RoleBinding, 0, len(s.items))
	for _, rb := range s.items {
		result = append(result, rb)
	}
	return result
}

func (s *roleBindingStore) Add(obj interface{}) error {
	rb, ok := obj.(*rbacv1.RoleBinding)
	if !ok {
		return nil
	}
	if !referencesDefaultServiceAccount(rb.Subjects) {
		return nil
	}
	key := rb.GetNamespace() + "/" + rb.GetName()
	s.mu.Lock()
	s.items[key] = rb
	s.mu.Unlock()
	return nil
}

func (s *roleBindingStore) Update(obj interface{}) error {
	rb, ok := obj.(*rbacv1.RoleBinding)
	if !ok {
		return nil
	}
	key := rb.GetNamespace() + "/" + rb.GetName()
	s.mu.Lock()
	if referencesDefaultServiceAccount(rb.Subjects) {
		s.items[key] = rb
	} else {
		delete(s.items, key)
	}
	s.mu.Unlock()
	return nil
}

func (s *roleBindingStore) Delete(obj interface{}) error {
	rb, ok := obj.(*rbacv1.RoleBinding)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return nil
		}
		rb, ok = tombstone.Obj.(*rbacv1.RoleBinding)
		if !ok {
			return nil
		}
	}
	key := rb.GetNamespace() + "/" + rb.GetName()
	s.mu.Lock()
	delete(s.items, key)
	s.mu.Unlock()
	return nil
}

func (s *roleBindingStore) Replace(list []interface{}, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]*rbacv1.RoleBinding)
	for _, obj := range list {
		rb, ok := obj.(*rbacv1.RoleBinding)
		if !ok {
			continue
		}
		if referencesDefaultServiceAccount(rb.Subjects) {
			key := rb.GetNamespace() + "/" + rb.GetName()
			s.items[key] = rb
		}
	}
	s.hasSynced = true
	return nil
}

// List is not implemented
func (s *roleBindingStore) List() []interface{} {
	panic("not implemented")
}

// ListKeys is not implemented
func (s *roleBindingStore) ListKeys() []string {
	panic("not implemented")
}

// Get is not implemented
func (s *roleBindingStore) Get(_ interface{}) (interface{}, bool, error) {
	panic("not implemented")
}

// GetByKey is not implemented
func (s *roleBindingStore) GetByKey(_ string) (interface{}, bool, error) {
	panic("not implemented")
}

// Resync is not implemented
func (s *roleBindingStore) Resync() error {
	panic("not implemented")
}

type clusterRoleBindingStore struct {
	mu        sync.RWMutex
	items     map[string]*rbacv1.ClusterRoleBinding
	hasSynced bool
}

func newClusterRoleBindingStore() *clusterRoleBindingStore {
	return &clusterRoleBindingStore{
		items: make(map[string]*rbacv1.ClusterRoleBinding),
	}
}

func (s *clusterRoleBindingStore) HasSynced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasSynced
}

func (s *clusterRoleBindingStore) GetFiltered() []*rbacv1.ClusterRoleBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*rbacv1.ClusterRoleBinding, 0, len(s.items))
	for _, crb := range s.items {
		result = append(result, crb)
	}
	return result
}

func (s *clusterRoleBindingStore) Add(obj interface{}) error {
	crb, ok := obj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return nil
	}
	if !referencesDefaultServiceAccount(crb.Subjects) {
		return nil
	}
	s.mu.Lock()
	s.items[crb.GetName()] = crb
	s.mu.Unlock()
	return nil
}

func (s *clusterRoleBindingStore) Update(obj interface{}) error {
	crb, ok := obj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return nil
	}
	s.mu.Lock()
	if referencesDefaultServiceAccount(crb.Subjects) {
		s.items[crb.GetName()] = crb
	} else {
		delete(s.items, crb.GetName())
	}
	s.mu.Unlock()
	return nil
}

func (s *clusterRoleBindingStore) Delete(obj interface{}) error {
	crb, ok := obj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return nil
		}
		crb, ok = tombstone.Obj.(*rbacv1.ClusterRoleBinding)
		if !ok {
			return nil
		}
	}
	s.mu.Lock()
	delete(s.items, crb.GetName())
	s.mu.Unlock()
	return nil
}

func (s *clusterRoleBindingStore) Replace(list []interface{}, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]*rbacv1.ClusterRoleBinding)
	for _, obj := range list {
		crb, ok := obj.(*rbacv1.ClusterRoleBinding)
		if !ok {
			continue
		}
		if referencesDefaultServiceAccount(crb.Subjects) {
			s.items[crb.GetName()] = crb
		}
	}
	s.hasSynced = true
	return nil
}

// List is not implemented
func (s *clusterRoleBindingStore) List() []interface{} {
	panic("not implemented")
}

// ListKeys is not implemented
func (s *clusterRoleBindingStore) ListKeys() []string {
	panic("not implemented")
}

// Get is not implemented
func (s *clusterRoleBindingStore) Get(_ interface{}) (interface{}, bool, error) {
	panic("not implemented")
}

// GetByKey is not implemented
func (s *clusterRoleBindingStore) GetByKey(_ string) (interface{}, bool, error) {
	panic("not implemented")
}

// Resync is not implemented
func (s *clusterRoleBindingStore) Resync() error {
	panic("not implemented")
}
