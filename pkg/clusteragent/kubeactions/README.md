# Kubernetes Actions

A Datadog cluster-agent feature for executing one-time Kubernetes actions via remote configuration.

## Features

- **Action Execution**: Execute Kubernetes actions like delete pod, restart deployment, etc.
- **Duplicate Prevention**: Tracks executed actions by metadata-id + version to prevent re-execution
- **Persistent Storage**: Stores action history in a ConfigMap to survive restarts
- **Extensible**: Easy to add new action types and executors
- **Validation Framework**: Safety checker hook for custom validation logic
- **Cluster-wide Support**: Works with both namespaced and cluster-scoped resources
- **Result Reporting**: Structure in place (disabled by default)
- **Leader Election**: Only the leader processes actions

## Configuration

Enable in `datadog.yaml`:

```yaml
kubeactions:
  enabled: true
  persistent_store:
    namespace: "datadog"  # Optional, defaults to "default"
```

## Implemented Actions

- **delete_pod**: Deletes a Kubernetes pod
- **restart_deployment**: Restarts a deployment by updating restart annotation

## Example Payload

```json
{
  "actions": [
    {
      "action_type": "delete_pod",
      "resource": {
        "api_version": "v1",
        "kind": "Pod",
        "namespace": "default",
        "name": "my-pod-xyz"
      },
      "timestamp": {
        "seconds": 1734567890,
        "nanos": 0
      }
    }
  ]
}
```

## Testing

### 1. Build the cluster-agent

```bash
cd /Users/frank.spano/go/src/github.com/DataDog/datadog-agent

# Update dependencies first
go get github.com/DataDog/agent-payload/v5@latest
go mod tidy

# Build
dda inv cluster-agent.build
```

###2. Create a test cluster

```bash
# Using minikube or kind
minikube start
# or
kind create cluster
```

### 3. Deploy the cluster-agent

Create a configmap with your config:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: datadog-cluster-agent-config
  namespace: default
data:
  datadog.yaml: |
    api_key: YOUR_API_KEY
    cluster_name: test-cluster
    kubeactions:
      enabled: true
      persistent_store:
        namespace: default
    remote_configuration:
      enabled: true
      api_key: YOUR_API_KEY
```

Deploy the cluster-agent with the built binary.

### 4. Send a test action via remote config

Use the Datadog remote config API or UI to send a test action payload.

### 5. Verify execution

Check logs:
```bash
kubectl logs -f <cluster-agent-pod> -n default
```

Check the persistent store:
```bash
kubectl get configmap datadog-kubeactions-state -n default -o yaml
```

## Adding New Actions

1. Create a new executor in `pkg/clusteragent/kubeactions/executors/`:

```go
package executors

import (
	"context"
	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"k8s.io/client-go/kubernetes"
)

type MyActionExecutor struct {
	clientset kubernetes.Interface
}

func NewMyActionExecutor(clientset kubernetes.Interface) *MyActionExecutor {
	return &MyActionExecutor{clientset: clientset}
}

func (e *MyActionExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	// Implement your action here
	return ExecutionResult{
		Status:  "success",
		Message: "action completed",
	}
}
```

2. Register it in `setup.go`:

```go
registry.Register("my_action", &executorAdapter{exec: executors.NewMyActionExecutor(clientset)})
```

## Architecture

- **config_retriever.go**: Subscribes to remote config updates
- **processor.go**: Processes actions and coordinates execution
- **action_store.go**: In-memory tracking of executed actions
- **persistent_store.go**: ConfigMap-based persistence
- **validator.go**: Validation framework with safety hooks
- **executor.go**: Registry for action executors
- **executors/**: Individual action executor implementations
- **reporter.go**: Result reporting (future use)

## Security Considerations

- Only the leader agent processes actions
- All actions are validated before execution
- Execution history is persisted to prevent duplicates
- Add custom validation logic in `validator.go` as needed
- Consider RBAC permissions for the cluster-agent service account

## Troubleshooting

**Actions not executing:**
- Check if `kubeactions.enabled` is true
- Verify remote config is enabled and connected
- Check if the pod is the leader: look for "leader: true" in logs
- Verify RBAC permissions

**Actions executing multiple times:**
- Check ConfigMap persistence: `kubectl get cm datadog-kubeactions-state`
- Check for leader election issues
- Verify metadata-id and version are present in payloads

**Storage issues:**
- Verify namespace exists
- Check service account has ConfigMap read/write permissions
- Look for "Failed to persist action state" in logs
