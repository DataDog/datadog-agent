// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package catalog

import "fmt"

// The manifests below are static ports of the framework's Pulumi app
// definitions (components/datadog/apps/*). Names, images, labels,
// annotations and env vars are exactly what the containers k8sSuite
// asserts against; comments name the source of each port. The OpenShift
// SCC bindings are dropped — they only matter on OpenShift hosts.

// nginxManifests ports components/datadog/apps/nginx/k8s.go: namespace
// labels/annotations, nginx.conf ConfigMap, Deployment with AD annotation,
// PDB, DatadogMetric, HPA, VPA, Service with AD annotation, and the
// nginx-query client driving requests through the service.
func nginxManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
  labels:
    related_team: contp
    related_org: agent-org
  annotations:
    related_email: team-container-platform@datadoghq.com
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx
  namespace: %[1]s
  labels:
    app: nginx
data:
  nginx.conf: |
    worker_processes  auto;
    events {
        worker_connections  4096;
    }
    http {
        server {
            listen [::]:80 ipv6only=off reuseport fastopen=32 default_server;

            location /nginx_status {
              stub_status on;
              access_log  /dev/stdout;
              allow all;
            }
        }
    }
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: nginx-sa
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: %[1]s
  labels:
    app: nginx
    x-team: contp
  annotations:
    x-sub-team: contint
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
        x-parent-type: deployment
      annotations:
        x-parent-name: nginx
        ad.datadoghq.com/nginx.checks: |
          {
            "nginx": {
              "init_config": {},
              "check_tag_cardinality": "high",
              "instances": [
                {"nginx_status_url": "http://%%%%host%%%%:80/nginx_status"}
              ]
            }
          }
    spec:
      serviceAccountName: nginx-sa
      containers:
        - name: nginx
          image: ghcr.io/datadog/apps-nginx-server:%[2]s
          # Deviation from apps/nginx: worker_processes auto spawns one
          # worker per core, and local machines have far more cores than
          # the EC2 instance types the 32Mi limit was sized for.
          resources:
            limits:
              cpu: 100m
              memory: 512Mi
            requests:
              cpu: 10m
              memory: 32Mi
          ports:
            - name: http
              containerPort: 80
              protocol: TCP
          livenessProbe:
            httpGet:
              port: 80
            timeoutSeconds: 5
          readinessProbe:
            httpGet:
              port: 80
            timeoutSeconds: 5
          volumeMounts:
            - name: cache
              mountPath: /var/cache/nginx
            - name: var-run
              mountPath: /var/run
            - name: conf
              mountPath: /etc/nginx/nginx.conf
              subPath: nginx.conf
      volumes:
        - name: cache
          emptyDir: {}
        - name: var-run
          emptyDir: {}
        - name: conf
          configMap:
            name: nginx
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: nginx
  namespace: %[1]s
  labels:
    app: nginx
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      app: nginx
---
apiVersion: datadoghq.com/v1alpha1
kind: DatadogMetric
metadata:
  name: nginx
  namespace: %[1]s
  labels:
    app: nginx
spec:
  query: avg:nginx.net.request_per_s{kube_cluster_name:%%%%tag_kube_cluster_name%%%%,kube_namespace:%[1]s,kube_deployment:nginx}.rollup(60)
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: nginx
  namespace: %[1]s
  labels:
    app: nginx
spec:
  minReplicas: 1
  maxReplicas: 5
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: nginx
  metrics:
    - type: External
      external:
        metric:
          name: datadogmetric@%[1]s:nginx
        target:
          type: Value
          value: "10"
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 0
---
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: nginx
  namespace: %[1]s
  labels:
    app: nginx
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: nginx
  updatePolicy:
    updateMode: Auto
---
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: %[1]s
  labels:
    app: nginx
  annotations:
    ad.datadoghq.com/service.checks: |
      {
        "http_check": {
          "init_config": {},
          "instances": [
            {"name": "My Nginx", "url": "http://%%%%host%%%%", "timeout": 1}
          ]
        }
      }
