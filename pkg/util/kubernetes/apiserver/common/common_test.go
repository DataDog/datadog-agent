package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetOrCreateClusterID(t *testing.T) {
	client := fake.NewSimpleClientset().CoreV1()

	GetOrCreateClusterID(client)

	cm, err := client.ConfigMaps("default").Get(defaultClusterIDMap, metav1.GetOptions{})
	assert.Nil(t, err)
	id, found := cm.Data["id"]
	assert.True(t, found)
	assert.Equal(t, 36, len([]byte(id)))
}
