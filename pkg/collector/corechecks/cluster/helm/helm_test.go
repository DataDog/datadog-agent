// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

var testTimeout = 30 * time.Second
var testTicker = 100 * time.Millisecond

func TestRun(t *testing.T) {
	releases := []release{
		{
			Name: "my_datadog",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "datadog",
					Version:    "2.30.5",
					AppVersion: "7",
				},
			},
			Version:   1,
			Namespace: "default",
		},
		{
			Name: "my_app",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "some_app",
					Version:    "1.1.0",
					AppVersion: "1",
				},
			},
			Version:   2,
			Namespace: "app",
		},
		{ // Release with a nil chart reference
			Name: "release_without_chart",
			Info: &info{
				Status: "deployed",
			},
			Chart:     nil,
			Version:   1,
			Namespace: "default",
		},
		{ // Release with a nil info reference
			Name: "release_without_info",
			Info: nil,
			Chart: &chart{
				Metadata: &metadata{
					Name:       "example_app",
					Version:    "2.0.0",
					AppVersion: "1",
				},
			},
			Version:   1,
			Namespace: "default",
		},
		{
			// Release with escapable version
			Name: "foo",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "foo",
					Version:    "2+30+5",
					AppVersion: "7",
				},
			},
			Version:   1,
			Namespace: "default",
		},
		{
			// Release with helm values set
			Name: "with_helm_values",
			Info: &info{
				Status: "deployed",
			},
			Config: map[string]interface{}{
				"option": "2",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "with_helm_values",
					Version:    "1.0.0",
					AppVersion: "1",
				},
				Values: map[string]interface{}{
					"option": 1,
				},
			},
			Version:   1,
			Namespace: "default",
		},
	}

	// Same order as "releases" array
	var secretsForReleases []*v1.Secret
	for _, rel := range releases {
		secret, err := secretForRelease(&rel, time.Now())
		require.NoError(t, err)
		secretsForReleases = append(secretsForReleases, secret)
	}

	// Same order as "releases" array
	var configmapsForReleases []*v1.ConfigMap
	for _, rel := range releases {
		configMap, err := configMapForRelease(&rel)
		require.NoError(t, err)
		configmapsForReleases = append(configmapsForReleases, configMap)
	}

	// Same order as "releases" array
	expectedTagsForReleases := [][]string{
		{
			"helm_release:my_datadog",
			"helm_chart_name:datadog",
			"kube_namespace:default",
			"helm_namespace:default",
			"helm_revision:1",
			"helm_status:deployed",
			"helm_chart_version:2.30.5",
			"helm_app_version:7",
			"helm_chart:datadog-2.30.5",
		},
		{
			"helm_release:my_app",
			"helm_chart_name:some_app",
			"kube_namespace:app",
			"helm_namespace:app",
			"helm_revision:2",
			"helm_status:deployed",
			"helm_chart_version:1.1.0",
			"helm_app_version:1",
			"helm_chart:some_app-1.1.0",
		},
		{
			"helm_release:release_without_chart",
			"kube_namespace:default",
			"helm_namespace:default",
			"helm_revision:1",
			"helm_status:deployed",
		},
		{
			"helm_release:release_without_info",
			"helm_chart_name:example_app",
			"kube_namespace:default",
			"helm_namespace:default",
			"helm_revision:1",
			"helm_chart_version:2.0.0",
			"helm_app_version:1",
			"helm_chart:example_app-2.0.0",
		},
		{
			"helm_release:foo",
			"helm_chart_name:foo",
			"kube_namespace:default",
			"helm_namespace:default",
			"helm_revision:1",
			"helm_status:deployed",
			"helm_chart_version:2+30+5",
			"helm_app_version:7",
			"helm_chart:foo-2_30_5",
		},
		{
			"helm_release:with_helm_values",
			"helm_chart_name:with_helm_values",
			"kube_namespace:default",
			"helm_namespace:default",
			"helm_revision:1",
			"helm_status:deployed",
			"helm_chart_version:1.0.0",
			"helm_app_version:1",
			"helm_chart:with_helm_values-1.0.0",
			"option_value:2", // Because "HelmValuesAsTags" is set with "option" => "option_value" in the test check
		},
	}

	tests := []struct {
		name         string
		secrets      []*v1.Secret
		configmaps   []*v1.ConfigMap
		expectedTags [][]string
	}{
		{
			name:         "using secrets",
			secrets:      secretsForReleases,
			expectedTags: expectedTagsForReleases,
		},
		{
			name:         "using configmaps",
			configmaps:   configmapsForReleases,
			expectedTags: expectedTagsForReleases,
		},
		{
			name:         "using secrets and configmaps",
			secrets:      []*v1.Secret{secretsForReleases[0]},
			configmaps:   configmapsForReleases[1:],
			expectedTags: expectedTagsForReleases,
		},
		{
			name: "no secrets or configmaps owned by Helm",
			secrets: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "some_secret",
						Labels: map[string]string{"owner": "not-helm"},
					},
				},
			},
			configmaps: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "some_configmap",
						Labels: map[string]string{"owner": "not-helm"},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var kubeObjects []runtime.Object
			for _, secret := range test.secrets {
				kubeObjects = append(kubeObjects, secret)
			}
			for _, configMap := range test.configmaps {
				kubeObjects = append(kubeObjects, configMap)
			}

			check := newCheck().(*HelmCheck)
			check.runLeaderElection = false

			check.instance.HelmValuesAsTags = map[string]string{
				"option": "option_value",
			}

			check.informerFactory = informers.NewSharedInformerFactory(
				fake.NewSimpleClientset(kubeObjects...),
				time.Minute,
			)

			mockedSender := mocksender.NewMockSender(t, CheckName)
			mockedSender.SetupAcceptAll()

			// The informers are set up in the first run, but the first metrics
			// are not necessarily emitted in the first run. It depends on
			// whether the check had time to process the events.

			err := check.CommonConfigure(mockedSender.GetSenderManager(), nil, nil, "", "")
			require.NoError(t, err)

			err = check.Run()
			require.NoError(t, err)

			require.Eventually(t, func() bool { // Wait until the events are processed
				return len(check.store.getAll(k8sSecrets))+len(check.store.getAll(k8sConfigmaps)) == len(test.expectedTags)
			}, testTimeout, testTicker)

			err = check.Run()
			require.NoError(t, err)

			for _, tags := range test.expectedTags {
				mockedSender.AssertMetric(t, "Gauge", "helm.release", 1, "", tags)
			}
		})
	}
}