spec:
  selector:
    app: nginx
  ports:
    - port: 80
      targetPort: 80
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-query
  namespace: %[1]s
  labels:
    app: nginx-query
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx-query
  template:
    metadata:
      labels:
        app: nginx-query
    spec:
      containers:
        - name: query
          image: ghcr.io/datadog/apps-http-client:%[2]s
          args: ["-url", "http://nginx", "-min-tps", "1", "-max-tps", "60", "-period", "20m"]
          resources:
            limits:
              cpu: 100m
              memory: 512Mi
            requests:
              cpu: 10m
              memory: 32Mi
`, ns, Version)
	return []string{manifest}
}

// redisManifests ports components/datadog/apps/redis/k8s.go.
func redisManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: %[1]s
  labels:
    app: redis
    team: container-integrations # Test auto_team_tag_collection feature (default enabled)
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
        - name: redis
          image: ghcr.io/datadog/redis:%[2]s
          args: ["--loglevel", "verbose"]
          resources:
            limits:
              cpu: 100m
              memory: 32Mi
            requests:
              cpu: 10m
              memory: 32Mi
          ports:
            - name: redis
              containerPort: 6379
              protocol: TCP
          livenessProbe:
            tcpSocket:
              port: 6379
          readinessProbe:
            tcpSocket:
              port: 6379
          volumeMounts:
            - name: redis-data
              mountPath: /data
      volumes:
        - name: redis-data
          emptyDir: {}
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: redis
  namespace: %[1]s
  labels:
    app: redis
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      app: redis
---
apiVersion: datadoghq.com/v1alpha1
kind: DatadogMetric
metadata:
  name: redis
  namespace: %[1]s
  labels:
    app: redis
spec:
  query: avg:redis.net.instantaneous_ops_per_sec{kube_cluster_name:%%%%tag_kube_cluster_name%%%%,kube_namespace:%[1]s,kube_deployment:redis}.rollup(60)
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: redis
  namespace: %[1]s
  labels:
    app: redis
spec:
  minReplicas: 1
  maxReplicas: 5
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: redis
  metrics:
    - type: External
      external:
        metric:
          name: datadogmetric@%[1]s:redis
        target:
          type: AverageValue
          averageValue: "10"
---
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: redis
  namespace: %[1]s
  labels:
    app: redis
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: redis
  updatePolicy:
    updateMode: Auto
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: %[1]s
  labels:
    app: redis
  annotations:
    # endpoints-level AD (cluster-checks endpoints provider): the http_check
    # cannot connect to a redis port — the suite asserts the failure metric.
    ad.datadoghq.com/endpoints.checks: |
      {
        "http_check": {
          "init_config": {},
          "instances": [
            {"name": "My Redis", "url": "http://%%%%host%%%%:%%%%port%%%%", "timeout": 1}
          ]
        }
      }
spec:
  selector:
    app: redis
  ports:
    - name: redis
      port: 6379
      targetPort: redis
`, ns, Version)
	return []string{manifest}
}

// cpustressManifests ports components/datadog/apps/cpustress/k8s.go.
func cpustressManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stress-ng
  namespace: %[1]s
  labels:
    app: stress-ng
spec:
  replicas: 1
  selector:
    matchLabels:
      app: stress-ng
  template:
    metadata:
      labels:
        app: stress-ng
    spec:
      containers:
        - name: stress-ng
          image: ghcr.io/datadog/apps-stress-ng:%[2]s
          args: ["--cpu=1", "--cpu-load=15", "--temp-path=/tmp/"]
          workingDir: /tmp
          resources:
            limits:
              cpu: 200m
              memory: 64Mi
            requests:
              cpu: 200m
              memory: 64Mi
          volumeMounts:
            - name: temp-dir
              mountPath: /tmp
      volumes:
        - name: temp-dir
          emptyDir: {}
