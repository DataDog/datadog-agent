# UDP -> Unix Domain Socket proxy for DogStatsD

This docker image runs a simple UDP -> UDS proxy with `socat`, to help with the transition to the Unix Domain Socket protocol if your client libraries are not patched yet. It is intended to be run as a sidecar container for your application and will forward UDP packets sent to `localhost:8125` to `/socket/statsd.socket`.

To run it, you need to:

  - expose the dogstatsd socket path to `/socket/statsd.socket` (for example `-v /var/run/datadog/:/socket:ro`)
  - run the container with `--autorestart ALWAYS`, as `socat` will exit on write errors. Please note that packets will be dropped while the container is restarting.

This is an example Kubernetes spec:

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-producer
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-producer
  template:
    metadata:
      labels:
        app: test-producer
    spec:
      containers:
        - **[insert producer container specs]**
        - name: socat
          image: datadog/dogstatsd-socat-proxy:beta
          ports:
            - containerPort: 8125
              name: dogstatsdport
              protocol: UDP
          volumeMounts:
            - name: dsdsocket
              mountPath: /socket
      volumes:
        - hostPath:
            path: /var/run/dogstatsd
          name: dsdsocket
```