func TestRun_withCollectEvents(t *testing.T) {
	check := newCheck().(*HelmCheck)
	check.runLeaderElection = false
	check.instance.CollectEvents = true
	check.startTS = time.Now()

	rel := release{
		Name: "my_datadog",
		Info: &info{
			Status: "deployed",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	eventsAllowedDelta := 10 * time.Second

	// The creation timestamp of the secret needs to be after the check start. Otherwise, the event will not be emitted.
	secret, err := secretForRelease(&rel, check.startTS.Add(10))
	require.NoError(t, err)

	k8sClient := fake.NewSimpleClientset()
	check.informerFactory = informers.NewSharedInformerFactory(k8sClient, time.Minute)

	mockedSender := mocksender.NewMockSender(t, CheckName)
	mockedSender.SetupAcceptAll()

	// First run to set up the informers.
	err = check.CommonConfigure(mockedSender.GetSenderManager(), nil, nil, "", "")
	require.NoError(t, err)

	err = check.Run()
	require.NoError(t, err)

	// Create a new release and check that it creates the appropriate event.
	_, err = k8sClient.CoreV1().Secrets("default").Create(context.TODO(), secret, metav1.CreateOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { // Wait until the create event has been processed
		return len(check.store.getAll(k8sSecrets)) == 1
	}, testTimeout, testTicker)
	err = check.Run()
	require.NoError(t, err)
	expectedTags := check.allTags(&rel, k8sSecrets, true)
	mockedSender.AssertEventWithCompareFunc(
		t,
		eventForRelease(&rel, "New Helm release \"my_datadog\" has been deployed in \"default\" namespace. Its status is \"deployed\".", expectedTags),
		eventsAllowedDelta,
		eventCompareFunc,
	)
	mockedSender.ResetCalls()

	// Upgrade the release and check that it creates the appropriate event.
	upgradedRel := rel
	upgradedRel.Version = 2
	secretUpgradedRel, err := secretForRelease(&upgradedRel, time.Now().Add(10))
	require.NoError(t, err)
	_, err = k8sClient.CoreV1().Secrets("default").Create(context.TODO(), secretUpgradedRel, metav1.CreateOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { // Wait until the create event has been processed (revision 2 created)
		return check.store.get("default/my_datadog", 2, k8sSecrets) != nil
	}, testTimeout, testTicker)
	err = check.Run()
	require.NoError(t, err)
	expectedTags = check.allTags(&upgradedRel, k8sSecrets, true)
	mockedSender.AssertEventWithCompareFunc(
		t,
		eventForRelease(&upgradedRel, "Helm release \"my_datadog\" in \"default\" namespace upgraded to revision 2. Its status is \"deployed\".", expectedTags),
		eventsAllowedDelta,
		eventCompareFunc,
	)
	mockedSender.ResetCalls()

	// Delete the release (all revisions) and check that it creates the
	// appropriate event.
	err = k8sClient.CoreV1().Secrets("default").Delete(context.TODO(), rel.Name+".1", metav1.DeleteOptions{})
	require.NoError(t, err)
	err = k8sClient.CoreV1().Secrets("default").Delete(context.TODO(), rel.Name+".2", metav1.DeleteOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { // Wait until the delete events have been processed (store should be empty)
		return len(check.store.getAll(k8sSecrets)) == 0
	}, testTimeout, testTicker)
	err = check.Run()
	require.NoError(t, err)
	expectedTags = check.allTags(&rel, k8sSecrets, false)
	for i, tag := range expectedTags {
		if strings.HasPrefix(tag, "helm_status:") {
			expectedTags[i] = "helm_status:uninstalled"
			break
		}
	}
	mockedSender.AssertEventWithCompareFunc(
		t,
		eventForRelease(&rel, "Helm release \"my_datadog\" in \"default\" namespace has been deleted.", expectedTags),
		eventsAllowedDelta,
		eventCompareFunc,
	)
}

func TestRun_skipEventForExistingRelease(t *testing.T) {
	check := newCheck().(*HelmCheck)
	check.runLeaderElection = false
	check.instance.CollectEvents = true
	check.startTS = time.Now()

	rel := release{
		Name: "my_datadog",
		Info: &info{
			Status: "deployed",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	// The creation timestamp for the secret needs to be before the check start to simulate an existing release.
	secret, err := secretForRelease(&rel, check.startTS.Add(-10))
	require.NoError(t, err)

	k8sClient := fake.NewSimpleClientset()
	check.informerFactory = informers.NewSharedInformerFactory(k8sClient, time.Minute)

	mockedSender := mocksender.NewMockSender(t, CheckName)
	mockedSender.SetupAcceptAll()

	// Create a new release and check that we never send an event for it
	_, err = k8sClient.CoreV1().Secrets("default").Create(context.TODO(), secret, metav1.CreateOptions{})
	require.NoError(t, err)
	err = check.CommonConfigure(mockedSender.GetSenderManager(), nil, nil, "", "")
	require.NoError(t, err)
	err = check.Run()
	require.NoError(t, err)
	mockedSender.AssertNotCalled(t, "Event")
}

func TestRun_ServiceCheck(t *testing.T) {
	// Releases used for this test:
	// - "my_datadog" has 2 revisions without failures.
	// - "my_app" has 2 revisions but the latest one is not in "failed" state.
	// - "my_proxy" has 2 revisions and the latest one is in "failed" state.
	//
	// Only "my_proxy" should be marked as failed by the service check.
	releases := []*release{
		{
			Name: "my_datadog",
			Info: &info{
				Status: "superseded",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "datadog",
					Version:    "2.30.5",
					AppVersion: "7",
				},
			},
			Version:   1,
			Namespace: "default",
		},
		{
			Name: "my_datadog",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "datadog",
					Version:    "2.30.5",
					AppVersion: "7",
				},
			},
			Version:   2,
			Namespace: "default",
		},
		{
			Name: "my_app",
			Info: &info{
				Status: "failed", // Notice that it's failed, but it's not the latest release
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "some_app",
					Version:    "1.0.0",
					AppVersion: "1",
				},
			},
			Version:   1,
			Namespace: "default",
		},
		{
			Name: "my_app",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "some_app",
					Version:    "1.0.0",
					AppVersion: "1",
				},
			},
			Version:   2,
			Namespace: "default",
		},
		{
			Name: "my_proxy",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "nginx",
					Version:    "1.0.0",
					AppVersion: "1",
				},
			},
			Version:   10,
			Namespace: "default",
		},
		{
			Name: "my_proxy",
			Info: &info{
				Status: "failed", // Notice that this is the latest release, and it's failed
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "nginx",
					Version:    "1.0.0",
					AppVersion: "1",
				},
			},
			Version:   12,
			Namespace: "default",
		},
	}

	tests := []struct {
		name    string
		storage helmStorage
	}{
		{
			name:    "using secrets storage",
			storage: k8sSecrets,
		},
		{
			name:    "using configmaps storage",
			storage: k8sConfigmaps,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check := newCheck().(*HelmCheck)
			check.runLeaderElection = false

			for _, rel := range releases {
				check.store.add(rel, test.storage, commonTags(rel, test.storage), check.tagsForMetricsAndEvents(rel, true))
			}

			mockedSender := mocksender.NewMockSender(t, CheckName)
			mockedSender.SetupAcceptAll()

			k8sClient := fake.NewSimpleClientset()
			check.informerFactory = informers.NewSharedInformerFactory(k8sClient, time.Minute)
			err := check.CommonConfigure(mockedSender.GetSenderManager(), nil, nil, "", "")
			require.NoError(t, err)
			err = check.Run()
			require.NoError(t, err)

			// "my_datadog" release should report OK.
			mockedSender.AssertServiceCheck(
				t,
				"helm.release_state",
				servicecheck.ServiceCheckOK,
				"",
				[]string{
					"helm_release:my_datadog",
					"kube_namespace:default",
					"helm_namespace:default",
					fmt.Sprintf("helm_storage:%s", test.storage),
					"helm_chart_name:datadog",
				},
				"",
			)

			// "my_app" release should report OK.
			mockedSender.AssertServiceCheck(
				t,
				"helm.release_state",
				servicecheck.ServiceCheckOK,
				"",
				[]string{
					"helm_release:my_app",
					"kube_namespace:default",
					"helm_namespace:default",
					fmt.Sprintf("helm_storage:%s", test.storage),
					"helm_chart_name:some_app",
				},
				"",
			)

			// "my_proxy" release should report a failure.
			mockedSender.AssertServiceCheck(
				t,
				"helm.release_state",
				servicecheck.ServiceCheckCritical,
				"",
				[]string{
					"helm_release:my_proxy",
					"kube_namespace:default",
					"helm_namespace:default",
					fmt.Sprintf("helm_storage:%s", test.storage),
					"helm_chart_name:nginx",
				},
				"Release in \"failed\" state",
			)
		})
	}

}

