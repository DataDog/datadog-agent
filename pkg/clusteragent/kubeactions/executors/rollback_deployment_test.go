// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestValidateTargetDeployment(t *testing.T) {
	pausedDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "paused-deploy",
			Namespace: "default",
			UID:       k8stypes.UID("uid-paused"),
		},
		Spec: appsv1.DeploymentSpec{
			Paused: true,
		},
	}

	activeDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "active-deploy",
			Namespace: "default",
			UID:       k8stypes.UID("uid-active"),
		},
		Spec: appsv1.DeploymentSpec{
			Paused: false,
		},
	}

	tests := []struct {
		testName  string
		name      string
		namespace string
		uid       string
		client    kubernetes.Interface
		wantErr   bool
	}{
		{
			testName:  "deployment not found",
			name:      "nonexistent",
			namespace: "default",
			uid:       "some-uid",
			client:    fake.NewSimpleClientset(),
			wantErr:   true,
		},
		{
			testName:  "UID mismatch",
			name:      "active-deploy",
			namespace: "default",
			uid:       "wrong-uid",
			client:    fake.NewSimpleClientset(activeDeployment),
			wantErr:   true,
		},
		{
			testName:  "deployment is paused",
			name:      "paused-deploy",
			namespace: "default",
			uid:       "uid-paused",
			client:    fake.NewSimpleClientset(pausedDeployment),
			wantErr:   true,
		},
		{
			testName:  "valid deployment",
			name:      "active-deploy",
			namespace: "default",
			uid:       "uid-active",
			client:    fake.NewSimpleClientset(activeDeployment),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			_, err := validateTargetDeployment(context.Background(), tt.client, tt.name, tt.namespace, tt.uid)
			if (err != nil) != tt.wantErr {
				t.Errorf("unexpected error %s", err)
			}
		})
	}
}

func TestGetPatchAnnotations(t *testing.T) {
	tests := []struct {
		name       string
		deploy     *appsv1.Deployment
		replicaSet *appsv1.ReplicaSet
		expected   map[string]string
	}{
		{
			name: "skipped annotations preserved from deployment",
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: buildAnnotationsToSkip("deploy"),
				},
			},
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: buildAnnotationsToSkip("replicaset"),
				},
			},
			expected: buildAnnotationsToSkip("deploy"),
		},
		{
			name: "non-skipped annotations taken from replicaset",
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: buildAnnotationsToSkip("deploy"),
				},
			},
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: mergeMaps(buildAnnotationsToSkip("replicaset"), map[string]string{
						"app":  "nginx",
						"team": "platform",
					}),
				},
			},
			expected: mergeMaps(buildAnnotationsToSkip("deploy"), map[string]string{
				"app":  "nginx",
				"team": "platform",
			}),
		},
		{
			name: "mixed: skipped from deployment, non-skipped from replicaset, deployment custom annotations dropped",
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: mergeMaps(buildAnnotationsToSkip("deploy"), map[string]string{
						"deploy-only": "should-be-dropped",
					}),
				},
			},
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: mergeMaps(buildAnnotationsToSkip("replicaset"), map[string]string{
						"app": "nginx",
					}),
				},
			},
			expected: mergeMaps(buildAnnotationsToSkip("deploy"), map[string]string{
				"app": "nginx",
			}),
		},
		{
			name: "both have no annotations",
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			expected: map[string]string{},
		},
		{
			name: "deployment has no skipped annotations",
			deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"app": "nginx",
					},
				},
			},
			expected: map[string]string{
				"app": "nginx",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotations := getPatchAnnotations(tt.deploy, tt.replicaSet)
			assert.Equal(t, tt.expected, annotations)
		})
	}
}

