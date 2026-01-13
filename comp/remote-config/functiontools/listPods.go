package functiontools

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func listPods() (map[string][]string, error) {
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

	ctx := context.Background()
	list, err := client.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Map of namespace -> pod names
	podsByNamespace := make(map[string][]string)
	for _, item := range list.Items {
		name, nameFound, _ := unstructured.NestedString(item.Object, "metadata", "name")
		namespace, nsFound, _ := unstructured.NestedString(item.Object, "metadata", "namespace")

		if nameFound && nsFound {
			podsByNamespace[namespace] = append(podsByNamespace[namespace], name)
		}
	}

	return podsByNamespace, nil
}