// Test the actual informer stores as well as watch updates and deletions: a
// transform that only trims the check's release store leaves the large Kubernetes
// payloads alive in these caches.
func TestCompactInformerCache(t *testing.T) {
	for _, storage := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		t.Run(string(storage), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				check := newCheck().(*HelmCheck)
				check.instance.CollectEvents = true
				check.instance.HelmValuesAsTags = map[string]string{"option": "option_value", "default": "default_value"}
				check.startTS = time.Now().Add(-time.Hour)
				check.informersStopCh = make(chan struct{})
				defer check.Cancel()

				rel := release{
					Name: "app", Namespace: "default", Version: 1,
					Info:   &info{Status: "deployed"},
					Config: map[string]interface{}{"option": "override", "unused": strings.Repeat("x", 100000)},
					Chart: &chart{
						Metadata: &metadata{Name: "chart", Version: "1.0", AppVersion: "2.0"},
						Values:   map[string]interface{}{"option": "default", "default": "fallback", "unused": strings.Repeat("y", 100000)},
					},
				}
				makeObject := func(rel *release, resourceVersion string) runtime.Object {
					var obj runtime.Object
					var err error
					if storage == k8sSecrets {
						obj, err = secretForRelease(rel, time.Now())
					} else {
						obj, err = configMapForRelease(rel)
					}
					require.NoError(t, err)
					metadata := obj.(metav1.Object)
					metadata.SetNamespace("default")
					metadata.SetResourceVersion(resourceVersion)
					metadata.SetCreationTimestamp(metav1.Now())
					metadata.SetAnnotations(map[string]string{"unused": strings.Repeat("z", 100000)})
					return obj
				}
				first := makeObject(&rel, "1")
				latest := rel
				latest.Version = 2
				second := makeObject(&latest, "2")
				client := fake.NewSimpleClientset(first, second)
				check.informerFactory = informers.NewSharedInformerFactory(client, time.Minute)
				require.NoError(t, check.setupInformers())
				synctest.Wait()
				require.Len(t, check.store.getAll(storage), 2)

				informer := check.informerFactory.Core().V1().ConfigMaps().Informer()
				resource := v1.SchemeGroupVersion.WithResource("configmaps")
				if storage == k8sSecrets {
					informer = check.informerFactory.Core().V1().Secrets().Informer()
					resource = v1.SchemeGroupVersion.WithResource("secrets")
				}
				require.Len(t, informer.GetStore().List(), 2)
				for _, obj := range informer.GetStore().List() {
					cached, ok := obj.(*cachedRelease)
					require.True(t, ok, "informer retained Kubernetes object %T", obj)
					require.Nil(t, cached.Annotations)
					require.Nil(t, cached.Labels)
					require.Nil(t, cached.release.Config)
					require.Nil(t, cached.release.Chart.Values)
					require.Contains(t, cached.tagsForMetricsAndEvents, "option_value:override")
					require.Contains(t, cached.tagsForMetricsAndEvents, "default_value:fallback")
					stored := check.store.get(cached.release.namespacedName(), cached.release.revision(), storage)
					require.Same(t, cached.release, stored.release)
					transformed, err := check.transformRelease(cached)
					require.NoError(t, err)
					require.Same(t, cached, transformed, "transform must be idempotent")
				}

				latest.Info = &info{Status: "failed"}
				require.NoError(t, client.Tracker().Update(resource, makeObject(&latest, "3"), "default"))
				synctest.Wait()
				require.Equal(t, "failed", check.store.get("default/app", 2, storage).release.Info.Status)
				check.eventsManager.eventsMutex.Lock()
				events := append([]event.Event(nil), check.eventsManager.events...)
				check.eventsManager.eventsMutex.Unlock()
				require.Len(t, events, 3)
				require.Contains(t, events[2].Text, `changed its status from "deployed" to "failed"`)
				require.Contains(t, events[2].Tags, "option_value:override")

				// Deleting the latest revision must restore the previous revision.
				require.NoError(t, client.Tracker().Delete(resource, "default", "app.2"))
				synctest.Wait()
				require.Len(t, check.store.getAll(storage), 1)
				require.Equal(t, 1, check.store.getLatestRevisions(storage)[0].release.Version)
				require.NoError(t, client.Tracker().Delete(resource, "default", "app.1"))
				synctest.Wait()
				require.Empty(t, check.store.getAll(storage))
				require.Empty(t, informer.GetStore().List())
				check.eventsManager.eventsMutex.Lock()
				events = append([]event.Event(nil), check.eventsManager.events...)
				check.eventsManager.eventsMutex.Unlock()
				require.Len(t, events, 4)
				require.Contains(t, events[3].Tags, "helm_status:uninstalled")
				require.Contains(t, events[3].Tags, "option_value:override")
				require.Contains(t, events[3].Tags, "default_value:fallback")
				require.NotContains(t, events[3].Tags, "helm_revision:1")
			})
		})
	}
}

