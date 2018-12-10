#!/bin/bash

printf '=%.0s' {0..79} ; echo
set -x

# ${DATADOG_AGENT_IMAGE} is provided by the CI
test ${DATADOG_AGENT_IMAGE} || {
    echo "DATADOG_AGENT_IMAGE envvar needs to be set" >&2
    exit 2
}

echo "DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"

cd "$(dirname $0)"

# TODO run all workflows ?

AGENT_DAEMONSET=$(cat << EOF
---
apiVersion: extensions/v1beta1
kind: DaemonSet
metadata:
  name: datadog-agent
  namespace: default
spec:
  updateStrategy:
    rollingUpdate:
      maxUnavailable: 1
  template:
    metadata:
      labels:
        app: datadog-agent
      name: datadog-agent
    spec:
      serviceAccount: datadog-agent
      containers:
      - name: agent
        image: ${DATADOG_AGENT_IMAGE}
        command:
        - /opt/datadog-agent/bin/agent/agent
        - start
        env:
        - name: DD_KUBERNETES_KUBELET_HOST
          valueFrom:
            fieldRef:
              fieldPath: status.hostIP
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "250m"
        livenessProbe:
          exec:
            command:
            - /opt/datadog-agent/bin/agent/agent
            - health
          initialDelaySeconds: 30
          periodSeconds: 5
        readinessProbe:
          exec:
            command:
            - /opt/datadog-agent/bin/agent/agent
            - health
          failureThreshold: 5
          initialDelaySeconds: 20
        volumeMounts:
        - name: datadog-config
          mountPath: /etc/datadog-agent/datadog.yaml
          subPath: datadog.yaml
        - name: datadog-config
          mountPath: /etc/datadog-agent/conf.d/kubernetes_apiserver.d/conf.yaml
          subPath: apiserver.yaml
        - name: datadog-config
          mountPath: /etc/datadog-agent/conf.d/kubelet.d/conf.yaml
          subPath: kubelet.yaml
        - name: datadog-config
          mountPath: /etc/datadog-agent/conf.d/network.d/conf.yaml.default
          subPath: network.yaml
        - name: datadog-config
          mountPath: /etc/datadog-agent/conf.d/docker.d/conf.yaml.default
          subPath: docker.yaml
        - name: datadog-config
          mountPath: /etc/datadog-agent/conf.d/memory.d/conf.yaml.default
          subPath: memory.yaml
        - name: proc
          mountPath: /host/proc
          readOnly: true
        - name: cgroup
          mountPath: /host/sys/fs/cgroup
          readOnly: true
        - name: dockersocket
          mountPath: /var/run/docker.sock
          readOnly: true
        - name: dogstatsd
          mountPath: /var/run/dogstatsd
          readOnly: false

      volumes:
      - name: datadog-config
        configMap:
          name: datadog
      - name: proc
        hostPath:
          path: /proc
      - name: cgroup
        hostPath:
          path: /sys/fs/cgroup
      - hostPath:
          path: /var/run/docker.sock
        name: dockersocket
      - hostPath:
          path: /var/run/dogstatsd
        name: dogstatsd
---
EOF
)

./argo submit ../../argo-workflows/agent.yaml -w --parameter agent-daemonset="${AGENT_DAEMONSET}"
# we are waiting for the end of the workflow but we don't care about its return code
exit 0
