package functiontools

import (
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func _getClient() (*dynamic.DynamicClient, error) {
	// Try in-cluster config first (for cluster-agent running in a pod)
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		// Fall back to kubeconfig file (for local development)
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}

	// Create dynamic client
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// func _getYamlObject(filepath string) (*unstructured.Unstructured, error) {
// 	// Read and parse YAML file
// 	yamlFile, err := os.ReadFile(filepath)
// 	if err != nil {
// 		return nil, err
// 	}

// 	obj := &unstructured.Unstructured{}
// 	if err := yaml.Unmarshal(yamlFile, obj); err != nil {
// 		return nil, err
// 	}

// 	return obj, nil
// }

func _getGVR(obj *unstructured.Unstructured) schema.GroupVersionResource {
	// Define the resource
	gvk := obj.GroupVersionKind()
	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: strings.ToLower(gvk.Kind) + "s",
	}

	return gvr
}
