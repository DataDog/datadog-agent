## package `providers`

Providers implement the `ConfigProvider` interface and are responsible for scanning different sources like files on
disk, environment variables, databases or containers and objects metadata, searching for integration configurations. Every configuration, regardless of the format, must specify at least one check `instance`. Providers dump every configuration they find into a `CheckConfig`
struct containing an array of configuration instances. Configuration instances are converted in YAML format so that a
check object will be eventually able to convert them into the appropriate data structure.

Usage example:
```go
var configs []loader.CheckConfig
for _, provider := range configProviders {
  c, _ := provider.Collect(ctx)
  configs = append(configs, c...)
}
```

## Config Providers

### `FileConfigProvider`

The `FileConfigProvider` is a file-based config provider. By default it only scans files once at startup but can configured to poll regularly.

### `KubeletConfigProvider`

The `KubeletConfigProvider` relies on the Kubelet API to detect check configs defined on pod annotations.

### `ContainerConfigProvider`

The `ContainerConfigProvider` detects check configs defined on container labels.

### `ECSConfigProvider`

The `ECSConfigProvider` relies on the ECS API to detect check configs defined on container labels. This config provider is enabled on ECS Fargate only, on ECS EC2 we use the docker config provider.

### `KubeServiceConfigProvider`

The `KubeServiceConfigProvider` relies on the Kubernetes API server to detect the cluster check configs defined on service annotations. The Datadog Cluster Agent runs this `ConfigProvider`.

### `ClusterChecksConfigProvider`

The `ClusterChecksConfigProvider` queries the Datadog Cluster Agent API to consume the exposed cluster check configs. The node Agent or the cluster check runner can run this config provider.

### `KubeEndpointsConfigProvider`

The `KubeEndpointsConfigProvider` relies on the Kubernetes API server to detect the endpoints check configs defined on service annotations. The Datadog Cluster Agent runs this `ConfigProvider`.

### `EndpointChecksConfigProvider`

The `EndpointChecksConfigProvider` queries the Datadog Cluster Agent API to consume the exposed endpoints check configs.

### `PrometheusPodsConfigProvider`

The `PrometheusPodsConfigProvider` relies on the Kubelet API to detect Prometheus pod annotations and generate a corresponding `Openmetrics` config.

### `PrometheusServicesConfigProvider`

The `PrometheusServicesConfigProvider` relies on the Kubernetes API server to watch Prometheus service annotations and generate a corresponding `Openmetrics` config. The Datadog Cluster Agent runs this `ConfigProvider`.

### `CloudFoundryConfigProvider`

The `CloudFoundryConfigProvider` relies on the CloudFoundry BBS API to detect check configs defined in LRP environment variables.

### `ConsulConfigProvider`

The `ConsulConfigProvider` reads the check configs from consul.

### `ETCDConfigProvider`

The `ETCDConfigProvider` reads the check configs from etcd.

### `ZookeeperConfigProvider`

The `ZookeeperConfigProvider` reads the check configs from zookeeper.

### `RemoteConfigProvider`

The `RemoteConfigProvider` reads the check configs from remote-config.