func TestCachedReleaseTombstone(t *testing.T) {
	for _, storage := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		t.Run(string(storage), func(t *testing.T) {
			check := newCheck().(*HelmCheck)
			check.instance.CollectEvents = true
			check.instance.HelmValuesAsTags = map[string]string{"option": "option_value"}
			rel := &release{
				Name: "app", Namespace: "default", Version: 1,
				Info:   &info{Status: "deployed"},
				Config: map[string]interface{}{"option": "custom"},
			}
			var obj interface{}
			var err error
			if storage == k8sSecrets {
				obj, err = secretForRelease(rel, time.Now())
			} else {
				obj, err = configMapForRelease(rel)
			}
			require.NoError(t, err)
			obj, err = check.transformRelease(obj)
			require.NoError(t, err)
			check.addRelease(obj)
			check.deleteRelease(cache.DeletedFinalStateUnknown{Key: "default/app.1", Obj: obj})
			require.Empty(t, check.store.getAll(storage))
			deleted := check.eventsManager.events[len(check.eventsManager.events)-1]
			require.Contains(t, deleted.Tags, "option_value:custom")
			require.Contains(t, deleted.Tags, "helm_status:uninstalled")
			require.NotContains(t, deleted.Tags, "helm_revision:1")
			require.Contains(t, obj.(*cachedRelease).tagsForMetricsAndEvents, "helm_status:deployed", "deletion must not mutate cached tags")
		})
	}
}