`, ns, Version)
	return []string{manifest}
}

// dogstatsdManifests ports components/datadog/apps/dogstatsd/k8s.go with
// the in-agent endpoint parameters (port 8125, socket /var/run/datadog/dsd.socket).
func dogstatsdManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dogstatsd-sa
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogstatsd-uds-with-csi
  namespace: %[1]s
  labels:
    app: dogstatsd-uds-with-csi
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogstatsd-uds-with-csi
  template:
    metadata:
      labels:
        admission.datadoghq.com/config.mode: csi
        app: dogstatsd-uds-with-csi
        admission.datadoghq.com/enabled: "true"
    spec:
      serviceAccountName: dogstatsd-sa
      containers:
        - name: dogstatsd
          image: ghcr.io/datadog/apps-dogstatsd:%[2]s
          resources:
            limits:
              cpu: 100m
              memory: 32Mi
            requests:
              cpu: 10m
              memory: 32Mi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogstatsd-uds
  namespace: %[1]s
  labels:
    app: dogstatsd-uds
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogstatsd-uds
  template:
    metadata:
      labels:
        app: dogstatsd-uds
    spec:
      serviceAccountName: dogstatsd-sa
      containers:
        - name: dogstatsd
          image: ghcr.io/datadog/apps-dogstatsd:%[2]s
          env:
            - name: STATSD_URL
              value: unix:///var/dsd.socket
          resources:
            limits:
              cpu: 100m
              memory: 32Mi
            requests:
              cpu: 10m
              memory: 32Mi
          volumeMounts:
            - name: dogstatsd-socket
              mountPath: /var/dsd.socket
      volumes:
        - name: dogstatsd-socket
          hostPath:
            path: /var/run/datadog/dsd.socket
            type: Socket
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogstatsd-udp
  namespace: %[1]s
  labels:
    app: dogstatsd-udp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogstatsd-udp
  template:
    metadata:
      labels:
        app: dogstatsd-udp
    spec:
      serviceAccountName: dogstatsd-sa
      containers:
        - name: dogstatsd
          image: ghcr.io/datadog/apps-dogstatsd:%[2]s
          env:
            - name: HOST_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.hostIP
            - name: STATSD_URL
              value: $(HOST_IP):8125
            - name: DD_ENTITY_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.uid
          resources:
            limits:
              cpu: 10m
              memory: 32Mi
            requests:
              cpu: 2m
              memory: 32Mi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogstatsd-udp-origin-detection
  namespace: %[1]s
  labels:
    app: dogstatsd-udp-origin-detection
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogstatsd-udp-origin-detection
  template:
    metadata:
      labels:
        app: dogstatsd-udp-origin-detection
    spec:
      serviceAccountName: dogstatsd-sa
      containers:
        - name: dogstatsd
          image: ghcr.io/datadog/apps-dogstatsd:%[2]s
          env:
            - name: HOST_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.hostIP
            - name: STATSD_URL
              value: $(HOST_IP):8125
          resources:
            limits:
              cpu: 10m
              memory: 32Mi
            requests:
              cpu: 2m
              memory: 32Mi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogstatsd-udp-contname-injected
  namespace: %[1]s
  labels:
    app: dogstatsd-udp-contname-injected
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogstatsd-udp-contname-injected
  template:
    metadata:
      labels:
        app: dogstatsd-udp-contname-injected
    spec:
      serviceAccountName: dogstatsd-sa
      containers:
        - name: dogstatsd
          image: ghcr.io/datadog/apps-dogstatsd:%[2]s
          env:
            - name: HOST_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.hostIP
            - name: STATSD_URL
              value: $(HOST_IP):8125
            - name: DD_INTERNAL_POD_UID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.uid
            - name: DD_ENTITY_ID
              value: en-$(DD_INTERNAL_POD_UID)/dogstatsd
          resources:
            limits:
              cpu: 10m
              memory: 32Mi
            requests:
              cpu: 2m
              memory: 32Mi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogstatsd-udp-external-data-only
  namespace: %[1]s
  labels:
    app: dogstatsd-udp-external-data-only
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogstatsd-udp-external-data-only
  template:
    metadata:
      labels:
        app: dogstatsd-udp-external-data-only
        admission.datadoghq.com/enabled: "true"
    spec:
      serviceAccountName: dogstatsd-sa
      containers:
        - name: dogstatsd
          image: ghcr.io/datadog/apps-dogstatsd:%[2]s
          env:
            - name: HOST_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.hostIP
            - name: STATSD_URL
              value: $(HOST_IP):8125
            - name: DD_ENTITY_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.uid
            - name: DD_EXTERNAL_DATA_ONLY
              value: "true"
          resources:
            limits:
              cpu: 10m
              memory: 32Mi
            requests:
              cpu: 2m
              memory: 32Mi
`, ns, Version)
	return []string{manifest}
}

