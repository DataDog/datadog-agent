// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var defaultSASubject = rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: "default", Namespace: "ns1"}
var otherSASubject = rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: "other", Namespace: "ns1"}

func TestRoleBindingStoreAdd(t *testing.T) {
	tests := []struct {
		name         string
		roleBinding  *rbacv1.RoleBinding
		expectStored bool
	}{
		{
			name: "references default SA",
			roleBinding: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rb1", Namespace: "ns1"},
				Subjects:   []rbacv1.Subject{defaultSASubject},
			},
			expectStored: true,
		},
		{
			name: "does not reference default SA",
			roleBinding: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rb2", Namespace: "ns1"},
				Subjects:   []rbacv1.Subject{otherSASubject},
			},
			expectStored: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newRoleBindingStore()

			err := store.Add(test.roleBinding)
			require.NoError(t, err)

			if test.expectStored {
				assert.ElementsMatch(t, store.GetFiltered(), []*rbacv1.RoleBinding{test.roleBinding})
			} else {
				assert.Empty(t, store.GetFiltered())
			}
		})
	}
}

func TestRoleBindingStoreUpdate(t *testing.T) {
	tests := []struct {
		name         string
		initial      *rbacv1.RoleBinding
		updated      *rbacv1.RoleBinding
		expectStored bool
	}{
		{
			name: "add default SA reference",
			initial: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rb1", Namespace: "ns1"},
				Subjects:   []rbacv1.Subject{otherSASubject},
			},
			updated: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rb1", Namespace: "ns1"},
				Subjects:   []rbacv1.Subject{defaultSASubject},
			},
			expectStored: true,
		},
		{
			name: "remove default SA reference",
			initial: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rb1", Namespace: "ns1"},
				Subjects:   []rbacv1.Subject{defaultSASubject},
			},
			updated: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rb1", Namespace: "ns1"},
				Subjects:   []rbacv1.Subject{otherSASubject},
			},
			expectStored: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newRoleBindingStore()
			err := store.Add(test.initial)
			require.NoError(t, err)

			err = store.Update(test.updated)
			require.NoError(t, err)

			if test.expectStored {
				assert.Len(t, store.GetFiltered(), 1)
			} else {
				assert.Empty(t, store.GetFiltered())
			}
		})
	}
}

func TestRoleBindingStoreDelete(t *testing.T) {
	store := newRoleBindingStore()
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb1", Namespace: "ns1"},
		Subjects:   []rbacv1.Subject{defaultSASubject},
	}
	err := store.Add(roleBinding)
	require.NoError(t, err)
	require.Len(t, store.GetFiltered(), 1)

	err = store.Delete(roleBinding)
	require.NoError(t, err)
	assert.Empty(t, store.GetFiltered())
}

func TestRoleBindingStoreReplace(t *testing.T) {
	store := newRoleBindingStore()

	initialRoleBindings := []interface{}{
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "rb1", Namespace: "ns1"},
			Subjects:   []rbacv1.Subject{defaultSASubject},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "rb2", Namespace: "ns2"},
			Subjects:   []rbacv1.Subject{otherSASubject},
		},
	}

	for _, roleBinding := range initialRoleBindings {
		err := store.Add(roleBinding)
		require.NoError(t, err)
	}

	// Only the first element refers to default SA
	require.ElementsMatch(t, store.GetFiltered(), []*rbacv1.RoleBinding{initialRoleBindings[0].(*rbacv1.RoleBinding)})

	newRoleBindings := []interface{}{
		&rbacv1.RoleBinding{ // No changes
			ObjectMeta: metav1.ObjectMeta{Name: "rb1", Namespace: "ns1"},
			Subjects:   []rbacv1.Subject{defaultSASubject},
		},
		&rbacv1.RoleBinding{ // Now refers to default SA
			ObjectMeta: metav1.ObjectMeta{Name: "rb2", Namespace: "ns2"},
			Subjects:   []rbacv1.Subject{defaultSASubject},
		},
	}

	err := store.Replace(newRoleBindings, "")
	require.NoError(t, err)
	assert.ElementsMatch(t, store.GetFiltered(), newRoleBindings)
}

