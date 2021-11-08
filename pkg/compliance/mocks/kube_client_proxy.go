package mocks

import (
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	dynamic "k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
)

// KubeClientProxy is a proxy that maps methods to either a mock or a fake client
type KubeClientProxy struct {
	FakeClient *fake.FakeDynamicClient
	MockClient KubeClient
}

// Resource maps the call to the underlying fake client
func (p *KubeClientProxy) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return p.FakeClient.Resource(resource)
}

// ClusterID maps the call to the underlying mock
func (p *KubeClientProxy) ClusterID() (string, error) {
	return p.MockClient.ClusterID()
}
