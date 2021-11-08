package mocks

import (
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	dynamic "k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
)

type KubeClientProxy struct {
	FakeClient *fake.FakeDynamicClient
	MockClient KubeClient
}

func (p *KubeClientProxy) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return p.FakeClient.Resource(resource)
}

func (p *KubeClientProxy) ClusterID() (string, error) {
	return p.MockClient.ClusterID()
}
