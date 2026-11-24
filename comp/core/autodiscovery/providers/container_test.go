// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package providers

import (
	"context"
	"sync"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	acTelemetry "github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	healthplatformnoop "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type containerHealthTestStore struct {
	workloadmeta.Component
	events chan workloadmeta.EventBundle
	once   sync.Once
}

func (s *containerHealthTestStore) Subscribe(string, workloadmeta.SubscriberPriority, *workloadmeta.Filter) chan workloadmeta.EventBundle {
	return s.events
}

func (s *containerHealthTestStore) Unsubscribe(chan workloadmeta.EventBundle) {
	s.once.Do(func() { close(s.events) })
}

type blockingContainerHealth struct {
	*healthplatformmock.Mock
	reportStarted  chan string
	resolveStarted chan string
	completed      chan string
	reportRelease  <-chan struct{}
	resolveRelease <-chan struct{}
}

func (h *blockingContainerHealth) ReportIssue(issue *healthplatformpayload.Issue) error {
	h.reportStarted <- issue.Id
	if h.reportRelease != nil {
		<-h.reportRelease
	}
	err := h.Mock.ReportIssue(issue)
	h.completed <- "report:" + issue.Id
	return err
}

func (h *blockingContainerHealth) ResolveIssue(id string) {
	h.resolveStarted <- id
	if h.resolveRelease != nil {
		<-h.resolveRelease
	}
	h.Mock.ResolveIssue(id)
	h.completed <- "resolve:" + id
}

func newContainerHealthTestStream(t *testing.T, reportRelease, resolveRelease <-chan struct{}) (*ContainerConfigProvider, *containerHealthTestStore, *blockingContainerHealth, <-chan integration.ConfigChanges) {
	t.Helper()
	store := &containerHealthTestStore{events: make(chan workloadmeta.EventBundle)}
	hp := &blockingContainerHealth{
		Mock: healthplatformmock.New(t), reportStarted: make(chan string, 10),
		resolveStarted: make(chan string, 10), completed: make(chan string, 10),
		reportRelease: reportRelease, resolveRelease: resolveRelease,
	}
	provider, err := NewContainerConfigProvider(nil, store, nil, nil, hp, nil)
	require.NoError(t, err)
	cp := provider.(*ContainerConfigProvider)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return cp, store, hp, cp.Stream(ctx)
}

func processContainerHealthTestEvent(t *testing.T, store *containerHealthTestStore, changes <-chan integration.ConfigChanges, eventType workloadmeta.EventType, entity workloadmeta.Entity) integration.ConfigChanges {
	t.Helper()
	ack := make(chan struct{})
	bundle := workloadmeta.EventBundle{Ch: ack, Events: []workloadmeta.Event{{Type: eventType, Entity: entity}}}
	select {
	case store.events <- bundle:
	case <-time.After(5 * time.Second):
		t.Fatal("provider stopped reading workloadmeta events")
	}
	var result integration.ConfigChanges
	select {
	case result = <-changes:
	case <-time.After(5 * time.Second):
		t.Fatal("provider waited for health reporting before delivering configs")
	}
	select {
	case <-ack:
	case <-time.After(5 * time.Second):
		t.Fatal("provider waited for health reporting before acknowledging events")
	}
	return result
}

func receiveContainerHealthCall(t *testing.T, calls <-chan string) string {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("health worker did not process the update")
		return ""
	}
}

func TestContainerHealthReportDoesNotBlockStream(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	cp, store, hp, changes := newContainerHealthTestStream(t, release, nil)
	t.Cleanup(unblock)
	bad := basicDockerContainer()
	bad.Labels["com.datadoghq.ad.check_names"] = "["
	issueID := adAnnotationIssueID(buildEntityName(bad))
	processContainerHealthTestEvent(t, store, changes, workloadmeta.EventTypeSet, bad)
	assert.Equal(t, issueID, receiveContainerHealthCall(t, hp.reportStarted))
	assert.NotEmpty(t, cp.GetConfigErrors())

	// Multiple updates coalesce while the first report is blocked, and the
	// fixed configuration still reaches AD and is acknowledged immediately.
	for range 20 {
		processContainerHealthTestEvent(t, store, changes, workloadmeta.EventTypeSet, bad)
	}
	got := processContainerHealthTestEvent(t, store, changes, workloadmeta.EventTypeSet, basicDockerContainer())
	assert.Len(t, got.Schedule, 2)
	assert.Empty(t, cp.GetConfigErrors())
	unblock()
	assert.Equal(t, "report:"+issueID, receiveContainerHealthCall(t, hp.completed))
	assert.Equal(t, "resolve:"+issueID, receiveContainerHealthCall(t, hp.completed))
	assert.Nil(t, hp.GetIssue(issueID), "the delayed report must not reopen the fixed issue")
}