func TestTransformInvalidRelease(t *testing.T) {
	for _, obj := range []interface{}{
		&v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "bad", Labels: map[string]string{"owner": "helm"}}, Data: map[string]string{"release": "invalid"}},
		&v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "bad", Labels: map[string]string{"owner": "helm"}}, Data: map[string][]byte{"release": []byte("invalid")}},
	} {
		check := newCheck().(*HelmCheck)
		transformed, err := check.transformRelease(obj)
		require.NoError(t, err, "invalid payloads must not prevent informer synchronization")
		cached := transformed.(*cachedRelease)
		require.Equal(t, "bad", cached.Name)
		require.Nil(t, cached.taggedRelease)
		check.addRelease(cached)
		check.deleteRelease(cached)
		require.Empty(t, check.store.getAll(cached.storage))
	}
}

// secretForRelease returns a Kubernetes secret that contains the info of the
// given Helm release.
func secretForRelease(rls *release, creationTS time.Time) (*v1.Secret, error) {
	encodedRel, err := encodeRelease(rls)
	if err != nil {
		return nil, err
	}

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// The name is not important for this test. We only need to make
			// sure that there are no collisions.
			Name:              fmt.Sprintf("%s.%d", rls.Name, rls.Version),
			Labels:            map[string]string{"owner": "helm"},
			CreationTimestamp: metav1.NewTime(creationTS),
		},
		Data: map[string][]byte{"release": []byte(encodedRel)},
	}, nil
}