func TestClusterRoleBindingStoreAdd(t *testing.T) {
	tests := []struct {
		name               string
		clusterRoleBinding *rbacv1.ClusterRoleBinding
		expectStored       bool
	}{
		{
			name: "references default SA",
			clusterRoleBinding: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "crb1"},
				Subjects:   []rbacv1.Subject{defaultSASubject},
			},
			expectStored: true,
		},
		{
			name: "does not reference default SA",
			clusterRoleBinding: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "crb2"},
				Subjects:   []rbacv1.Subject{otherSASubject},
			},
			expectStored: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newClusterRoleBindingStore()

			err := store.Add(test.clusterRoleBinding)
			require.NoError(t, err)

			if test.expectStored {
				assert.ElementsMatch(t, store.GetFiltered(), []*rbacv1.ClusterRoleBinding{test.clusterRoleBinding})
			} else {
				assert.Empty(t, store.GetFiltered())
			}
		})
	}
}

func TestClusterRoleBindingStoreUpdate(t *testing.T) {
	tests := []struct {
		name         string
		initial      *rbacv1.ClusterRoleBinding
		updated      *rbacv1.ClusterRoleBinding
		expectStored bool
	}{
		{
			name: "add default SA reference",
			initial: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "crb1"},
				Subjects:   []rbacv1.Subject{otherSASubject},
			},
			updated: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "crb1"},
				Subjects:   []rbacv1.Subject{defaultSASubject},
			},
			expectStored: true,
		},
		{
			name: "remove default SA reference",
			initial: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "crb1"},
				Subjects:   []rbacv1.Subject{defaultSASubject},
			},
			updated: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "crb1"},
				Subjects:   []rbacv1.Subject{otherSASubject},
			},
			expectStored: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newClusterRoleBindingStore()
			err := store.Add(test.initial)
			require.NoError(t, err)

			err = store.Update(test.updated)
			require.NoError(t, err)

			if test.expectStored {
				assert.Len(t, store.GetFiltered(), 1)
			} else {
				assert.Empty(t, store.GetFiltered())
			}
		})
	}
}

func TestClusterRoleBindingStoreDelete(t *testing.T) {
	store := newClusterRoleBindingStore()
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "crb1"},
		Subjects:   []rbacv1.Subject{defaultSASubject},
	}
	err := store.Add(clusterRoleBinding)
	require.NoError(t, err)
	require.Len(t, store.GetFiltered(), 1)

	err = store.Delete(clusterRoleBinding)
	assert.NoError(t, err)
	assert.Empty(t, store.GetFiltered())
}

func TestClusterRoleBindingStoreReplace(t *testing.T) {
	store := newClusterRoleBindingStore()

	initialClusterRoleBindings := []interface{}{
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "crb1", Namespace: "ns1"},
			Subjects:   []rbacv1.Subject{defaultSASubject},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "crb2", Namespace: "ns2"},
			Subjects:   []rbacv1.Subject{otherSASubject},
		},
	}

	for _, clusterRoleBinding := range initialClusterRoleBindings {
		err := store.Add(clusterRoleBinding)
		require.NoError(t, err)
	}

	// Only the first element refers to default SA
	require.ElementsMatch(t, store.GetFiltered(), []*rbacv1.ClusterRoleBinding{initialClusterRoleBindings[0].(*rbacv1.ClusterRoleBinding)})

	newClusterRoleBindings := []interface{}{
		&rbacv1.ClusterRoleBinding{ // No changes
			ObjectMeta: metav1.ObjectMeta{Name: "crb1", Namespace: "ns1"},
			Subjects:   []rbacv1.Subject{defaultSASubject},
		},
		&rbacv1.ClusterRoleBinding{ // Now refers to default SA
			ObjectMeta: metav1.ObjectMeta{Name: "crb2", Namespace: "ns2"},
			Subjects:   []rbacv1.Subject{defaultSASubject},
		},
	}

	err := store.Replace(newClusterRoleBindings, "")
	require.NoError(t, err)
	assert.ElementsMatch(t, store.GetFiltered(), newClusterRoleBindings)
}
