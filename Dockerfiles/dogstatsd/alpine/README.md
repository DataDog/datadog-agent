# DogStatsD6 docker image

This docker image runs dogstatsd as a standalone container. It can be used in cases a full-fledged agent6 is not needed on the node, or as a sidecar container inside pods. It supports both the UDP protocol (default) or Unix Domain Socket (if `DD_DOGSTATSD_SOCKET` is set to a valid path). To know more about each protocol, see the [dogstatsd readme](../../../cmd/dogstatsd/README.md).

## How to run it

The following environment variables are supported:

  - `DD_API_KEY`: your API key (**required**)
  - `DD_HOSTNAME`: hostname to use for metrics
  - `DD_DOGSTATSD_SOCKET`: path to the unix socket to use instead of UDP. Must be in a `rw` mounted volume.
  - `DD_ENABLE_METADATA_COLLECTION`: whether to collect metadata (default is true, set to false only if running alonside an existing dd-agent)
  - `DD_SITE`: Destination site for your metrics, should be set to the site you are using. Defaults to datadoghq.com.

This is a sample Kubernetes DaemonSet, using the UDS protocol, running alongside an existing agent5:

```
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: dogstatsd
spec:
  selector:
    matchLabels:
      app: dogstatsd
  template:
    metadata:
      labels:
        app: dogstatsd
      name: dogstatsd
    spec:
      containers:
      - image: datadog/dogstatsd:latest
        imagePullPolicy: Always
        name: dogstatsd
        env:
          - name: DD_API_KEY
            value: ___value___
          - name: DD_DOGSTATSD_SOCKET
            value: "/socket/statsd.socket"
          - name: DD_SEND_HOST_METADATA
            # Legacy option name, keep as `false` when running alongside another Agent 
            value: "false"
          - name: DD_ENABLE_METADATA_COLLECTION
            value: "false"
          - name: DD_HOSTNAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
        volumeMounts:
          - name: dsdsocket
            mountPath: /socket
      volumes:
        - hostPath:
            path: /var/run/dogstatsd
          name: dsdsocket
```

If you want to use the UDP protocol on port 8125, running alongside an existing agent5:

```
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: dogstatsd
spec:
  selector:
    matchLabels:
      app: dogstatsd
  template:
    metadata:
      labels:
        app: dogstatsd
      name: dogstatsd
    spec:
      containers:
      - image: datadog/dogstatsd:latest
        imagePullPolicy: Always
        name: dogstatsd
        ports:
          - containerPort: 8125
            name: dogstatsdport
            protocol: UDP
        env:
          - name: DD_API_KEY
            value: ___value___
          - name: DD_SEND_HOST_METADATA
            # Legacy option name, keep as `false` when running alongside another Agent 
            value: "false"
          - name: DD_ENABLE_METADATA_COLLECTION
            value: "false"
          - name: DD_HOSTNAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
```

## How to build it

You can use `inv dogstatsd.image-build` to build your own dogstatsd image