// dogstatsdStandaloneManifests ports
// components/datadog/dogstatsd-standalone/k8s.go: the DaemonSet listening
// on host port 8128 with the standalone socket, its RBAC and priority class.
func dogstatsdStandaloneManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dogstatsd-standalone
  namespace: %[1]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dogstatsd-standalone
rules:
  - nonResourceURLs: ["/metrics"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["nodes/metrics", "nodes/spec", "nodes/proxy", "nodes/stats"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dogstatsd-standalone
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: dogstatsd-standalone
subjects:
  - kind: ServiceAccount
    name: dogstatsd-standalone
    namespace: %[1]s
---
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: dogstatsd-standalone
value: 1000000000
preemptionPolicy: PreemptLowerPriority
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: dogstatsd-standalone
  namespace: %[1]s
spec:
  selector:
    matchLabels:
      app: dogstatsd-standalone
  template:
    metadata:
      labels:
        app: dogstatsd-standalone
    spec:
      hostPID: true
      serviceAccountName: dogstatsd-standalone
      priorityClassName: dogstatsd-standalone
      containers:
        - name: dogstatsd-standalone
          image: registry.datadoghq.com/dogstatsd:latest
          ports:
            - containerPort: 8125
              hostPort: 8128
              name: dogstatsdport
              protocol: UDP
          env:
            - name: DD_API_KEY
              value: "{{API_KEY}}"
            - name: DD_ENABLE_METADATA_COLLECTION
              value: "false"
            - name: DD_KUBERNETES_KUBELET_HOST
              valueFrom:
                fieldRef:
                  fieldPath: status.hostIP
            - name: DD_DOGSTATSD_NON_LOCAL_TRAFFIC
              value: "true"
            - name: DD_DOGSTATSD_ORIGIN_DETECTION
              value: "true"
            - name: DD_DOGSTATSD_SOCKET
              value: /run/datadog/dsd-standalone.socket
            - name: DD_DOGSTATSD_TAG_CARDINALITY
              value: high
            - name: DD_KUBELET_TLS_VERIFY
              value: "false"
            - name: DD_CRI_SOCKET_PATH
              value: /run/containerd/containerd.sock
            - name: DD_ADDITIONAL_ENDPOINTS
              value: '{"{{FAKEINTAKE_URL}}": ["FAKEAPIKEY"]}'
            - name: DD_CLUSTER_NAME
              value: "{{CLUSTER_NAME}}"
          resources:
            limits:
              cpu: 100m
              memory: 512Mi
            requests:
              cpu: 100m
              memory: 512Mi
          volumeMounts:
            - name: hostvar
              mountPath: /host/var
              readOnly: true
            - name: hostrun
              mountPath: /host/run
              readOnly: true
            - name: logdir
              mountPath: /var/log/datadog
            - name: procdir
              mountPath: /host/proc
              readOnly: true
            - name: cgroups
              mountPath: /host/sys/fs/cgroup
              readOnly: true
            - name: datadog
              mountPath: /run/datadog
            - name: crisocket
              mountPath: /run/containerd/containerd.sock
              readOnly: true
      volumes:
        - name: hostvar
          hostPath:
            path: /var
        - name: hostrun
          hostPath:
            path: /run
        - name: logdir
          emptyDir: {}
        - name: procdir
          hostPath:
            path: /proc
        - name: cgroups
          hostPath:
            path: /sys/fs/cgroup
        - name: datadog
          hostPath:
            path: /run/datadog
        - name: crisocket
          hostPath:
            path: /run/containerd/containerd.sock
            type: Socket
`, ns)
	return []string{manifest}
}

// dogstatsdStandaloneClientsManifests ports apps/dogstatsd/k8s.go with the
// standalone endpoint parameters (host port 8128, standalone socket).
func dogstatsdStandaloneClientsManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dogstatsd-sa
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogstatsd-uds
  namespace: %[1]s
  labels:
    app: dogstatsd-uds
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogstatsd-uds
  template:
    metadata:
      labels:
        app: dogstatsd-uds
    spec:
      serviceAccountName: dogstatsd-sa
      containers:
        - name: dogstatsd
          image: ghcr.io/datadog/apps-dogstatsd:%[2]s
          env:
            - name: STATSD_URL
              value: unix:///var/dsd.socket
          resources:
            limits:
              cpu: 100m
              memory: 32Mi
            requests:
              cpu: 10m
              memory: 32Mi
          volumeMounts:
            - name: dogstatsd-socket
              mountPath: /var/dsd.socket
      volumes:
        - name: dogstatsd-socket
          hostPath:
            path: /run/datadog/dsd-standalone.socket
            type: Socket
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogstatsd-udp
  namespace: %[1]s
  labels:
    app: dogstatsd-udp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogstatsd-udp
  template:
    metadata:
      labels:
        app: dogstatsd-udp
    spec:
      serviceAccountName: dogstatsd-sa
      containers:
        - name: dogstatsd
          image: ghcr.io/datadog/apps-dogstatsd:%[2]s
          env:
            - name: HOST_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.hostIP
            - name: STATSD_URL
              value: $(HOST_IP):8128
            - name: DD_ENTITY_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.uid
          resources:
            limits:
              cpu: 10m
              memory: 32Mi
            requests:
              cpu: 2m
              memory: 32Mi
`, ns, Version)
	return []string{manifest}
}

