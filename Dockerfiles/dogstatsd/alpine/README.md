# DogStatsD6 docker image

This docker image runs dogstatsd as a standalone container. It can be used in cases a full-fledged agent6 is not needed on the node, or as a sidecar container inside pods. It supports both the UDP protocol (default) or Unix Domain Socket (if `DD_DOGSTATSD_SOCKET` is set to a valid path). To know more about each protocol, see the [dogstatsd readme](../../../cmd/dogstatsd/README.md).

The following environment variables are supported:

  - `DD_API_KEY`: your API key (**required**)
  - `DD_HOSTNAME`: hostname to use for metrics
  - `DD_DOGSTATSD_SOCKET`: path to the unix socket to use instead of UDP. Must be in a `rw` mounted volume.
  - `DD_ENABLE_METADATA_COLLECTION`: whether to collect metadata (default is true, set to false only if running alonside an existing dd-agent)

This is a sample Kubernetes DaemonSet, using the UDS protocol, running alongside an existing agent5:

```
apiVersion: extensions/v1beta1
kind: DaemonSet
metadata:
  name: dogstatsd
spec:
  template:
    metadata:
      labels:
        app: dogstatsd
      name: dogstatsd
    spec:
      containers:
      - image: datadog/dogstatsd:beta
        imagePullPolicy: Always
        name: dogstatsd
        env:
          - name: DD_API_KEY
            value: ___value___
          - name: DD_DOGSTATSD_SOCKET
            value: "/socket/statsd.socket"
          - name: DD_ENABLE_METADATA_COLLECTION
            value: false
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
