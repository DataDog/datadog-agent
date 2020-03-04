package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetOrCreateClusterID(t *testing.T) {
	client := fake.NewSimpleClientset().CoreV1()

	// kubernetes service doesn't exist
	GetOrCreateClusterID(client)

	cm, err := client.ConfigMaps("default").Get(defaultClusterIDMap, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	// kubernetes service does exist
	kSvc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             "226430c6-5e57-11ea-91d5-42010a8400c6",
			Name:            "kubernetes",
			Namespace:       "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.1",
			Ports:     []corev1.ServicePort{},
		},
	}
	client.Services("default").Create(&kSvc)

	GetOrCreateClusterID(client)

	cm, err = client.ConfigMaps("default").Get(defaultClusterIDMap, metav1.GetOptions{})
	assert.Nil(t, err)
	id, found := cm.Data["id"]
	assert.True(t, found)
	assert.Equal(t, "226430c6-5e57-11ea-91d5-42010a8400c6", id)
}
