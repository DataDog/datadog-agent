// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver && cel

package providers

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

var tplCel = integration.Config{
	Name: "check-cel",
	CELSelector: workloadfilter.Rules{
		KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`},
	},
}

// Initialize the shared dummy CEL template with a matching program
func init() {
	matchingProgram, celADID, compileErr, recErr := integration.CreateMatchingProgram(tplCel.CELSelector)
	if compileErr != nil {
		panic("failed to compile CEL matching program: " + compileErr.Error())
	}
	if recErr != nil {
		panic("failed to create CEL matching program: " + recErr.Error())
	}
	if celADID != adtypes.CelEndpointIdentifier {
		panic("expected CEL identifier to be " + string(adtypes.CelEndpointIdentifier) + " but got " + string(celADID))
	}
	tplCel.SetMatchingProgram(matchingProgram)
}

func TestBuildConfigStore(t *testing.T) {
	tpl1 := &integration.Config{
		Name: "check1",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ep-ns1", "ep-name1", ""),
		}},
	}

	tpl2 := &integration.Config{
		Name: "check2",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ep-ns2", "ep-name2", ""),
		}},
	}

	tpl3 := &integration.Config{
		Name: "check3",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ep-ns3", "ep-name3", ""),
			KubeService:   kubeNsName("svc-ns", "svc-name"),
		}},
	}

	tpl4 := &integration.Config{
		Name: "check4",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeService: kubeNsName("svc-ns", "svc-name"),
		}},
	}

	tests := []struct {
		name      string
		templates []integration.Config
		want      map[string]*epConfig
	}{
		{
			name:      "empty",
			templates: []integration.Config{},
			want:      map[string]*epConfig{},
		},
		{
			name:      "with ep templates",
			templates: []integration.Config{*tpl1, *tpl2},
			want: map[string]*epConfig{
				"ep-ns1/ep-name1": {
					templates:     []integration.Config{*tpl1},
					eps:           nil,
					shouldCollect: false,
					resolveMode:   kubeEndpointResolveAuto,
				},
				"ep-ns2/ep-name2": {
					templates:     []integration.Config{*tpl2},
					eps:           nil,
					shouldCollect: false,
					resolveMode:   kubeEndpointResolveAuto,
				},
			},
		},
		{
			name:      "with ep and svc templates",
			templates: []integration.Config{*tpl3, *tpl4},
			want: map[string]*epConfig{
				"ep-ns3/ep-name3": {
					templates:     []integration.Config{*tpl3},
					eps:           nil,
					shouldCollect: false,
					resolveMode:   kubeEndpointResolveAuto,
				},
			},
		},
		{
			name:      "with cel template",
			templates: []integration.Config{tplCel},
			want: map[string]*epConfig{
				celEndpointID: {
					templates:     []integration.Config{tplCel},
					eps:           nil,
					shouldCollect: false,
					resolveMode:   kubeEndpointResolveAuto,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &KubeEndpointsFileConfigProvider{}
			p.buildConfigStore(tt.templates)
			// Ignore unexported matchingProgram in the integration.Config
			// because it gets recompiled in buildConfigStore with a different signature
			if diff := cmp.Diff(tt.want, p.store.epConfigs,
				cmp.AllowUnexported(epConfig{}),
				cmpopts.IgnoreUnexported(integration.Config{})); diff != "" {
				t.Errorf("buildConfigStore() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStoreInsertEp(t *testing.T) {
	tpl := integration.Config{
		Name: "check1",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ep-ns1", "ep-name1", ""),
		}},
	}

	ns1CelTpl := integration.Config{
		Name: "check2",
		CELSelector: workloadfilter.Rules{
			KubeEndpoints: []string{`kube_endpoint.namespace == "ns1" && kube_endpoint.name == "ep1"`},
		},
	}

	ep1 := &v1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "ep1", Namespace: "ns1"}}

	tests := []struct {
		name          string
		epConfigs     map[string]*epConfig
		ep            *v1.Endpoints
		want          bool
		wantEpConfigs map[string]*epConfig
	}{
		{
			name:          "not found",
			epConfigs:     make(map[string]*epConfig),
			ep:            ep1,
			want:          false,
			wantEpConfigs: make(map[string]*epConfig),
		},
		{
			name:          "found",
			epConfigs:     map[string]*epConfig{"ns1/ep1": {templates: []integration.Config{tpl}, eps: nil, shouldCollect: false}},
			ep:            ep1,
			want:          true,
			wantEpConfigs: map[string]*epConfig{"ns1/ep1": {templates: []integration.Config{tpl}, eps: map[*v1.Endpoints]struct{}{ep1: {}}, shouldCollect: true}},
		},
		{
			name: "not found with cel template",
			epConfigs: map[string]*epConfig{celEndpointID: {
				templates: []integration.Config{
					tplCel,
				},
				eps: nil,
			}},
			ep:   ep1,
			want: false,
			// Expect no changes because the inserted Ep does not match the CEL template
			wantEpConfigs: map[string]*epConfig{celEndpointID: {
				templates: []integration.Config{
					tplCel,
				},
				eps: nil,
			}},
		},
		{
			name: "found and inserts into both AdvancedAD and CEL configurations",
			epConfigs: map[string]*epConfig{
				"ns1/ep1":     {templates: []integration.Config{tpl}, eps: nil},
				celEndpointID: {templates: []integration.Config{ns1CelTpl}, eps: nil},
			},
			ep:   ep1,
			want: true,
			wantEpConfigs: map[string]*epConfig{
				"ns1/ep1":     {templates: []integration.Config{tpl}, eps: map[*v1.Endpoints]struct{}{ep1: {}}, shouldCollect: true},
				celEndpointID: {templates: []integration.Config{ns1CelTpl}, eps: map[*v1.Endpoints]struct{}{ep1: {}}, shouldCollect: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &store{epConfigs: tt.epConfigs}

			got := s.insertEp(tt.ep)

			assert.Equal(t, tt.want, got)
			assert.EqualValues(t, tt.wantEpConfigs, s.epConfigs)
		})
	}
}
func TestCELSelector(t *testing.T) {
	// Template 1: Only production namespace
	tpl1 := integration.Config{
		Name: "prod-check",
		CELSelector: workloadfilter.Rules{
			KubeEndpoints: []string{`kube_endpoint.namespace == "production" && kube_endpoint.name.matches("")`},
		},
	}

	// Template 2: Only redis endpoints
	tpl2 := integration.Config{
		Name: "redis-check",
		CELSelector: workloadfilter.Rules{
			KubeEndpoints: []string{`kube_endpoint.namespace.matches("") && kube_endpoint.name == "redis"`},
		},
	}

	p := &KubeEndpointsFileConfigProvider{}
	p.buildConfigStore([]integration.Config{tpl1, tpl2})

	// Verify both templates were stored with their matching programs
	assert.Contains(t, p.store.epConfigs, celEndpointID)
	assert.Len(t, p.store.epConfigs[celEndpointID].templates, 2, "Should have 2 templates")

	// Verify each template has a matching program by testing IsMatched
	templates := p.store.epConfigs[celEndpointID].templates

	prodEp := workloadfilter.CreateKubeEndpoint("test", "production", nil)
	devEp := workloadfilter.CreateKubeEndpoint("test", "development", nil)
	assert.True(t, templates[0].IsMatched(prodEp))
	assert.False(t, templates[0].IsMatched(devEp))

	redisEp := workloadfilter.CreateKubeEndpoint("redis", "default", nil)
	mongoEp := workloadfilter.CreateKubeEndpoint("mongo", "default", nil)
	assert.True(t, templates[1].IsMatched(redisEp))
	assert.False(t, templates[1].IsMatched(mongoEp))

	// Create the dummy endpoints for each service
	prodRedis := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: "production",
		},
	}

	devRedis := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: "development",
		},
	}

	prodMongo := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mongo",
			Namespace: "production",
		},
	}

	devMongo := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mongo",
			Namespace: "development",
		},
	}

	// Test insertEp - should cache endpoints passing any matching program
	s := p.store

	// prodRedis matches BOTH templates (production namespace AND redis name)
	updated := s.insertEp(prodRedis)
	assert.True(t, updated, "prodRedis should match")
	assert.Contains(t, s.epConfigs[celEndpointID].eps, prodRedis)

	// devRedis matches only template 2 (redis name)
	updated = s.insertEp(devRedis)
	assert.True(t, updated, "devRedis should match (name=redis)")
	assert.Contains(t, s.epConfigs[celEndpointID].eps, devRedis)

	// prodMongo matches only template 1 (production namespace)
	updated = s.insertEp(prodMongo)
	assert.True(t, updated, "prodMongo should match (namespace=production)")
	assert.Contains(t, s.epConfigs[celEndpointID].eps, prodMongo)

	// devMongo matches NEITHER template
	updated = s.insertEp(devMongo)
	assert.False(t, updated, "devMongo should NOT match")
	assert.NotContains(t, s.epConfigs[celEndpointID].eps, devMongo)

	// Verify all 3 matching endpoints are cached
	assert.Len(t, s.epConfigs[celEndpointID].eps, 3, "Should have 3 cached endpoints")

	// Test shouldHandle
	assert.True(t, s.shouldHandle(prodRedis), "prodRedis should be handled")
	assert.True(t, s.shouldHandle(devRedis), "devRedis should be handled")
	assert.True(t, s.shouldHandle(prodMongo), "prodMongo should be handled")
	assert.False(t, s.shouldHandle(devMongo), "devMongo should NOT be handled")
}

func TestStoreGenerateConfigs(t *testing.T) {
	tpl1 := integration.Config{
		Name: "check1",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ep-ns1", "ep-name1", ""),
		}},
	}

	tpl2 := integration.Config{
		Name: "check2",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ep-ns2", "ep-name2", ""),
		}},
	}

	node1, node2 := "node1", "node2"
	ep1 := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "ep1", Namespace: "ns1"},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: "10.0.0.1", NodeName: &node1, TargetRef: &v1.ObjectReference{UID: types.UID("pod-1"), Kind: "Pod"}},
					{IP: "10.0.0.2", NodeName: &node2, TargetRef: &v1.ObjectReference{UID: types.UID("pod-2"), Kind: "Pod"}},
				},
			},
		},
	}

	// Additional endpoint for CEL test
	ep2 := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "ep2", Namespace: "ns2"},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: "10.0.1.1", NodeName: &node1, TargetRef: &v1.ObjectReference{UID: types.UID("pod-3"), Kind: "Pod"}},
					{IP: "10.0.1.2", NodeName: &node2, TargetRef: &v1.ObjectReference{UID: types.UID("pod-4"), Kind: "Pod"}},
				},
			},
		},
	}

	tests := []struct {
		name      string
		epConfigs map[string]*epConfig
		want      []integration.Config
	}{
		{
			name: "nominal case",
			epConfigs: map[string]*epConfig{
				"ns1/ep1": {templates: []integration.Config{tpl1}, eps: map[*v1.Endpoints]struct{}{ep1: {}}, shouldCollect: true},
				"ns2/ep2": {templates: []integration.Config{tpl2}, eps: nil, shouldCollect: false},
			},
			want: []integration.Config{
				{
					Name:                  "check1",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.1",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.1", "kubernetes_pod://pod-1"},
					AdvancedADIdentifiers: nil,
					NodeName:              "node1",
					Provider:              "kubernetes-endpoints-file",
					ClusterCheck:          true,
				},
				{
					Name:                  "check1",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.2",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.2", "kubernetes_pod://pod-2"},
					AdvancedADIdentifiers: nil,
					NodeName:              "node2",
					Provider:              "kubernetes-endpoints-file",
					ClusterCheck:          true,
				},
			},
		},
		{
			name: "should not collect",
			epConfigs: map[string]*epConfig{
				"ns1/ep1": {templates: []integration.Config{tpl1}, eps: map[*v1.Endpoints]struct{}{ep1: {}}, shouldCollect: false},
			},
			want: []integration.Config{},
		},
		{
			name: "cel case with multiple endpoints",
			epConfigs: map[string]*epConfig{
				celEndpointID: {
					templates: []integration.Config{tplCel},
					eps: map[*v1.Endpoints]struct{}{
						ep1: {},
						ep2: {},
					},
					shouldCollect: true,
					resolveMode:   kubeEndpointResolveAuto,
				},
			},
			want: []integration.Config{
				{
					Name:                  "check-cel",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.1",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.1", "kubernetes_pod://pod-1"},
					CELSelector:           workloadfilter.Rules{KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`}},
					AdvancedADIdentifiers: nil,
					NodeName:              "node1",
					Provider:              "kubernetes-endpoints-file",
					ClusterCheck:          true,
				},
				{
					Name:                  "check-cel",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.2",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.2", "kubernetes_pod://pod-2"},
					CELSelector:           workloadfilter.Rules{KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`}},
					AdvancedADIdentifiers: nil,
					NodeName:              "node2",
					Provider:              "kubernetes-endpoints-file",
					ClusterCheck:          true,
				},
				{
					Name:                  "check-cel",
					ServiceID:             "kube_endpoint_uid://ns2/ep2/10.0.1.1",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns2/ep2/10.0.1.1", "kubernetes_pod://pod-3"},
					CELSelector:           workloadfilter.Rules{KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`}},
					AdvancedADIdentifiers: nil,
					NodeName:              "node1",
					Provider:              "kubernetes-endpoints-file",
					ClusterCheck:          true,
				},
				{
					Name:                  "check-cel",
					ServiceID:             "kube_endpoint_uid://ns2/ep2/10.0.1.2",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns2/ep2/10.0.1.2", "kubernetes_pod://pod-4"},
					CELSelector:           workloadfilter.Rules{KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`}},
					AdvancedADIdentifiers: nil,
					NodeName:              "node2",
					Provider:              "kubernetes-endpoints-file",
					ClusterCheck:          true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &store{epConfigs: tt.epConfigs}

			assert.ElementsMatch(t, tt.want, s.generateConfigs())
		})
	}
}

func TestEndpointChecksFromTemplateWithResolveMode(t *testing.T) {
	tpl := integration.Config{
		Name: "http_check",
		Instances: []integration.Data{
			integration.Data(`{"name": "test", "url": "http://%%host%%"}`),
		},
	}

	node1, node2 := "node1", "node2"
	ep := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "ep1", Namespace: "ns1"},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: "10.0.0.1", NodeName: &node1, TargetRef: &v1.ObjectReference{UID: types.UID("pod-1"), Kind: "Pod"}},
					{IP: "10.0.0.2", NodeName: &node2, TargetRef: &v1.ObjectReference{UID: types.UID("pod-2"), Kind: "Pod"}},
				},
			},
		},
	}

	tests := []struct {
		name               string
		resolveMode        endpointResolveMode
		expectedNodeNames  []string
		expectedServiceIDs []string
	}{
		{
			name:               "Auto mode",
			resolveMode:        "auto",
			expectedNodeNames:  []string{"node1", "node2"},
			expectedServiceIDs: []string{"kube_endpoint_uid://ns1/ep1/10.0.0.1", "kube_endpoint_uid://ns1/ep1/10.0.0.2"},
		},
		{
			name:               "IP mode",
			resolveMode:        "ip",
			expectedNodeNames:  []string{"", ""},
			expectedServiceIDs: []string{"kube_endpoint_uid://ns1/ep1/10.0.0.1", "kube_endpoint_uid://ns1/ep1/10.0.0.2"},
		},
		{
			name:               "Unknown mode",
			resolveMode:        "unknown",
			expectedNodeNames:  []string{"node1", "node2"},
			expectedServiceIDs: []string{"kube_endpoint_uid://ns1/ep1/10.0.0.1", "kube_endpoint_uid://ns1/ep1/10.0.0.2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs := endpointChecksFromTemplate(tpl, ep, tt.resolveMode)
			assert.Len(t, configs, len(tt.expectedNodeNames))
			for i := range configs {
				assert.Equal(t, tt.expectedNodeNames[i], configs[i].NodeName)
				assert.Equal(t, tt.expectedServiceIDs[i], configs[i].ServiceID)
			}
		})
	}
}

func kubeEndpointIdentifier(ns string, name string, resolve string) integration.KubeEndpointsIdentifier {
	return integration.KubeEndpointsIdentifier{
		KubeNamespacedName: integration.KubeNamespacedName{
			Name:      name,
			Namespace: ns,
		},
		Resolve: resolve,
	}
}