func TestContainerHealthResolveDoesNotBlockStream(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	cp, store, hp, changes := newContainerHealthTestStream(t, nil, release)
	t.Cleanup(unblock)
	bad := basicDockerContainer()
	bad.Labels["com.datadoghq.ad.check_names"] = "["
	issueID := adAnnotationIssueID(buildEntityName(bad))
	processContainerHealthTestEvent(t, store, changes, workloadmeta.EventTypeSet, bad)
	assert.Equal(t, "report:"+issueID, receiveContainerHealthCall(t, hp.completed))
	processContainerHealthTestEvent(t, store, changes, workloadmeta.EventTypeSet, basicDockerContainer())
	assert.Equal(t, issueID, receiveContainerHealthCall(t, hp.resolveStarted))

	// A new error arriving during resolution must be reported after the clear.
	bad.Labels["com.datadoghq.ad.check_names"] = "not-json"
	processContainerHealthTestEvent(t, store, changes, workloadmeta.EventTypeSet, bad)
	assert.NotEmpty(t, cp.GetConfigErrors())
	unblock()
	assert.Equal(t, "resolve:"+issueID, receiveContainerHealthCall(t, hp.completed))
	assert.Equal(t, "report:"+issueID, receiveContainerHealthCall(t, hp.completed))
	issue := hp.GetIssue(issueID)
	require.NotNil(t, issue)
	assert.Equal(t, string(types.ContainerLabelSource), issue.Extra.Fields["error_source"].GetStringValue())

	processContainerHealthTestEvent(t, store, changes, workloadmeta.EventTypeUnset, bad)
	assert.Equal(t, "resolve:"+issueID, receiveContainerHealthCall(t, hp.completed))
	assert.Empty(t, cp.GetConfigErrors())
	assert.Nil(t, hp.GetIssue(issueID))
}

func TestContainerHealthResolvesRestoredIssue(t *testing.T) {
	_, store, hp, changes := newContainerHealthTestStream(t, nil, nil)
	container := basicDockerContainer()
	issueID := adAnnotationIssueID(buildEntityName(container))
	require.NoError(t, hp.Mock.ReportIssue(&healthplatformpayload.Issue{Id: issueID, IssueName: "restored"}))
	processContainerHealthTestEvent(t, store, changes, workloadmeta.EventTypeSet, container)
	assert.Equal(t, "resolve:"+issueID, receiveContainerHealthCall(t, hp.completed))
	assert.Nil(t, hp.GetIssue(issueID), "a valid entity must clear issues restored before this provider started")
}