func TestGetReplicaSetByRevision(t *testing.T) {
	deploy := makeDeployment("my-deploy", "default", "deploy-uid")

	tests := []struct {
		testName       string
		targetRevision int64
		objects        []appsv1.ReplicaSet
		wantRevision   int64
		wantRSName     string
		wantErr        bool
	}{
		{
			testName:       "negative revision returns error",
			targetRevision: -1,
			wantErr:        true,
		},
		{
			testName:       "specific revision found",
			targetRevision: 3,
			objects: []appsv1.ReplicaSet{
				*makeReplicaSet("rs-1", "default", "1", "deploy-uid", "my-deploy"),
				*makeReplicaSet("rs-2", "default", "2", "deploy-uid", "my-deploy"),
				*makeReplicaSet("rs-3", "default", "3", "deploy-uid", "my-deploy"),
			},
			wantRevision: 3,
			wantRSName:   "rs-3",
		},
		{
			testName:       "specific revision not found",
			targetRevision: 99,
			objects: []appsv1.ReplicaSet{
				*makeReplicaSet("rs-1", "default", "1", "deploy-uid", "my-deploy"),
				*makeReplicaSet("rs-2", "default", "2", "deploy-uid", "my-deploy"),
				*makeReplicaSet("rs-3", "default", "3", "deploy-uid", "my-deploy"),
			},
			wantErr: true,
		},
		{
			testName:       "default (0) returns previous revision",
			targetRevision: 0,
			objects: []appsv1.ReplicaSet{
				*makeReplicaSet("rs-1", "default", "1", "deploy-uid", "my-deploy"),
				*makeReplicaSet("rs-2", "default", "2", "deploy-uid", "my-deploy"),
				*makeReplicaSet("rs-3", "default", "3", "deploy-uid", "my-deploy"),
			},
			wantRevision: 2,
			wantRSName:   "rs-2",
		},
		{
			testName:       "default (0) with only one RS returns error",
			targetRevision: 0,
			objects: []appsv1.ReplicaSet{
				*makeReplicaSet("rs-1", "default", "1", "deploy-uid", "my-deploy"),
			},
			wantErr: true,
		},
		{
			testName:       "no ReplicaSets returns error",
			targetRevision: 0,
			wantErr:        true,
		},
		{
			testName:       "RS not owned by deployment is ignored",
			targetRevision: 3,
			objects: []appsv1.ReplicaSet{
				*makeReplicaSet("rs-other", "default", "3", "other-uid", "other-deploy"),
			},
			wantErr: true,
		},
		{
			testName:       "non-sequential revisions default returns second highest",
			targetRevision: 0,
			objects: []appsv1.ReplicaSet{
				*makeReplicaSet("rs-1", "default", "1", "deploy-uid", "my-deploy"),
				*makeReplicaSet("rs-5", "default", "5", "deploy-uid", "my-deploy"),
				*makeReplicaSet("rs-10", "default", "10", "deploy-uid", "my-deploy"),
			},
			wantRevision: 5,
			wantRSName:   "rs-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			clientObjects := make([]k8sruntime.Object, 0, len(tt.objects))
			for i := range tt.objects {
				clientObjects = append(clientObjects, &tt.objects[i])
			}
			client := fake.NewSimpleClientset(clientObjects...)

			revision, rs, err := getReplicaSetByRevision(context.Background(), client, deploy, tt.targetRevision)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantRevision, revision)
			assert.Equal(t, tt.wantRSName, rs.Name)
		})
	}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		testName       string
		deployment     *appsv1.Deployment
		replicaSets    []appsv1.ReplicaSet
		targetRevision *int64
		resourceID     string
		wantStatus     string
		wantContains   string
		wantPatched    bool
		wantRevision   string // revision annotation of the RS whose template should appear in the patch
	}{
		{
			testName:   "rollback to previous revision",
			deployment: makeDeploymentWithTemplate("my-deploy", "default", "deploy-uid", "nginx:v3"),
			replicaSets: []appsv1.ReplicaSet{
				*makeReplicaSetWithTemplate("rs-1", "default", "1", "deploy-uid", "my-deploy", "nginx:v1"),
				*makeReplicaSetWithTemplate("rs-2", "default", "2", "deploy-uid", "my-deploy", "nginx:v2"),
				*makeReplicaSetWithTemplate("rs-3", "default", "3", "deploy-uid", "my-deploy", "nginx:v3"),
			},
			targetRevision: nil,
			resourceID:     "deploy-uid",
			wantStatus:     StatusSuccess,
			wantContains:   "successfully restored revision 2",
			wantPatched:    true,
			wantRevision:   "2",
		},
		{
			testName:   "rollback to specific revision",
			deployment: makeDeploymentWithTemplate("my-deploy", "default", "deploy-uid", "nginx:v3"),
			replicaSets: []appsv1.ReplicaSet{
				*makeReplicaSetWithTemplate("rs-1", "default", "1", "deploy-uid", "my-deploy", "nginx:v1"),
				*makeReplicaSetWithTemplate("rs-2", "default", "2", "deploy-uid", "my-deploy", "nginx:v2"),
				*makeReplicaSetWithTemplate("rs-3", "default", "3", "deploy-uid", "my-deploy", "nginx:v3"),
			},
			targetRevision: int64Ptr(1),
			resourceID:     "deploy-uid",
			wantStatus:     StatusSuccess,
			wantContains:   "successfully restored revision 1",
			wantPatched:    true,
			wantRevision:   "1",
		},
		{
			testName:   "template already matches previous revision",
			deployment: makeDeploymentWithTemplate("my-deploy", "default", "deploy-uid", "nginx:v2"),
			replicaSets: []appsv1.ReplicaSet{
				*makeReplicaSetWithTemplate("rs-1", "default", "1", "deploy-uid", "my-deploy", "nginx:v1"),
				*makeReplicaSetWithTemplate("rs-2", "default", "2", "deploy-uid", "my-deploy", "nginx:v2"),
				*makeReplicaSetWithTemplate("rs-3", "default", "3", "deploy-uid", "my-deploy", "nginx:v2"),
			},
			targetRevision: nil,
			resourceID:     "deploy-uid",
			wantStatus:     StatusSuccess,
			wantContains:   "already matches",
			wantPatched:    false,
		},
		{
			testName:       "deployment UID mismatch",
			deployment:     makeDeploymentWithTemplate("my-deploy", "default", "deploy-uid", "nginx:v1"),
			replicaSets:    nil,
			targetRevision: nil,
			resourceID:     "wrong-uid",
			wantStatus:     StatusFailed,
			wantPatched:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			var objects []k8sruntime.Object
			if tt.deployment != nil {
				objects = append(objects, tt.deployment)
			}
			for i := range tt.replicaSets {
				objects = append(objects, &tt.replicaSets[i])
			}

			clientset := fake.NewSimpleClientset(objects...)
			executor := NewRollbackDeploymentExecutor(clientset)

			action := makeRollbackAction(tt.deployment.GetNamespace(), tt.deployment.GetName(), tt.resourceID, tt.targetRevision)
			result := executor.Execute(context.Background(), action)

			assert.Equal(t, tt.wantStatus, result.Status)
			if tt.wantContains != "" {
				assert.Contains(t, result.Message, tt.wantContains)
			}

			if tt.wantPatched {
				patchAction, foundPatchErr := findPatchAction(clientset, tt.deployment)
				require.NoError(t, foundPatchErr)
				require.NotNil(t, patchAction, "expected a patch action on the deployment")
				assert.Equal(t, k8stypes.JSONPatchType, patchAction.GetPatchType())

				var ops []map[string]interface{}
				require.NoError(t, json.Unmarshal(patchAction.GetPatch(), &ops))
				require.Len(t, ops, 2)

				// Verify the template replacement matches the expected RS's spec
				assert.Equal(t, "replace", ops[0]["op"])
				assert.Equal(t, "/spec/template", ops[0]["path"])
				templateBytes, err := json.Marshal(ops[0]["value"])
				require.NoError(t, err)

				var template corev1.PodTemplateSpec
				require.NoError(t, json.Unmarshal(templateBytes, &template))
				expectedTemplate := findRSTemplate(t, tt.replicaSets, tt.wantRevision)
				assert.Equal(t, expectedTemplate, template)

				// Verify the annotations replacement
				assert.Equal(t, "replace", ops[1]["op"])
				assert.Equal(t, "/metadata/annotations", ops[1]["path"])
			}
		})
	}
}