// tracegenManifests ports components/datadog/apps/tracegen/k8s.go.
func tracegenManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: tracegen-uds-sa
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tracegen-tcp
  namespace: %[1]s
  labels:
    app: tracegen-tcp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tracegen-tcp
  template:
    metadata:
      labels:
        app: tracegen-tcp
    spec:
      containers:
        - name: tracegen-tcp
          image: ghcr.io/datadog/apps-tracegen:%[2]s
          env:
            - name: DD_AGENT_HOST
              valueFrom:
                fieldRef:
                  fieldPath: status.hostIP
          resources:
            limits:
              cpu: 10m
              memory: 32Mi
            requests:
              cpu: 2m
              memory: 32Mi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tracegen-uds
  namespace: %[1]s
  labels:
    app: tracegen-uds
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tracegen-uds
  template:
    metadata:
      labels:
        app: tracegen-uds
    spec:
      serviceAccountName: tracegen-uds-sa
      containers:
        - name: tracegen-uds
          image: ghcr.io/datadog/apps-tracegen:%[2]s
          env:
            - name: DD_TRACE_AGENT_URL
              value: unix:///var/run/datadog/apm.socket
          resources:
            limits:
              cpu: 10m
              memory: 32Mi
            requests:
              cpu: 2m
              memory: 32Mi
          volumeMounts:
            - name: apmsocketpath
              mountPath: /var/run/datadog
      volumes:
        - name: apmsocketpath
          hostPath:
            path: /var/run/datadog
