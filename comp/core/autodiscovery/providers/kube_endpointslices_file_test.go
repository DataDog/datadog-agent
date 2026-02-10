// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

var tplCelSlice = integration.Config{
	Name: "check-cel",
	CELSelector: workloadfilter.Rules{
		KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`},
	},
}

// Initialize the shared dummy CEL template with a matching program
func init() {
	matchingProgram, celADID, compileErr, recErr := integration.CreateMatchingProgram(tplCelSlice.CELSelector)
	if compileErr != nil {
		panic("failed to compile CEL matching program: " + compileErr.Error())
	}
	if recErr != nil {
		panic("failed to create CEL matching program: " + recErr.Error())
	}
	if celADID != adtypes.CelEndpointIdentifier {
		panic("expected CEL identifier to be " + string(adtypes.CelEndpointIdentifier) + " but got " + string(celADID))
	}
	tplCelSlice.SetMatchingProgram(matchingProgram)
}

func TestEndpointSlices_BuildConfigStore(t *testing.T) {
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
		want      map[string]*epSliceConfig
	}{
		{
			name:      "empty",
			templates: []integration.Config{},
			want:      map[string]*epSliceConfig{},
		},
		{
			name:      "with endpoint templates",
			templates: []integration.Config{*tpl1, *tpl2},
			want: map[string]*epSliceConfig{
				"ep-ns1/ep-name1": {
					templates:   []integration.Config{*tpl1},
					slices:      nil,
					resolveMode: kubeEndpointResolveAuto,
				},
				"ep-ns2/ep-name2": {
					templates:   []integration.Config{*tpl2},
					slices:      nil,
					resolveMode: kubeEndpointResolveAuto,
				},
			},
		},
		{
			name:      "with endpoint and service templates",
			templates: []integration.Config{*tpl3, *tpl4},
			want: map[string]*epSliceConfig{
				"ep-ns3/ep-name3": {
					templates:   []integration.Config{*tpl3},
					slices:      nil,
					resolveMode: kubeEndpointResolveAuto,
				},
			},
		},
		{
			name:      "with cel template",
			templates: []integration.Config{tplCelSlice},
			want: map[string]*epSliceConfig{
				celEndpointSliceID: {
					templates:   []integration.Config{tplCelSlice},
					slices:      nil,
					resolveMode: kubeEndpointResolveAuto,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &KubeEndpointSlicesFileConfigProvider{}
			p.buildConfigStore(tt.templates)
			// Ignore unexported matchingProgram because it gets recompiled with different signature
			if diff := cmp.Diff(tt.want, p.store.epSliceConfigs,
				cmp.AllowUnexported(epSliceConfig{}),
				cmpopts.IgnoreUnexported(integration.Config{})); diff != "" {
				t.Errorf("buildConfigStore() mismatch (-wantShouldUpdate +got):\n%s", diff)
			}
		})
	}
}

func TestStoreInsertEndpointSlice(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	tpl := integration.Config{
		Name: "check1",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ns1", "ep1", ""),
		}},
	}

	tplcel := integration.Config{
		Name: "check2",
		CELSelector: workloadfilter.Rules{
			KubeEndpoints: []string{`kube_endpoint.namespace == "ns1" && kube_endpoint.name == "ep1"`},
		},
	}

	slice1 := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ep1-abc",
			Namespace: "ns1",
			UID:       types.UID("slice-uid"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "ep1",
			},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	tests := []struct {
		name              string
		epSliceConfigs    map[string]*epSliceConfig
		slice             *discv1.EndpointSlice
		wantShouldUpdate  bool
		wantEpSliceConfig map[string]*epSliceConfig
	}{
		{
			name:             "not found",
			epSliceConfigs:   make(map[string]*epSliceConfig),
			slice:            slice1,
			wantShouldUpdate: false,
		},
		{
			name: "found",
			epSliceConfigs: map[string]*epSliceConfig{
				"ns1/ep1": {templates: []integration.Config{tpl}, slices: nil},
			},
			slice:            slice1,
			wantShouldUpdate: true,
			wantEpSliceConfig: map[string]*epSliceConfig{
				"ns1/ep1": {
					templates: []integration.Config{tpl},
					slices:    map[string]*discv1.EndpointSlice{string(slice1.UID): slice1},
				},
			},
		},
		{
			name: "not found with cel template",
			epSliceConfigs: map[string]*epSliceConfig{
				celEndpointSliceID: {
					templates: []integration.Config{tplCelSlice},
					slices:    nil,
				},
			},
			slice:            slice1,
			wantShouldUpdate: false,
		},
		{
			name: "found and inserts into both AdvancedAD and CEL configurations",
			epSliceConfigs: map[string]*epSliceConfig{
				"ns1/ep1": {templates: []integration.Config{tpl}, slices: nil},
				celEndpointSliceID: {
					templates: []integration.Config{tplcel},
					slices:    nil,
				},
			},
			slice:            slice1,
			wantShouldUpdate: true,
			wantEpSliceConfig: map[string]*epSliceConfig{
				"ns1/ep1": {
					templates: []integration.Config{tpl},
					slices:    map[string]*discv1.EndpointSlice{string(slice1.UID): slice1},
				},
				celEndpointSliceID: {
					templates: []integration.Config{tplcel},
					slices:    map[string]*discv1.EndpointSlice{string(slice1.UID): slice1},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &endpointSliceStore{epSliceConfigs: tt.epSliceConfigs}

			got := s.insertSlice(tt.slice)

			assert.Equal(t, tt.wantShouldUpdate, got)
			if tt.wantEpSliceConfig != nil {
				if diff := cmp.Diff(tt.wantEpSliceConfig, s.epSliceConfigs,
					cmp.AllowUnexported(epSliceConfig{}),
					cmpopts.IgnoreUnexported(integration.Config{})); diff != "" {
					t.Errorf("insertSlice() epSliceConfigs mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestEndpointSlices_CELSelector(t *testing.T) {
	tpl1 := integration.Config{
		Name: "prod-check",
		CELSelector: workloadfilter.Rules{
			KubeEndpoints: []string{`kube_endpoint.namespace == "production" && kube_endpoint.name.matches("")`},
		},
	}

	tpl2 := integration.Config{
		Name: "redis-check",
		CELSelector: workloadfilter.Rules{
			KubeEndpoints: []string{`kube_endpoint.namespace.matches("") && kube_endpoint.name == "redis"`},
		},
	}

	p := &KubeEndpointSlicesFileConfigProvider{}
	p.buildConfigStore([]integration.Config{tpl1, tpl2})

	// Verify both templates were stored with their matching programs
	assert.Contains(t, p.store.epSliceConfigs, celEndpointSliceID)
	assert.Len(t, p.store.epSliceConfigs[celEndpointSliceID].templates, 2, "Should have 2 templates")

	// Verify each template has a matching program by testing IsMatched
	templates := p.store.epSliceConfigs[celEndpointSliceID].templates

	prodEp := workloadfilter.CreateKubeEndpoint("test", "production", nil)
	devEp := workloadfilter.CreateKubeEndpoint("test", "development", nil)
	assert.True(t, templates[0].IsMatched(prodEp))
	assert.False(t, templates[0].IsMatched(devEp))

	redisEp := workloadfilter.CreateKubeEndpoint("redis", "default", nil)
	mongoEp := workloadfilter.CreateKubeEndpoint("mongo", "default", nil)
	assert.True(t, templates[1].IsMatched(redisEp))
	assert.False(t, templates[1].IsMatched(mongoEp))

	port80 := int32(80)
	portName := "http"

	prodRedis := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID("prod-redis-uid"),
			Name:      "redis-abc",
			Namespace: "production",
			Labels: map[string]string{
				"kubernetes.io/service-name": "redis",
			},
		},
		Endpoints: []discv1.Endpoint{{Addresses: []string{"10.0.0.1"}}},
		Ports:     []discv1.EndpointPort{{Name: &portName, Port: &port80}},
	}

	devRedis := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID("dev-redis-uid"),
			Name:      "redis-xyz",
			Namespace: "development",
			Labels: map[string]string{
				"kubernetes.io/service-name": "redis",
			},
		},
		Endpoints: []discv1.Endpoint{{Addresses: []string{"10.0.0.2"}}},
		Ports:     []discv1.EndpointPort{{Name: &portName, Port: &port80}},
	}

	prodMongo := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID("prod-mongo-uid"),
			Name:      "mongo-abc",
			Namespace: "production",
			Labels: map[string]string{
				"kubernetes.io/service-name": "mongo",
			},
		},
		Endpoints: []discv1.Endpoint{{Addresses: []string{"10.0.0.3"}}},
		Ports:     []discv1.EndpointPort{{Name: &portName, Port: &port80}},
	}

	devMongo := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID("dev-mongo-uid"),
			Name:      "mongo-xyz",
			Namespace: "development",
			Labels: map[string]string{
				"kubernetes.io/service-name": "mongo",
			},
		},
		Endpoints: []discv1.Endpoint{{Addresses: []string{"10.0.0.4"}}},
		Ports:     []discv1.EndpointPort{{Name: &portName, Port: &port80}},
	}

	// Test insertSlice - should cache slices passing any matching program
	s := p.store

	// prodRedis matches BOTH templates (production namespace AND redis name)
	updated := s.insertSlice(prodRedis)
	assert.True(t, updated, "prodRedis should match")
	assert.Contains(t, s.epSliceConfigs[celEndpointSliceID].slices, string(prodRedis.UID))

	// devRedis matches only template 2 (redis name)
	updated = s.insertSlice(devRedis)
	assert.True(t, updated, "devRedis should match (name=redis)")
	assert.Contains(t, s.epSliceConfigs[celEndpointSliceID].slices, string(devRedis.UID))

	// prodMongo matches only template 1 (production namespace)
	updated = s.insertSlice(prodMongo)
	assert.True(t, updated, "prodMongo should match (namespace=production)")
	assert.Contains(t, s.epSliceConfigs[celEndpointSliceID].slices, string(prodMongo.UID))

	// devMongo matches NEITHER template
	updated = s.insertSlice(devMongo)
	assert.False(t, updated, "devMongo should NOT match")
	assert.NotContains(t, s.epSliceConfigs[celEndpointSliceID].slices, string(devMongo.UID))

	// Verify all 3 matching slices are cached
	assert.Len(t, s.epSliceConfigs[celEndpointSliceID].slices, 3, "Should have 3 cached slices")

	// Test shouldHandle
	assert.True(t, s.shouldHandle(prodRedis), "prodRedis should be handled")
	assert.True(t, s.shouldHandle(devRedis), "devRedis should be handled")
	assert.True(t, s.shouldHandle(prodMongo), "prodMongo should be handled")
	assert.False(t, s.shouldHandle(devMongo), "devMongo should NOT be handled")
}

func TestEndpointSlices_StoreGenerateConfigs(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	tpl1 := integration.Config{
		Name: "check1",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ns1", "ep1", ""),
		}},
	}

	tpl2 := integration.Config{
		Name: "check2",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeEndpointIdentifier("ns2", "ep2", ""),
		}},
	}

	node1, node2 := "node1", "node2"
	slice1 := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ep1-abc",
			Namespace: "ns1",
			UID:       types.UID("slice-1-uid"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "ep1",
			},
		},
		Endpoints: []discv1.Endpoint{
			{
				Addresses: []string{"10.0.0.1"},
				NodeName:  &node1,
				TargetRef: &v1.ObjectReference{UID: types.UID("pod-1"), Kind: "Pod"},
			},
			{
				Addresses: []string{"10.0.0.2"},
				NodeName:  &node2,
				TargetRef: &v1.ObjectReference{UID: types.UID("pod-2"), Kind: "Pod"},
			},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	slice2 := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ep2-xyz",
			Namespace: "ns2",
			UID:       types.UID("slice-2-uid"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "ep2",
			},
		},
		Endpoints: []discv1.Endpoint{
			{
				Addresses: []string{"10.0.1.1"},
				NodeName:  &node1,
				TargetRef: &v1.ObjectReference{UID: types.UID("pod-3"), Kind: "Pod"},
			},
			{
				Addresses: []string{"10.0.1.2"},
				NodeName:  &node2,
				TargetRef: &v1.ObjectReference{UID: types.UID("pod-4"), Kind: "Pod"},
			},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	tests := []struct {
		name           string
		epSliceConfigs map[string]*epSliceConfig
		want           []integration.Config
	}{
		{
			name: "nominal case",
			epSliceConfigs: map[string]*epSliceConfig{
				"ns1/ep1": {
					templates: []integration.Config{tpl1},
					slices:    map[string]*discv1.EndpointSlice{string(slice1.UID): slice1},
				},
				"ns2/ep2": {
					templates: []integration.Config{tpl2},
					slices:    nil,
				},
			},
			want: []integration.Config{
				{
					Name:                  "check1",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.1",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.1", "kubernetes_pod://pod-1"},
					AdvancedADIdentifiers: nil,
					NodeName:              "node1",
					Provider:              names.KubeEndpointSlicesFile,
					ClusterCheck:          true,
				},
				{
					Name:                  "check1",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.2",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.2", "kubernetes_pod://pod-2"},
					AdvancedADIdentifiers: nil,
					NodeName:              "node2",
					Provider:              names.KubeEndpointSlicesFile,
					ClusterCheck:          true,
				},
			},
		},
		{
			name: "should not collect",
			epSliceConfigs: map[string]*epSliceConfig{
				"ns1/ep1": {
					templates: []integration.Config{tpl1},
					slices:    map[string]*discv1.EndpointSlice{},
				},
			},
			want: []integration.Config{},
		},
		{
			name: "cel case with multiple slices",
			epSliceConfigs: map[string]*epSliceConfig{
				celEndpointSliceID: {
					templates: []integration.Config{tplCelSlice},
					slices: map[string]*discv1.EndpointSlice{
						string(slice1.UID): slice1,
						string(slice2.UID): slice2,
					},
					resolveMode: kubeEndpointResolveAuto,
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
					Provider:              names.KubeEndpointSlicesFile,
					ClusterCheck:          true,
				},
				{
					Name:                  "check-cel",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.2",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.2", "kubernetes_pod://pod-2"},
					CELSelector:           workloadfilter.Rules{KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`}},
					AdvancedADIdentifiers: nil,
					NodeName:              "node2",
					Provider:              names.KubeEndpointSlicesFile,
					ClusterCheck:          true,
				},
				{
					Name:                  "check-cel",
					ServiceID:             "kube_endpoint_uid://ns2/ep2/10.0.1.1",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns2/ep2/10.0.1.1", "kubernetes_pod://pod-3"},
					CELSelector:           workloadfilter.Rules{KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`}},
					AdvancedADIdentifiers: nil,
					NodeName:              "node1",
					Provider:              names.KubeEndpointSlicesFile,
					ClusterCheck:          true,
				},
				{
					Name:                  "check-cel",
					ServiceID:             "kube_endpoint_uid://ns2/ep2/10.0.1.2",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns2/ep2/10.0.1.2", "kubernetes_pod://pod-4"},
					CELSelector:           workloadfilter.Rules{KubeEndpoints: []string{`kube_endpoint.namespace == "default" && kube_endpoint.name.matches("")`}},
					AdvancedADIdentifiers: nil,
					NodeName:              "node2",
					Provider:              names.KubeEndpointSlicesFile,
					ClusterCheck:          true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &endpointSliceStore{epSliceConfigs: tt.epSliceConfigs}

			assert.ElementsMatch(t, tt.want, s.generateConfigs())
		})
	}
}

func TestEndpointSliceChecksFromTemplateWithResolveMode(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	tpl := integration.Config{
		Name: "http_check",
		Instances: []integration.Data{
			integration.Data(`{"name": "test", "url": "http://%%host%%"}`),
		},
	}

	node1, node2 := "node1", "node2"
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ep1-abc",
			Namespace: "ns1",
			Labels: map[string]string{
				"kubernetes.io/service-name": "ep1",
			},
		},
		Endpoints: []discv1.Endpoint{
			{
				Addresses: []string{"10.0.0.1"},
				NodeName:  &node1,
				TargetRef: &v1.ObjectReference{UID: types.UID("pod-1"), Kind: "Pod"},
			},
			{
				Addresses: []string{"10.0.0.2"},
				NodeName:  &node2,
				TargetRef: &v1.ObjectReference{UID: types.UID("pod-2"), Kind: "Pod"},
			},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
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
			resolveMode:        kubeEndpointResolveAuto,
			expectedNodeNames:  []string{"node1", "node2"},
			expectedServiceIDs: []string{"kube_endpoint_uid://ns1/ep1/10.0.0.1", "kube_endpoint_uid://ns1/ep1/10.0.0.2"},
		},
		{
			name:               "IP mode",
			resolveMode:        kubeEndpointResolveIP,
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
			configs := endpointSliceChecksFromTemplate(tpl, slice, tt.resolveMode)
			assert.Len(t, configs, len(tt.expectedNodeNames))
			for i := range configs {
				assert.Equal(t, tt.expectedNodeNames[i], configs[i].NodeName)
				assert.Equal(t, tt.expectedServiceIDs[i], configs[i].ServiceID)
			}
		})
	}
}