// Helper functions

func boolPtr(b bool) *bool { return &b }

func int64Ptr(i int64) *int64 { return &i }

func mergeMaps(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}

func buildAnnotationsToSkip(val string) map[string]string {
	annotations := make(map[string]string)
	for annotation := range annotationsToSkip {
		annotations[annotation] = val
	}

	return annotations
}

func makeDeployment(name, namespace string, uid k8stypes.UID) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
		},
	}
}

func makeReplicaSet(name, namespace string, revision string, ownerUID k8stypes.UID, ownerName string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": ownerName},
			Annotations: map[string]string{
				revisionAnnotation: revision,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       ownerName,
					UID:        ownerUID,
					Controller: boolPtr(true),
				},
			},
		},
	}
}

func makeDeploymentWithTemplate(name, namespace string, uid k8stypes.UID, image string) *appsv1.Deployment {
	d := makeDeployment(name, namespace, uid)
	d.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": name},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: image},
			},
		},
	}
	return d
}

func makeReplicaSetWithTemplate(name, namespace, revision string, ownerUID k8stypes.UID, ownerName, image string) *appsv1.ReplicaSet {
	rs := makeReplicaSet(name, namespace, revision, ownerUID, ownerName)
	rs.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": ownerName},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: image},
			},
		},
	}
	return rs
}

func makeRollbackAction(namespace, name, resourceID string, targetRevision *int64) *kubeactions.KubeAction {
	return &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "Deployment",
			Namespace:  namespace,
			Name:       name,
			ResourceId: resourceID,
		},
		Action: &kubeactions.KubeAction_RollbackDeployment{
			RollbackDeployment: &kubeactions.RollbackDeploymentParams{
				TargetRevision: targetRevision,
			},
		},
	}
}

func findRSTemplate(t *testing.T, replicaSets []appsv1.ReplicaSet, revision string) corev1.PodTemplateSpec {
	t.Helper()
	for _, rs := range replicaSets {
		if rs.Annotations[revisionAnnotation] == revision {
			return rs.Spec.Template
		}
	}
	t.Fatalf("no ReplicaSet found with revision %s", revision)
	return corev1.PodTemplateSpec{}
}

func findPatchAction(clientset *fake.Clientset, deployment *appsv1.Deployment) (k8stesting.PatchAction, error) {
	if deployment == nil {
		return nil, fmt.Errorf("deployment is nil")
	}
	for _, action := range clientset.Actions() {
		if pa, ok := action.(k8stesting.PatchAction); ok && pa.GetResource().Resource == "deployments" && pa.GetName() == deployment.Name && pa.GetNamespace() == deployment.Namespace {
			return pa, nil
		}
	}
	return nil, fmt.Errorf("no patch was found")
}