`, ns, Version)
	return []string{manifest}
}

// prometheusManifests ports components/datadog/apps/prometheus/k8s.go.
func prometheusManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: %[1]s
  labels:
    app: prometheus
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
      annotations:
        prometheus.io/scrape: "true"
    spec:
      containers:
        - name: prometheus
          image: ghcr.io/datadog/apps-prometheus:%[2]s
          resources:
            limits:
              cpu: 100m
              memory: 32Mi
            requests:
              cpu: 10m
              memory: 32Mi
          ports:
            - name: metrics
              containerPort: 8080
              protocol: TCP
          livenessProbe:
            httpGet:
              port: 8080
              path: /metrics
          readinessProbe:
            httpGet:
              port: 8080
              path: /metrics
`, ns, Version)
	return []string{manifest}
}

// etcdManifests ports components/datadog/apps/etcd/k8s.go: the v2 etcd
// server with the sidecar that publishes an openmetrics check config in
// etcd keys (the agent's etcd AD provider consumes it), plus the service.
func etcdManifests(ns string) []string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: etcd-sa
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: etcd
  namespace: %[1]s
  labels:
    app: etcd
spec:
  replicas: 1
  selector:
    matchLabels:
      app: etcd
  template:
    metadata:
      labels:
        app: etcd
    spec:
      serviceAccountName: etcd-sa
      containers:
        - name: etcd
          image: quay.io/coreos/etcd:v3.5.1
          command: ["etcd"]
          args:
            - --enable-v2
            - --name=etcd-0
            - --data-dir=/var/lib/etcd
            - --listen-client-urls=http://0.0.0.0:2379
            - --advertise-client-urls=http://etcd:2379
            - --listen-peer-urls=http://0.0.0.0:2380
            - --initial-advertise-peer-urls=http://etcd:2380
            - --initial-cluster=etcd-0=http://etcd:2380
            - --initial-cluster-token=etcd-cluster-1
            - --initial-cluster-state=new
          ports:
            - name: etcd
              containerPort: 2379
              protocol: TCP
          readinessProbe:
            httpGet:
              path: /health
              port: 2379
              scheme: HTTP
            initialDelaySeconds: 10
            timeoutSeconds: 5
          livenessProbe:
            httpGet:
              path: /health
              port: 2379
              scheme: HTTP
            initialDelaySeconds: 10
            timeoutSeconds: 5
          volumeMounts:
            - name: etcd-data
              mountPath: /var/lib/etcd
        - name: etcd-config
          image: ghcr.io/datadog/apps-alpine:%[2]s
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -e

              echo "[init] Waiting for etcd to respond on TCP port 2379..."
              until nc -z localhost 2379; do
                sleep 1
              done

              echo "[init] Waiting for etcd v2 API to be ready..."
              until curl -sf http://localhost:2379/v2/keys/; do
                echo "[init] etcd not ready yet..."
                sleep 1
              done

              echo "[init] Setting check configuration keys in etcd v2..."

              curl -sf -XPUT http://localhost:2379/v2/keys/datadog/check_configs/apps-prometheus/check_names \
                --data-urlencode 'value=["openmetrics"]'

              curl -sf -XPUT http://localhost:2379/v2/keys/datadog/check_configs/apps-prometheus/init_configs \
                --data-urlencode 'value=[{}]'

              curl -sf -XPUT http://localhost:2379/v2/keys/datadog/check_configs/apps-prometheus/instances \
                --data-urlencode 'value=[{"openmetrics_endpoint": "http://%%%%host%%%%:8080/metrics", "metrics":[{"prom_gauge": "prom_gauge_configured_in_etcd"}]}]'

              echo "[init] Done setting check configuration keys in etcd"
              sleep infinity
      volumes:
        - name: etcd-data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: etcd
  namespace: %[1]s
  labels:
    app: etcd
spec:
  selector:
    app: etcd
  ports:
    - name: client
      port: 2379
      targetPort: 2379
      protocol: TCP