// configMapForRelease returns a configmap that contains the info of the given
// Helm release.
func configMapForRelease(rls *release) (*v1.ConfigMap, error) {
	encodedRel, err := encodeRelease(rls)
	if err != nil {
		return nil, err
	}

	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			// The name is not important for this test. We only need to make
			// sure that there are no collisions.
			Name:   fmt.Sprintf("%s.%d", rls.Name, rls.Version),
			Labels: map[string]string{"owner": "helm"},
		},
		Data: map[string]string{"release": encodedRel},
	}, nil
}

// Note: the encodeRelease function has been copied from the Helm library.
// It's private, so we can't reuse it.
// Ref: https://github.com/helm/helm/blob/v3.8.0/pkg/storage/driver/util.go#L35

// encodeRelease encodes a release returning a base64 encoded
// gzipped string representation, or error.
func encodeRelease(rls *release) (string, error) {
	b, err := json.Marshal(rls)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err = w.Write(b); err != nil {
		return "", err
	}
	w.Close()

	return b64.EncodeToString(buf.Bytes()), nil
}

// This function is used to compare events, because by default, the mocksender
// package doesn't check the text of the event but for these tests we are
// interested on it
func eventCompareFunc(expected, actual event.Event) bool {
	return expected.Text == actual.Text
}
