package functiontools

import (
	"context"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func _getClient() (*dynamic.DynamicClient, error) {
	// Load kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("HOME")+"/.kube/config")
	if err != nil {
		return nil, err
	}

	// Create dynamic client
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func _getYamlObject(filepath string) (*unstructured.Unstructured, error) {
	// Read and parse YAML file
	yamlFile, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(yamlFile, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

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

func applyK8sResource(filepath string) (string, error) {
	client, err := _getClient()
	if err != nil {
		return "", err
	}
	obj, err := _getYamlObject(filepath)
	if err != nil {
		return "", err
	}
	gvr := _getGVR(obj)
	namespace := obj.GetNamespace()

	// Apply (create or update)
	ctx := context.Background()
	_, err = client.Resource(gvr).Namespace(namespace).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err == nil {
		// Update
		// _, err = client.Resource(gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
		// if err != nil {
		// 	return "", err
		// }
		return "update not implemented", nil
	}

	// Create
	_, err = client.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return "created", nil
}

func deleteK8sResource(filepath string) (string, error) {
	client, err := _getClient()
	if err != nil {
		return "", err
	}
	obj, err := _getYamlObject(filepath)
	if err != nil {
		return "", err
	}
	gvr := _getGVR(obj)

	namespace := obj.GetNamespace()

	// Delete the resource
	ctx := context.Background()
	err = client.Resource(gvr).Namespace(namespace).Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
	if err != nil {
		return "", err
	}
	return "deleted", nil
}