`, ns, Version)
	return []string{manifest}
}

// mutatedManifests ports components/datadog/apps/mutatedbyadmissioncontroller/k8s.go:
// one namespace without lib injection, one with it, and the three
// deployments the admission-controller tests assert against.
func mutatedManifests(ns string) []string {
	nsLib := ns + "-lib-injection"
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: v1
kind: Namespace
metadata:
  name: %[2]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mutated-sa
  namespace: %[1]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mutated-with-lib-sa
  namespace: %[2]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mutated
  namespace: %[1]s
  labels:
    app: mutated
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mutated
  template:
    metadata:
      labels:
        app: mutated
        admission.datadoghq.com/enabled: "true"
        tags.datadoghq.com/env: e2e
        tags.datadoghq.com/service: mutated
        tags.datadoghq.com/version: v0.0.1
    spec:
      serviceAccountName: mutated-sa
      containers:
        - name: mutated
          image: ghcr.io/datadog/apps-mutated:%[3]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mutated-with-lib-annotation
  namespace: %[2]s
  labels:
    app: mutated-with-lib-annotation
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mutated-with-lib-annotation
  template:
    metadata:
      labels:
        app: mutated-with-lib-annotation
        admission.datadoghq.com/enabled: "true"
        tags.datadoghq.com/env: e2e
        tags.datadoghq.com/service: mutated-with-lib-annotation
        tags.datadoghq.com/version: v0.0.1
      annotations:
        admission.datadoghq.com/python-lib.version: v2.7.3
    spec:
      serviceAccountName: mutated-with-lib-sa
      containers:
        - name: mutated-with-lib-annotation
          image: python:3.12-slim
          command: ["python", "-c", "while True: import time; time.sleep(60)"]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mutated-with-auto-detected-language
  namespace: %[2]s
  labels:
    app: mutated-with-auto-detected-language
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mutated-with-auto-detected-language
  template:
    metadata:
      labels:
        app: mutated-with-auto-detected-language
        admission.datadoghq.com/enabled: "true"
        tags.datadoghq.com/env: e2e
        tags.datadoghq.com/service: mutated-with-auto-detected-language
        tags.datadoghq.com/version: v0.0.1
    spec:
      serviceAccountName: mutated-with-lib-sa
      containers:
        - name: mutated-with-auto-detected-language
          image: python:3.12-slim
          command: ["python", "-c", "while True: import time; time.sleep(60)"]
`, ns, nsLib, Version)
	return []string{manifest}
}

// vpaCrdManifests ports components/kubernetes/vpa/vpa.go DeployCRD: the
// minimal VerticalPodAutoscaler CRD (cluster-scoped; namespace ignored).
func vpaCrdManifests(_ string) []string {
	manifest := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: verticalpodautoscalers.autoscaling.k8s.io
  annotations:
    api-approved.kubernetes.io: https://github.com/kubernetes/kubernetes/pull/63797
spec:
  group: autoscaling.k8s.io
  scope: Namespaced
  names:
    kind: VerticalPodAutoscaler
    listKind: VerticalPodAutoscalerList
    singular: verticalpodautoscaler
    plural: verticalpodautoscalers
    shortNames:
      - vpa
  versions:
    - name: v1beta2
      served: true
      storage: false
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                targetRef:
                  type: object
                  properties:
                    apiVersion: {type: string}
                    kind: {type: string}
                    name: {type: string}
                  required: [apiVersion, kind, name]
                updatePolicy:
                  type: object
                  properties:
                    updateMode: {type: string}
                resourcePolicy:
                  type: object
              required: [targetRef]
            status:
              type: object
    - name: v1
      served: true
      storage: true
      additionalPrinterColumns:
        - name: Target
          type: string
          jsonPath: .spec.targetRef.name
          description: Name of the target resource
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                targetRef:
                  type: object
                  properties:
                    apiVersion: {type: string}
                    kind: {type: string}
                    name: {type: string}
                  required: [apiVersion, kind, name]
                updatePolicy:
                  type: object
                  properties:
                    updateMode: {type: string}
                resourcePolicy:
                  type: object
              required: [targetRef]
            status:
              type: object
`
	return []string{manifest}
}