func TestProcessEvents(t *testing.T) {
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	telemetry := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryStore := acTelemetry.NewStore(telemetry)

	provider, err := NewContainerConfigProvider(nil, store, nil, nil, healthplatformnoop.NewNoopComponent(), telemetryStore)
	require.NoError(t, err)
	cp := provider.(*ContainerConfigProvider)

	tests := []struct {
		name    string
		events  []workloadmeta.Event
		changes integration.ConfigChanges
	}{
		{
			name: "create config",
			events: []workloadmeta.Event{
				{
					Type:   workloadmeta.EventTypeSet,
					Entity: basicDockerContainer(),
				},
			},
			changes: integration.ConfigChanges{
				Schedule: basicDockerConfigs(),
			},
		},
		{
			name: "replace config",
			events: []workloadmeta.Event{
				{
					Type:   workloadmeta.EventTypeSet,
					Entity: basicDockerContainerSingleCheck(),
				},
			},
			changes: integration.ConfigChanges{
				Unschedule: []integration.Config{
					basicDockerConfigs()[1],
				},
			},
		},
		{
			name: "delete config",
			events: []workloadmeta.Event{
				{
					Type:   workloadmeta.EventTypeUnset,
					Entity: basicDockerContainer(),
				},
			},
			changes: integration.ConfigChanges{
				Unschedule: []integration.Config{
					basicDockerConfigs()[0],
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := cp.processEvents(workloadmeta.EventBundle{
				Events: tt.events,
			})

			assert.Equal(t, tt.changes.Schedule, changes.Schedule)
			assert.Equal(t, tt.changes.Unschedule, changes.Unschedule)
		})
	}
}

func TestGenerateConfig(t *testing.T) {
	tests := []struct {
		name                string
		entity              workloadmeta.Entity
		expectedConfigs     []integration.Config
		expectedErr         types.ErrorMsgSet
		containerCollectAll bool
	}{
		{
			name:            "container check",
			entity:          basicDockerContainer(),
			expectedConfigs: basicDockerConfigs(),
		},
		{
			name: "No annotations",
			entity: &workloadmeta.KubernetesPod{
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "testName",
						ID:   "testID",
					},
				},
			},
		},
		{
			name: "v2 annotations",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.checks": `{
							"http_check": {
								"instances": [
									{
										"name": "My service",
										"url": "http://%%host%%",
										"timeout": 1
									}
								]
							}
						}`,
						"ad.datadoghq.com/apache.check_names":  "[\"invalid\"]",
						"ad.datadoghq.com/apache.init_configs": "[{}]",
						"ad.datadoghq.com/apache.instances":    "[{}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
				},
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
			},
			expectedErr: types.ErrorMsgSet{
				"ad.datadoghq.com/apache.checks takes precedence, ignoring ad.datadoghq.com/apache.check_names, ad.datadoghq.com/apache.init_configs, ad.datadoghq.com/apache.instances: Autodiscovery only applies the check configuration with the highest priority (v2, then v1, then legacy)": {},
			},
		},
		{
			name: "New + old, new takes over",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":                 "[\"http_check\"]",
						"ad.datadoghq.com/apache.init_configs":                "[{}]",
						"ad.datadoghq.com/apache.instances":                   "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"service-discovery.datadoghq.com/apache.check_names":  "[\"invalid\"]",
						"service-discovery.datadoghq.com/apache.init_configs": "[{}]",
						"service-discovery.datadoghq.com/apache.instances":    "[{}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
				},
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
			},
			expectedErr: types.ErrorMsgSet{
				"ad.datadoghq.com/apache.check_names takes precedence, ignoring service-discovery.datadoghq.com/apache.check_names, service-discovery.datadoghq.com/apache.init_configs, service-discovery.datadoghq.com/apache.instances: Autodiscovery only applies the check configuration with the highest priority (v2, then v1, then legacy)": {},
			},
		},
		{
			name: "New annotation prefix, two templates",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/apache.init_configs": "[{}]",
						"ad.datadoghq.com/apache.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/nginx.check_names":   "[\"http_check\"]",
						"ad.datadoghq.com/nginx.init_configs":  "[{}]",
						"ad.datadoghq.com/nginx.instances":     "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
				},
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"Other service\",\"timeout\":1,\"url\":\"http://%%host_external%%\"}")},
					Source:        "container:docker://4ac8352d70bf1",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Legacy annotation prefix, two checks in one template",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"service-discovery.datadoghq.com/apache.check_names":  "[\"apache\",\"http_check\"]",
						"service-discovery.datadoghq.com/apache.init_configs": "[{},{}]",
						"service-discovery.datadoghq.com/apache.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"},{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
				},
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "apache",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Custom check ID",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/nginx.check.id":            "nginx-custom",
						"ad.datadoghq.com/nginx-custom.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/nginx-custom.init_configs": "[{}]",
						"ad.datadoghq.com/nginx-custom.instances":    "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
				},
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"Other service\",\"timeout\":1,\"url\":\"http://%%host_external%%\"}")},
					Source:        "container:docker://4ac8352d70bf1",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Custom check ID with Init Container",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/nginx.check.id":            "nginx-custom",
						"ad.datadoghq.com/nginx-custom.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/nginx-custom.init_configs": "[{}]",
						"ad.datadoghq.com/nginx-custom.instances":    "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					},
				},
				InitContainers: []workloadmeta.OrchestratorContainer{
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
				},
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"Other service\",\"timeout\":1,\"url\":\"http://%%host_external%%\"}")},
					Source:        "container:docker://4ac8352d70bf1",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Non-duplicate errors",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "nginx-1752f8c774-wtjql",
					Namespace: "testNamespace",
					Annotations: map[string]string{
						"ad.datadoghq.com/nonmatching.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/nonmatching.init_configs": "[{}]",
						"ad.datadoghq.com/nonmatching.instances":    "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
				},
			},
			expectedErr: types.ErrorMsgSet{
				"annotation ad.datadoghq.com/nonmatching.check_names is invalid: nonmatching doesn't match a container identifier [apache nginx]":  {},
				"annotation ad.datadoghq.com/nonmatching.init_configs is invalid: nonmatching doesn't match a container identifier [apache nginx]": {},
				"annotation ad.datadoghq.com/nonmatching.instances is invalid: nonmatching doesn't match a container identifier [apache nginx]":    {},
			},
		},
		{
			name: "One invalid config, one valid config",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/nginx.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/nginx.init_configs": "[{}]",
						"ad.datadoghq.com/nginx.instances":    "[{\"name\": \"nginx\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/nginx.logs":         "[{\"source\": \"nginx\" \"service\": \"nginx\"}]", // invalid json
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
				},
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"nginx\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://4ac8352d70bf1",
				},
			},
			expectedErr: types.ErrorMsgSet{
				"could not extract logs config: in logs: invalid character '\"' after object key:value pair": {},
			},
		},
		{
			name: "bare container, container collect all",
			entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					ID: "3b8efe0c50e8",
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "container_collect_all",
					Source:        "container:docker://3b8efe0c50e8",
					LogsConfig:    integration.Data("[{}]"),
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
				},
			},
			containerCollectAll: true,
		},
		{
			name: "check defined for ephemeral container",
			entity: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.checks": `{
							"http_check": {
								"instances": [
									{
										"name": "My service",
										"url": "http://%%host%%",
										"timeout": 1
									}
								]
							}
						}`,
					},
				},
				EphemeralContainers: []workloadmeta.OrchestratorContainer{ // Targeted by the annotation
					{
						Name: "apache",
						ID:   "ephemeral-id",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{ // Not targeted by the annotation
					{
						Name: "nginx",
						ID:   "non-ephemeral-id",
					},
				},
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://ephemeral-id"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://ephemeral-id",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				fx.Provide(func() config.Component {
					return config.NewMockWithOverrides(t, map[string]interface{}{
						"logs_config.container_collect_all": tt.containerCollectAll,
					})
				}),
				fx.Provide(func() log.Component { return logmock.New(t) }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			if pod, ok := tt.entity.(*workloadmeta.KubernetesPod); ok {
				for _, c := range pod.GetAllContainers() {
					store.Set(&workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainer,
							ID:   c.ID,
						},
						Runtime: workloadmeta.ContainerRuntimeDocker,
					})
				}
			}

			provider, providerErr := NewContainerConfigProvider(nil, store, nil, nil, healthplatformnoop.NewNoopComponent(), nil)
			require.NoError(t, providerErr)
			cp := provider.(*ContainerConfigProvider)

			configs, err, _ := cp.generateConfig(tt.entity)

			assert.Equal(t, tt.expectedConfigs, configs)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func basicDockerContainer() *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "3b8efe0c50e8",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels: map[string]string{
				"com.datadoghq.ad.check_names":  "[\"apache\",\"http_check\"]",
				"com.datadoghq.ad.init_configs": "[{}, {}]",
				"com.datadoghq.ad.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"},{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}
}

func basicDockerContainerSingleCheck() *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "3b8efe0c50e8",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels: map[string]string{
				"com.datadoghq.ad.check_names":  "[\"apache\"]",
				"com.datadoghq.ad.init_configs": "[{}]",
				"com.datadoghq.ad.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"}]",
			},
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}
}

func basicDockerConfigs() []integration.Config {
	return []integration.Config{
		{
			Name:          "apache",
			ADIdentifiers: []string{"docker://3b8efe0c50e8"},
			InitConfig:    integration.Data("{}"),
			Source:        "container:docker://3b8efe0c50e8",
			Instances: []integration.Data{
				integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}"),
			},
		},
		{
			Name:          "http_check",
			ADIdentifiers: []string{"docker://3b8efe0c50e8"},
			InitConfig:    integration.Data("{}"),
			Source:        "container:docker://3b8efe0c50e8",
			Instances: []integration.Data{
				integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}"),
			},
		},
	}
}
