package functiontools

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type deletePodResponse struct {
	Status    string `json:"status"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func deletePod(namespace string, name string) (*deletePodResponse, error) {
	client, err := _getClient()
	if err != nil {
		return nil, err
	}

	// Define Pod GVR
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	// Delete the pod
	ctx := context.Background()
	err = client.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return nil, err
	}
	return &deletePodResponse{
		Status:    "deleted",
		Namespace: namespace,
		Name:      name,
	}, nil
}
