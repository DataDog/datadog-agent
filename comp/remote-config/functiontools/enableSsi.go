package functiontools

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func createSsiCrd(namespace string, instanceName string, tracerVersions string) (*unstructured.Unstructured, error) {
	tracerVersionsList := []string{}
	for _, tracer := range strings.Split(tracerVersions, ",") {
		tracerParts := strings.Split(tracer, ":")
		if len(tracerParts) != 2 {
			return nil, fmt.Errorf("invalid tracer format: %s", tracer)
		}
		tracerName := strings.TrimSpace(tracerParts[0])
		tracerVersion := strings.TrimSpace(tracerParts[1])
		tracerVersionsList = append(tracerVersionsList, fmt.Sprintf(`%[1]s: "%[2]s"`, tracerName, tracerVersion))
	}
	tracerVersionsYaml := strings.Join(tracerVersionsList, "\n    ")

	yamlContent := fmt.Sprintf(`
apiVersion: datadoghq.com/v1alpha1
kind: DatadogServiceMonitor
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  name: "%[1]s"
  podSelector:
    matchLabels:
      app.kubernetes.io/instance: %[1]s
      app.kubernetes.io/name: app
  namespaceSelector:
    matchNames:
      - "%[2]s"
  ddTraceVersions:
    %[3]s
  ddTraceConfigs:
    - name: "DD_PROFILING_ENABLED"
      value: "true"
`, instanceName, namespace, tracerVersionsYaml)

	// Parse YAML content
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(yamlContent), obj); err != nil {
		return nil, err
	}

	return obj, nil
}

type enableSsiResponse struct {
	Status         string `json:"status"`
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	TracerVersions string `json:"tracer_versions"`
}

func enableSsi(namespace string, instanceName string, tracerVersions string) (*enableSsiResponse, error) {
	obj, err := createSsiCrd(namespace, instanceName, tracerVersions)
	if err != nil {
		return nil, err
	}

	client, err := _getClient()
	if err != nil {
		return nil, err
	}

	gvr := _getGVR(obj)

	// Apply (create or update)
	ctx := context.Background()
	existing, err := client.Resource(gvr).Namespace(obj.GetNamespace()).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err == nil {
		// Resource exists, update it
		obj.SetResourceVersion(existing.GetResourceVersion())
		_, err = client.Resource(gvr).Namespace(obj.GetNamespace()).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return &enableSsiResponse{
			Status:         "updated",
			Namespace:      obj.GetNamespace(),
			Name:           obj.GetName(),
			TracerVersions: tracerVersions,
		}, nil
	}

	// Resource doesn't exist, create it
	_, err = client.Resource(gvr).Namespace(obj.GetNamespace()).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return &enableSsiResponse{
		Status:         "created",
		Namespace:      obj.GetNamespace(),
		Name:           obj.GetName(),
		TracerVersions: tracerVersions,
	}, nil
}
