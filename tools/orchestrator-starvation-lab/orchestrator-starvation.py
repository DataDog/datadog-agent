#!/usr/bin/env -S uv run --quiet
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""orchestrator-starvation.py

A self-contained kind+fakeintake lab that reproduces orchestrator-check
starvation under Prometheus-HTTP-SD-generated cluster-check load.

See README.md and the task spec at
  ~/tasks/04003-p2-ready--repro-orchestrator-starvation-fakeintake.md
"""
from __future__ import annotations

import argparse
import contextlib
import csv
import json
import os
import pathlib
import re
import shlex
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from dataclasses import dataclass

# ----------------------------------------------------------------------------
# Defaults — overridable via flags
# ----------------------------------------------------------------------------
CLUSTER_NAME = "orch-starve"
KIND_NODE_IMAGE = "kindest/node:v1.32.2"
OPERATOR_APP_VERSION_DEFAULT = "1.24.0"  # matches the production-scale workload modelled here
# Chart version -> operator app version.  Keep this list in sync with
# `helm search repo datadog/datadog-operator --versions`.
OPERATOR_CHART_BY_APP = {
    "1.24.0": "2.20.0",
    "1.25.0": "2.21.0",
    "1.26.0": "2.22.2",
}
AGENT_IMAGE_DEFAULT = "registry.datadoghq.com/agent-dev:nightly-full-main-jmx"
CLUSTER_AGENT_IMAGE_DEFAULT = "registry.datadoghq.com/cluster-agent-dev:master"
FAKEINTAKE_IMAGE_DEFAULT = "datadog/fakeintake:latest"
NAMESPACE = "datadog"
NOISE_NAMESPACE = "noise"
WORKLOAD_NAMESPACE = "workload"
FAKEINTAKE_LOCAL_PORT = 18080

# Endpoint where the orchestrator-check manifest payloads land.  Modern
# agents (v7.x and the nightly-full-main dev tags) POST orchestrator data
# to /api/v2/orch (collector summaries) and /api/v2/orchmanif (per-resource
# manifests).  The task spec references the older /api/v1/orchestrator;
# that route does not receive traffic from current agents.
#
# Manifest payloads are zstd-compressed protobuf (agent-payload v5) wrapped
# in a 16-byte framing header.  fakeintake's built-in JSON decode aggregator
# only knows the old /api/v1/orchestrator route, so format=json returns null
# for v2 endpoints.  We work around this by counting raw payloads (and
# distinct 15-second batches, which approximate orchestrator-check runs)
# instead of decoding individual resource types.  See the README's
# 'fakeintake decode' section.
ORCH_MANIF_ENDPOINT = "/api/v2/orchmanif"
ORCH_COLLECTOR_ENDPOINT = "/api/v2/orch"
MANIF_BATCH_BUCKET_SECONDS = 15  # orchestrator check ships once every ~15s

# CRDs covered by DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_ENABLED.
#
# When --ootb-crds is set, the cluster-checks-runner's orchestrator check
# registers a builtin collector for each of these resource types. If the
# matching CRD isn't installed in the cluster the collector silently
# logs 'no supported version found' and exits — so on a tiny kind cluster
# 'ootb' alone does almost nothing. We install the CRDs ourselves so the
# collectors actually wire up informers + LIST/WATCH on the apiserver,
# closer to the realistic-production load that the lab is reproducing.
#
# Source-of-truth for which groups/versions are expected:
#   pkg/collector/corechecks/cluster/orchestrator/collector_bundle.go
#   newBuiltinCRDConfigs()
#
# Coverage notes:
#   - Argo, Flux source+kustomize, Karpenter (sh + AWS): installed.
#   - Karpenter Azure (karpenter.azure.com) and EKS Auto Mode nodeclasses
#     (eks.amazonaws.com): intentionally skipped — there's no off-the-shelf
#     standalone CRD manifest for them, and the production Datadog group
#     CRDs (datadogslos, datadogdashboards, datadogagentprofiles,
#     datadogpodautoscalerclusterprofiles) are not shipped by the standard
#     2.20.0 operator chart we install. Both gaps just mean ~5 OOTB
#     collectors no-op, which is fine — the other 13 collectors carry the
#     load.
OOTB_CRD_URLS: tuple[str, ...] = (
    # Argo Rollouts
    "https://raw.githubusercontent.com/argoproj/argo-rollouts/stable/manifests/crds/rollout-crd.yaml",
    # ArgoCD (applications, applicationsets)
    "https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/crds/application-crd.yaml",
    "https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/crds/applicationset-crd.yaml",
    # Flux source-controller (buckets, helmcharts, externalartifacts,
    # gitrepositories, helmrepositories, ocirepositories) — single multi-doc.
    "https://github.com/fluxcd/source-controller/releases/latest/download/source-controller.crds.yaml",
    # Flux kustomize-controller (kustomizations)
    "https://github.com/fluxcd/kustomize-controller/releases/latest/download/kustomize-controller.crds.yaml",
    # Karpenter (karpenter.sh: nodeclaims, nodepools; karpenter.k8s.aws: ec2nodeclasses)
    "https://raw.githubusercontent.com/aws/karpenter-provider-aws/main/pkg/apis/crds/karpenter.sh_nodeclaims.yaml",
    "https://raw.githubusercontent.com/aws/karpenter-provider-aws/main/pkg/apis/crds/karpenter.sh_nodepools.yaml",
    "https://raw.githubusercontent.com/aws/karpenter-provider-aws/main/pkg/apis/crds/karpenter.k8s.aws_ec2nodeclasses.yaml",
)

# When seeding workload CRs we use Flux GitRepository because it has the
# simplest minimum-viable spec (url + interval) of the OOTB types — meaning
# we can bulk-create thousands cheaply via a single kubectl apply without
# needing a controller to reconcile them.
WORKLOAD_CR_NAMESPACE = "crload"


# ----------------------------------------------------------------------------
# Shell helpers
# ----------------------------------------------------------------------------


def log(msg: str) -> None:
    print(f"\033[1;34m[lab]\033[0m {msg}", flush=True)


def warn(msg: str) -> None:
    print(f"\033[1;33m[warn]\033[0m {msg}", flush=True)


def die(msg: str, code: int = 1) -> None:
    print(f"\033[1;31m[fatal]\033[0m {msg}", file=sys.stderr, flush=True)
    sys.exit(code)


def sh(
    cmd: list[str] | str,
    *,
    check: bool = True,
    capture: bool = False,
    stdin: str | None = None,
    env: dict[str, str] | None = None,
    quiet: bool = False,
) -> subprocess.CompletedProcess[str]:
    if isinstance(cmd, str):
        shell = True
        printable = cmd
    else:
        shell = False
        printable = " ".join(shlex.quote(c) for c in cmd)
    if not quiet:
        log(f"$ {printable}")
    return subprocess.run(
        cmd,
        shell=shell,
        check=check,
        text=True,
        input=stdin,
        env={**os.environ, **(env or {})},
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
    )


def kubectl(*args: str, capture: bool = False, stdin: str | None = None, check: bool = True, quiet: bool = False) -> subprocess.CompletedProcess[str]:
    return sh(
        ["kubectl", f"--context=kind-{CLUSTER_NAME}", *args],
        capture=capture,
        stdin=stdin,
        check=check,
        quiet=quiet,
    )


def helm(*args: str, capture: bool = False, check: bool = True) -> subprocess.CompletedProcess[str]:
    return sh(["helm", f"--kube-context=kind-{CLUSTER_NAME}", *args], capture=capture, check=check)


def kind(*args: str, capture: bool = False, check: bool = True) -> subprocess.CompletedProcess[str]:
    return sh(["kind", *args], capture=capture, check=check)


def require_tools() -> None:
    missing = [t for t in ("kind", "kubectl", "helm", "docker", "jq") if shutil.which(t) is None]
    if missing:
        die(f"required tools missing on PATH: {', '.join(missing)}")


def cluster_exists() -> bool:
    out = kind("get", "clusters", capture=True, check=False).stdout or ""
    return CLUSTER_NAME in out.splitlines()


# ----------------------------------------------------------------------------
# kind
# ----------------------------------------------------------------------------
KIND_CONFIG_YAML = f"""\
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: {CLUSTER_NAME}
networking:
  # Default per-node mask is /24 (254 pod IPs). Lab needs ~600+ Pods on the
  # single control-plane node, so expand the node slice to /20 (~4094 IPs).
  podSubnet: "10.244.0.0/16"
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        max-requests-inflight: "800"
        max-mutating-requests-inflight: "400"
    controllerManager:
      extraArgs:
        # Together with kubelet max-pods=2000, lets us run several hundred
        # filler Pods on the single control-plane node.
        node-cidr-mask-size: "20"
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        max-pods: "2000"
        pods-per-core: "0"
  - |
    kind: JoinConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        max-pods: "2000"
        pods-per-core: "0"
"""


def ensure_cluster() -> None:
    if cluster_exists():
        log(f"kind cluster '{CLUSTER_NAME}' already exists, reusing")
        return
    log(f"creating kind cluster '{CLUSTER_NAME}'")
    with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as f:
        f.write(KIND_CONFIG_YAML)
        cfg_path = f.name
    try:
        kind("create", "cluster", "--config", cfg_path, "--image", KIND_NODE_IMAGE, "--wait", "120s")
    finally:
        os.unlink(cfg_path)


def delete_cluster() -> None:
    if not cluster_exists():
        log(f"kind cluster '{CLUSTER_NAME}' does not exist")
        return
    kind("delete", "cluster", "--name", CLUSTER_NAME)


# ----------------------------------------------------------------------------
# Operator
# ----------------------------------------------------------------------------


def ensure_operator(operator_app_version: str) -> None:
    chart_version = OPERATOR_CHART_BY_APP.get(operator_app_version)
    if not chart_version:
        die(
            f"unknown operator app version '{operator_app_version}'. Known: "
            f"{sorted(OPERATOR_CHART_BY_APP)}. "
            f"Pass --operator-chart-version directly to override."
        )
    sh(["helm", "repo", "add", "datadog", "https://helm.datadoghq.com"], check=False, quiet=True)
    sh(["helm", "repo", "update", "datadog"], quiet=True)
    # idempotent install
    rc = helm("status", "datadog-operator", "-n", NAMESPACE, capture=True, check=False).returncode
    if rc == 0:
        log(f"operator already installed in ns/{NAMESPACE}")
        return
    kubectl("create", "namespace", NAMESPACE, check=False, quiet=True)
    helm(
        "install",
        "datadog-operator",
        "datadog/datadog-operator",
        "--namespace",
        NAMESPACE,
        "--version",
        chart_version,
        "--wait",
        "--timeout",
        "3m",
    )


# ----------------------------------------------------------------------------
# fakeintake
# ----------------------------------------------------------------------------
FAKEINTAKE_YAML = f"""\
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fakeintake
  namespace: {NAMESPACE}
spec:
  replicas: 1
  selector: {{ matchLabels: {{ app: fakeintake }} }}
  template:
    metadata: {{ labels: {{ app: fakeintake }} }}
    spec:
      containers:
      - name: fakeintake
        image: {{IMAGE}}
        env:
        - name: STORE_DRIVER
          value: memory
        ports:
        - containerPort: 80
        readinessProbe:
          httpGet: {{ path: /fakeintake/health, port: 80 }}
          periodSeconds: 5
        resources:
          requests: {{ cpu: 100m, memory: 256Mi }}
          limits: {{ cpu: 1, memory: 1Gi }}
---
apiVersion: v1
kind: Service
metadata:
  name: fakeintake
  namespace: {NAMESPACE}
spec:
  selector: {{ app: fakeintake }}
  ports:
  - {{ name: http, port: 80, targetPort: 80 }}
"""


def ensure_fakeintake(image: str) -> None:
    log("applying fakeintake Deployment + Service")
    kubectl("apply", "-f", "-", stdin=FAKEINTAKE_YAML.replace("{IMAGE}", image))
    kubectl(
        "-n",
        NAMESPACE,
        "wait",
        "--for=condition=available",
        "deploy/fakeintake",
        "--timeout=120s",
    )


# ----------------------------------------------------------------------------
# DatadogAgent CR
# ----------------------------------------------------------------------------


def dda_yaml(
    agent_image: str,
    cluster_agent_image: str,
    check_runners: int | None,
    clcr_replicas: int,
    ootb_crds: bool = False,
) -> str:
    """Build the DatadogAgent CR.

    check_runners is None means leave the default (4) — i.e. don't set
    DD_CHECK_RUNNERS, exactly mirroring the production-scale deployment
    that motivated this lab.

    ootb_crds=True enables DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_ENABLED
    on the cluster-agent and cluster-checks runners. This turns on the built-in
    set of CRD collectors (Argo, Flux, Karpenter, Datadog, EKS — ~20 CRD types).
    On a tiny kind cluster most of those CRDs don't exist, but each collector
    still issues an API LIST attempt every Run() of the orchestrator check,
    inflating its execution time from ~13ms to hundreds of ms without inflating
    the manifest payload count. This is how we simulate a 'real cluster'
    orchestrator-check cost in the lab.
    """
    fakeintake_url = f"http://fakeintake.{NAMESPACE}.svc.cluster.local"
    clcr_env_block = [
        f"- {{ name: DD_DD_URL,                                    value: \"{fakeintake_url}\" }}",
        f"- {{ name: DD_SKIP_SSL_VALIDATION,                       value: \"true\" }}",
        f"- {{ name: DD_LEADER_LEASE_DURATION,                     value: \"60\" }}",
    ]
    if check_runners is not None:
        clcr_env_block.append(
            f"- {{ name: DD_CHECK_RUNNERS,                             value: \"{check_runners}\" }}"
        )
    if ootb_crds:
        clcr_env_block.append(
            f"- {{ name: DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_ENABLED, value: \"true\" }}"
        )
    clcr_env_yaml = "\n      ".join(clcr_env_block)
    dca_extra_env = (
        "\n      - { name: DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_ENABLED, value: \"true\" }"
        if ootb_crds else ""
    )
    return f"""\
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: lab
  namespace: {NAMESPACE}
spec:
  global:
    clusterName: {CLUSTER_NAME}
    credentials:
      apiKey: "0000000000000000000000000000000000"
      appKey: "0000000000000000000000000000000000000000"
    site: datadoghq.com
    # kind nodes serve their kubelet API with a self-signed cert that the
    # Datadog agent's default kubelet client rejects. Without this, the
    # node-agent 'orchestrator_pod' check (which reaches the kubelet for
    # per-node Pod collection) errors with 'impossible to reach Kubelet'
    # and Pod data never makes it into the orchestrator pipeline.
    kubelet:
      tlsVerify: false
    tags:
    - "lab:orchestrator-starvation"
  features:
    orchestratorExplorer:
      enabled: true
      ddUrl: "{fakeintake_url}"
    clusterChecks:
      enabled: true
      useClusterChecksRunners: true
    prometheusScrape:
      enabled: true
      enableServiceEndpoints: false
  override:
    nodeAgent:
      image:
        name: {agent_image}
        jmxEnabled: true
      env:
      - name: DD_HOSTNAME
        valueFrom:
          fieldRef: {{ fieldPath: spec.nodeName }}
      - {{ name: DD_DD_URL,                                    value: "{fakeintake_url}" }}
      - {{ name: DD_PROCESS_CONFIG_PROCESS_DD_URL,             value: "{fakeintake_url}" }}
      - {{ name: DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL, value: "{fakeintake_url}" }}
      - {{ name: DD_SKIP_SSL_VALIDATION,                       value: "true" }}
    clusterAgent:
      image:
        name: {cluster_agent_image}
        jmxEnabled: false
      env:
      - {{ name: DD_DD_URL,                                    value: "{fakeintake_url}" }}
      - {{ name: DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL, value: "{fakeintake_url}" }}
      - {{ name: DD_SKIP_SSL_VALIDATION,                       value: "true" }}{dca_extra_env}
    clusterChecksRunner:
      replicas: {clcr_replicas}
      image:
        name: {agent_image}
        jmxEnabled: true
      env:
      - name: DD_HOSTNAME
        valueFrom:
          fieldRef: {{ fieldPath: spec.nodeName }}
      {clcr_env_yaml}
"""


def ensure_dda(
    agent_image: str,
    cluster_agent_image: str,
    check_runners: int | None,
    clcr_replicas: int,
    ootb_crds: bool = False,
) -> None:
    log("applying DatadogAgent CR")
    body = dda_yaml(agent_image, cluster_agent_image, check_runners, clcr_replicas, ootb_crds=ootb_crds)
    kubectl("apply", "-f", "-", stdin=body)
    log("waiting for cluster agent, node agent, and CLCR pods to come up")
    # Cluster agent
    _wait_for_pods("app.kubernetes.io/component=cluster-agent", min_count=1, ready=True, timeout=300)
    # Node agent (daemonset, one per node)
    _wait_for_pods("app.kubernetes.io/component=agent", min_count=1, ready=True, timeout=300)
    # CLCR
    _wait_for_pods(
        "app.kubernetes.io/component=cluster-checks-runner",
        min_count=clcr_replicas,
        ready=True,
        timeout=300,
    )


def _wait_for_pods(label_selector: str, *, min_count: int, ready: bool, timeout: int) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        out = kubectl(
            "-n",
            NAMESPACE,
            "get",
            "pods",
            "-l",
            label_selector,
            "-o",
            "json",
            capture=True,
            quiet=True,
        ).stdout
        pods = json.loads(out).get("items", [])
        ready_count = sum(
            1
            for p in pods
            if any(
                c.get("type") == "Ready" and c.get("status") == "True"
                for c in (p.get("status", {}).get("conditions") or [])
            )
        )
        log(f"  {label_selector}: {ready_count}/{min_count} ready")
        if ready_count >= min_count:
            return
        time.sleep(5)
    die(f"timed out waiting for {min_count} ready pods matching {label_selector}")


# ----------------------------------------------------------------------------
# Noise generator
# ----------------------------------------------------------------------------
SLOW_METRICS_YAML = f"""\
apiVersion: v1
kind: Namespace
metadata:
  name: {NOISE_NAMESPACE}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: slow-metrics-script
  namespace: {NOISE_NAMESPACE}
data:
  server.py: |
    import os, io
    from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
    # Pre-build a large-ish Prometheus payload at startup. The whole point
    # of this redesign is to make the openmetrics check’s Run() dominated
    # by Python-side parse/label-dict work — GIL-held — so we can study
    # the GIL-contention knee in DD_CHECK_RUNNERS. An I/O-bound server
    # (the old SLEEP_SECONDS model) releases the GIL during network wait
    # and shows no GIL ceiling, which masks the actual production bottleneck.
    METRICS_COUNT     = int(os.environ.get("METRICS_COUNT", "5000"))
    LABEL_CARDINALITY = int(os.environ.get("LABEL_CARDINALITY", "500"))
    METHODS = ["GET","POST","PUT","DELETE","PATCH"]
    CODES   = ["200","201","301","400","404","500","502","503"]
    buf = io.BytesIO()
    buf.write(b"# HELP fake_request_count number of fake requests\\n")
    buf.write(b"# TYPE fake_request_count counter\\n")
    for i in range(METRICS_COUNT):
        p = i % LABEL_CARDINALITY
        m = i % len(METHODS)
        c = i % len(CODES)
        line = (
            f'fake_request_count{{{{path="/x{{p}}",method="{{METHODS[m]}}",'
            f'code="{{CODES[c]}}",service="ray",region="us-east-1",'
            f'instance="i-{{i%50}}",version="v1.2.3"}}}} {{i*7+3}}\\n'
        )
        buf.write(line.encode())
    BODY = buf.getvalue()
    print(f"prebuilt body: {{len(BODY):,}} bytes, {{METRICS_COUNT}} metric series", flush=True)
    class H(BaseHTTPRequestHandler):
      def do_GET(self):
        if self.path == "/metrics":
          self.send_response(200)
          self.send_header("Content-Type","text/plain; version=0.0.4")
          self.send_header("Content-Length", str(len(BODY)))
          self.end_headers()
          self.wfile.write(BODY)
        else:
          self.send_response(200); self.send_header("Content-Length","2"); self.end_headers(); self.wfile.write(b"ok")
      def log_message(self, *a, **k): pass
    # ThreadingHTTPServer so concurrent scrapes don’t serialize at the
    # server. The agent-side parsing IS what we want to serialise via GIL.
    ThreadingHTTPServer(("", 8080), H).serve_forever()
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: slow-metrics
  namespace: {NOISE_NAMESPACE}
spec:
  replicas: 1
  selector: {{ matchLabels: {{ app: slow-metrics }} }}
  template:
    metadata: {{ labels: {{ app: slow-metrics }} }}
    spec:
      containers:
      - name: server
        image: python:3.13-slim
        command: ["python","/script/server.py"]
        env:
        - name: METRICS_COUNT
          value: "5000"
        - name: LABEL_CARDINALITY
          value: "500"
        ports:
        - containerPort: 8080
        resources:
          requests: {{ cpu: "100m", memory: "128Mi" }}
          limits:   {{ cpu: "2000m", memory: "512Mi" }}
        volumeMounts:
        - {{ name: script, mountPath: /script }}
        readinessProbe:
          tcpSocket: {{ port: 8080 }}
          periodSeconds: 5
      volumes:
      - name: script
        configMap: {{ name: slow-metrics-script }}
"""


def _service_yaml(index: int) -> str:
    return f"""\
apiVersion: v1
kind: Service
metadata:
  name: slow-metrics-{index:04d}
  namespace: {NOISE_NAMESPACE}
  labels:
    # Used by ensure_noise() to bulk-delete via the apiserver's
    # DELETE-collection endpoint (one API call instead of N) when
    # scaling noise count down between sweep cells. Without the
    # label, kubectl falls back to per-name DELETEs which are
    # serial and run at ~10–20 services/second in kind.
    lab.example.com/role: noise-svc
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    prometheus.io/path: "/metrics"
spec:
  selector: {{ app: slow-metrics }}
  ports:
  - {{ name: metrics, port: 8080, targetPort: 8080 }}
"""


WORKLOAD_YAML = f"""\
apiVersion: v1
kind: Namespace
metadata: {{ name: {WORKLOAD_NAMESPACE} }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: filler
  namespace: {WORKLOAD_NAMESPACE}
spec:
  replicas: 0  # set by ensure_workload
  selector: {{ matchLabels: {{ app: filler }} }}
  template:
    metadata:
      labels: {{ app: filler }}
      annotations:
        # Realistic-looking annotations — increase per-pod manifest weight.
        # The orchestrator check container-scrubber iterates these too.
        lab.example.com/owner: "orchestrator-starvation-lab"
        lab.example.com/purpose: "increase per-Run cost of orchestrator check"
    spec:
      terminationGracePeriodSeconds: 0
      containers:
      - name: pause
        image: registry.k8s.io/pause:3.10
        resources:
          requests: {{ cpu: "1m",  memory: "4Mi" }}
          limits:   {{ cpu: "5m",  memory: "8Mi" }}
        # A few env vars give the container-scrubber something to chew on,
        # closer to real-workload manifest size.
        env:
        - {{ name: APP_NAME,       value: "filler" }}
        - {{ name: APP_VERSION,    value: "1.0.0" }}
        - {{ name: LOG_LEVEL,      value: "INFO" }}
        - {{ name: DEPLOY_REGION,  value: "us-east-1" }}
        - {{ name: CLUSTER_NAME,   value: "orch-starve" }}
"""


def ensure_workload(pods: int) -> None:
    """Bring up `pods` pause-container Pods in the workload namespace.

    These Pods aren't backed by Services and don't run any check — they
    exist purely as k8s objects for the orchestrator-explorer check to
    iterate over each Run(). Per-pod manifest cost (YAML marshalling +
    container scrubbing + tag enrichment) is the dominant scaling factor
    for orchestrator-check execution time in real clusters; pause Pods
    cost ~1m CPU / 4Mi RAM each so a kind cluster can host hundreds.

    Scale-up is done in 100-pod waves with 30s pauses between, because
    bursting hundreds of pods at once swamps systemd's cgroup-scope
    creation on a single-node kind cluster and starves the Datadog pods.
    """
    log(f"applying workload deployment with replicas={pods}")
    kubectl("apply", "-f", "-", stdin=WORKLOAD_YAML)
    if pods == 0:
        kubectl("-n", WORKLOAD_NAMESPACE, "scale", "deploy/filler", "--replicas=0")
        return

    wave = 100
    current = 0
    while current < pods:
        next_target = min(current + wave, pods)
        log(f"scaling filler deployment {current} -> {next_target}")
        kubectl(
            "-n", WORKLOAD_NAMESPACE,
            "scale", "deploy/filler", f"--replicas={next_target}",
        )
        # Wait for this wave to be (mostly) Running before kicking the next.
        # We accept up to 10% laggards per wave so a few stuck pods don't
        # block the whole ramp.
        deadline = time.time() + 240
        while time.time() < deadline:
            ready_str = kubectl(
                "-n", WORKLOAD_NAMESPACE, "get", "deploy/filler",
                "-o", "jsonpath={.status.readyReplicas}",
                capture=True, check=False, quiet=True,
            ).stdout.strip()
            ready = int(ready_str) if ready_str.isdigit() else 0
            if ready >= int(next_target * 0.9):
                log(f"  wave ready: {ready}/{next_target}")
                break
            time.sleep(5)
        else:
            warn(f"filler wave timed out at {ready}/{next_target}; continuing")
        current = next_target
        # Brief pause between waves to let systemd settle.
        if current < pods:
            time.sleep(15)


def ensure_noise(prom_endpoints: int) -> None:
    log(f"applying noise generator with {prom_endpoints} prometheus-annotated Services")
    kubectl("apply", "-f", "-", stdin=SLOW_METRICS_YAML)

    # Quick count of existing noise Services so we know whether this
    # call is scaling up (no nuke needed, just additive apply) or down
    # (need to remove stale services from a previous run/cell).
    existing_names = kubectl(
        "-n", NOISE_NAMESPACE, "get", "svc",
        "-o", "name", "-l", "lab.example.com/role=noise-svc",
        capture=True, check=False, quiet=True,
    ).stdout.split()
    existing_count = len(existing_names)

    if existing_count > prom_endpoints:
        # Scale-down path. Bulk-delete via the apiserver's DELETE-collection
        # endpoint by label selector — a single API call that the apiserver
        # processes atomically server-side, in contrast to the previous
        # per-name kubectl-delete loop that took several minutes to clear
        # 3000 services. We always delete all labelled noise services and
        # then re-create the target set, because deletecollection has no
        # 'all-except-these' filter and emulating one via field-selectors
        # is more code than it's worth.
        log(
            f"bulk-deleting {existing_count} noise services via deletecollection "
            f"(target={prom_endpoints})"
        )
        kubectl(
            "-n", NOISE_NAMESPACE, "delete", "svc",
            "-l", "lab.example.com/role=noise-svc",
            "--wait=false", "--grace-period=1",
            check=False, quiet=True,
        )
        # The deletecollection call returns once the apiserver has accepted
        # the request. Object removal from informer caches may lag by a
        # second or two as watch events propagate; wait for it so the
        # next sweep cell's observe doesn't see leftover prometheus-HTTP-SD
        # check configs from this cell.
        deadline = time.time() + 120
        while time.time() < deadline:
            remaining = kubectl(
                "-n", NOISE_NAMESPACE, "get", "svc",
                "-o", "name", "-l", "lab.example.com/role=noise-svc",
                capture=True, check=False, quiet=True,
            ).stdout.split()
            if not remaining:
                break
            time.sleep(2)
        else:
            warn(f"deletecollection drain timed out with {len(remaining)} services still present")

    if prom_endpoints == 0:
        # Nothing more to do — the deletecollection (if any) already cleared
        # all noise services. We intentionally do not delete the slow-metrics
        # Deployment or the namespace because they're cheap to keep around
        # and recreating them adds ~30s of rollout time.
        return

    # Build a single concatenated YAML and apply once for speed.
    chunks: list[str] = []
    for i in range(prom_endpoints):
        chunks.append(_service_yaml(i))
        if len(chunks) >= 200:
            kubectl("apply", "-f", "-", stdin="---\n".join(chunks), quiet=True)
            chunks = []
    if chunks:
        kubectl("apply", "-f", "-", stdin="---\n".join(chunks), quiet=True)
    # Wait for the backing deployment.
    kubectl(
        "-n",
        NOISE_NAMESPACE,
        "wait",
        "--for=condition=available",
        "deploy/slow-metrics",
        "--timeout=120s",
    )


def ensure_ootb_crds() -> None:
    """Install the CRDs covered by DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_ENABLED.

    Without these, the orchestrator-explorer 'ootb' built-in CR collectors
    can't bind to anything in a tiny kind cluster (Argo / Flux / Karpenter
    aren't installed) and the --ootb-crds flag is effectively a no-op.
    Installing them lets the agent's discovery + permission-check + informer
    machinery actually wire up against each group/version on every check Run
    and on periodic resync.

    See ``OOTB_CRD_URLS`` for the full list and rationale.
    """
    log(f"installing {len(OOTB_CRD_URLS)} OOTB CRDs (Argo / Flux / Karpenter)")
    for url in OOTB_CRD_URLS:
        log(f"  apply {url}")
        # ArgoCD's ApplicationSet CRD is large enough to trip the
        # client-side last-applied-configuration annotation limit, so we
        # always use server-side apply — cheap, idempotent, and works for
        # the smaller CRDs too.
        kubectl(
            "apply",
            "--server-side",
            "--force-conflicts",
            "-f",
            url,
            check=False,  # tolerate transient apiserver hiccups; we retry whole-script
            quiet=True,
        )


def ensure_workload_deployments(n: int, replicas: int = 1) -> None:
    """Create N tiny standalone Deployments in the workload namespace.

    Each Deployment runs ``replicas`` pause containers. With ``replicas=0``
    only the Deployment and its empty ReplicaSet exist — useful when you
    want to scale past kind’s ~2000-pod-per-node ceiling: 5000 Deployments
    × 1 replica = 5000 pods (over budget), but 5000 Deployments × 0 replicas
    = 0 pods (fits trivially). At 0 replicas the CLC orchestrator check
    still sees N Deployments + N ReplicaSets in its informer caches.

    Each non-zero-replicas Deployment contributes 3 cluster-scoped objects
    (Deployment + ReplicaSet + Pod) to the orchestrator check's per-Run
    work. The natural scaling knob for cluster-scoped orchestrator load.

    Distinct from --workload-pods (which produces N pods backed by a
    *single* Deployment+ReplicaSet — useful for stressing the node-agent's
    'orchestrator_pod' check but invisible to the CLC orchestrator check
    because Deployment count stays at 1).
    """
    log(f"applying {n} standalone workload Deployments (replicas={replicas})")
    # Ensure namespace exists.
    kubectl(
        "apply", "-f", "-",
        stdin=f"apiVersion: v1\nkind: Namespace\nmetadata: {{ name: {WORKLOAD_NAMESPACE} }}\n",
        quiet=True,
    )
    if n == 0:
        kubectl(
            "-n", WORKLOAD_NAMESPACE,
            "delete", "deploy", "-l", "lab.example.com/role=workload-deployment",
            check=False, quiet=True,
        )
        return

    def _deploy_yaml(i: int) -> str:
        return (
            f"apiVersion: apps/v1\n"
            f"kind: Deployment\n"
            f"metadata:\n"
            f"  name: wd-{i:04d}\n"
            f"  namespace: {WORKLOAD_NAMESPACE}\n"
            f"  labels: {{ lab.example.com/role: workload-deployment }}\n"
            f"spec:\n"
            f"  replicas: {replicas}\n"
            f"  selector: {{ matchLabels: {{ app: wd-{i:04d} }} }}\n"
            f"  template:\n"
            f"    metadata: {{ labels: {{ app: wd-{i:04d} }} }}\n"
            f"    spec:\n"
            f"      terminationGracePeriodSeconds: 0\n"
            f"      containers:\n"
            f"      - name: pause\n"
            f"        image: registry.k8s.io/pause:3.10\n"
            f"        resources:\n"
            f"          requests: {{ cpu: \"1m\", memory: \"4Mi\" }}\n"
            f"          limits:   {{ cpu: \"5m\", memory: \"8Mi\" }}\n"
        )

    chunks: list[str] = []
    for i in range(n):
        chunks.append(_deploy_yaml(i))
        if len(chunks) >= 200:
            kubectl("apply", "-f", "-", stdin="---\n".join(chunks), quiet=True)
            chunks = []
    if chunks:
        kubectl("apply", "-f", "-", stdin="---\n".join(chunks), quiet=True)


def ensure_workload_crs(n: int) -> None:
    """Create N Flux GitRepository CRs in a dedicated namespace.

    These are NOT reconciled by any controller (we never install
    source-controller itself) — they exist purely as stored objects so the
    orchestrator check's Flux 'gitrepositories' informer has a populated
    cache to walk on every Run() and serialize into manifest payloads.

    Pre-requisite: the source.toolkit.fluxcd.io CRDs must be installed,
    which ensure_ootb_crds() handles when --ootb-crds is set.
    """
    log(f"applying {n} workload GitRepository CRs in ns/{WORKLOAD_CR_NAMESPACE}")
    kubectl(
        "apply", "-f", "-",
        stdin=f"apiVersion: v1\nkind: Namespace\nmetadata: {{ name: {WORKLOAD_CR_NAMESPACE} }}\n",
        quiet=True,
    )
    if n == 0:
        kubectl(
            "-n", WORKLOAD_CR_NAMESPACE,
            "delete", "gitrepositories.source.toolkit.fluxcd.io", "--all",
            check=False, quiet=True,
        )
        return

    def _cr_yaml(i: int) -> str:
        return (
            f"apiVersion: source.toolkit.fluxcd.io/v1\n"
            f"kind: GitRepository\n"
            f"metadata:\n"
            f"  name: gr-{i:04d}\n"
            f"  namespace: {WORKLOAD_CR_NAMESPACE}\n"
            f"spec:\n"
            f"  interval: 1h\n"
            f"  url: https://example.com/r{i:04d}.git\n"
        )

    chunks: list[str] = []
    for i in range(n):
        chunks.append(_cr_yaml(i))
        if len(chunks) >= 250:
            kubectl("apply", "-f", "-", stdin="---\n".join(chunks), quiet=True)
            chunks = []
    if chunks:
        kubectl("apply", "-f", "-", stdin="---\n".join(chunks), quiet=True)



# ----------------------------------------------------------------------------
# fakeintake querying via port-forward
# ----------------------------------------------------------------------------


@contextlib.contextmanager
def port_forward(local_port: int = FAKEINTAKE_LOCAL_PORT):
    """kubectl port-forward as a context manager.  Yields the local base URL."""
    proc = subprocess.Popen(
        [
            "kubectl",
            f"--context=kind-{CLUSTER_NAME}",
            "-n",
            NAMESPACE,
            "port-forward",
            "svc/fakeintake",
            f"{local_port}:80",
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    # Wait for the local port to become reachable.
    deadline = time.time() + 30
    while time.time() < deadline:
        with contextlib.suppress(OSError):
            with socket.create_connection(("127.0.0.1", local_port), timeout=1):
                break
        time.sleep(0.2)
    else:
        proc.terminate()
        die("port-forward to fakeintake never became reachable")
    try:
        yield f"http://127.0.0.1:{local_port}"
    finally:
        proc.terminate()
        with contextlib.suppress(subprocess.TimeoutExpired):
            proc.wait(timeout=5)


def http_get_json(url: str, *, timeout: int = 10) -> dict:
    req = urllib.request.Request(url)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read())


def fakeintake_route_stats(base: str) -> dict[str, int]:
    data = http_get_json(f"{base}/fakeintake/routestats")
    return {k: v.get("count", 0) for k, v in data.get("routes", {}).items()}


def fakeintake_payloads(base: str, endpoint: str) -> list[dict]:
    """Fetch raw payloads recorded by fakeintake on the given endpoint.

    Each payload has at least 'timestamp', 'data' (base64-encoded body),
    'encoding' and 'content_type'.  We don't decode 'data' here — the
    orchestrator v2 routes use zstd+protobuf and fakeintake has no
    aggregator wired for them.  We just need timestamps for rate counting.
    """
    url = f"{base}/fakeintake/payloads?endpoint={endpoint}"
    req = urllib.request.Request(url)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            data = json.loads(r.read())
    except urllib.error.HTTPError as e:
        warn(f"fakeintake /payloads({endpoint}) failed: {e}")
        return []
    return data.get("payloads", []) or []


def _payload_epoch(p: dict) -> float | None:
    ts = p.get("timestamp")
    if not ts:
        return None
    try:
        from datetime import datetime
        return datetime.fromisoformat(ts.replace("Z", "+00:00")).timestamp()
    except Exception:
        return None


def count_in_window(payloads: list[dict], since_epoch: float) -> int:
    n = 0
    for p in payloads:
        ts = _payload_epoch(p)
        if ts is None or ts >= since_epoch:
            n += 1
    return n


def count_batches_in_window(payloads: list[dict], since_epoch: float, bucket_seconds: int = MANIF_BATCH_BUCKET_SECONDS) -> int:
    """Count distinct ``bucket_seconds``-second buckets that contain >= 1 payload.

    A 'batch' approximates one orchestrator-check run: when the check runs
    successfully, it ships ~3+ manifest payloads (one per K8s resource type)
    within ~1 second, every ``min_collection_interval`` (default 15s).
    Saturation produces gaps in this cadence.
    """
    buckets: set[int] = set()
    for p in payloads:
        ts = _payload_epoch(p)
        if ts is None or ts < since_epoch:
            continue
        buckets.add(int(ts // bucket_seconds))
    return len(buckets)


# Threshold for grouping payload timestamps into a single "batch" (one
# orchestrator-check Run() ships its 3+ manifest payloads within ~1s).
# Anything farther apart than this is treated as a separate batch — i.e.
# a separate check run. Tuned generously so that occasional
# back-pressured payloads inside a single run don't fragment the batch,
# while normal idle cadence (>=10s) is clearly separated.
BATCH_GAP_THRESHOLD_SECONDS = 4.0


def batch_starts_in_window(
    payloads: list[dict],
    since_epoch: float,
    gap_threshold: float = BATCH_GAP_THRESHOLD_SECONDS,
) -> list[float]:
    """Cluster payload timestamps into batches; return the start time of each.

    Two consecutive payloads (sorted by timestamp) belong to the same batch
    iff their gap < ``gap_threshold`` seconds. The 'start' of a batch is
    the timestamp of its earliest payload.

    A batch is the empirical fingerprint of one orchestrator-check Run():
    the check ships several per-resource manifest payloads in rapid
    succession, then is silent until the next dispatch. Inter-batch gap
    distribution measures dispatch cadence — the actual starvation signal.
    """
    ts_list = sorted(
        ts for ts in (_payload_epoch(p) for p in payloads)
        if ts is not None and ts >= since_epoch
    )
    if not ts_list:
        return []
    starts: list[float] = [ts_list[0]]
    prev = ts_list[0]
    for ts in ts_list[1:]:
        if ts - prev >= gap_threshold:
            starts.append(ts)
        prev = ts
    return starts


def _percentile(values: list[float], pct: float) -> float:
    """Linear-interpolated percentile (0..100). Empty -> 0.0."""
    if not values:
        return 0.0
    vs = sorted(values)
    if len(vs) == 1:
        return vs[0]
    k = (pct / 100.0) * (len(vs) - 1)
    lo, hi = int(k), min(int(k) + 1, len(vs) - 1)
    return vs[lo] + (vs[hi] - vs[lo]) * (k - lo)


def gap_stats(batch_starts: list[float]) -> tuple[int, float, float, float]:
    """Return (n_gaps, p50_gap_s, p95_gap_s, max_gap_s) between consecutive batches.

    With N batch starts there are N-1 gaps. p50 = median, p95 = 95th
    percentile. All values are seconds. n_gaps < 2 -> all zeros (not
    enough data to characterise cadence in this window).
    """
    if len(batch_starts) < 2:
        return 0, 0.0, 0.0, 0.0
    gaps = [batch_starts[i + 1] - batch_starts[i] for i in range(len(batch_starts) - 1)]
    return len(gaps), _percentile(gaps, 50), _percentile(gaps, 95), max(gaps)




# ----------------------------------------------------------------------------
# Subcommands
# ----------------------------------------------------------------------------


def cmd_up(args: argparse.Namespace) -> None:
    require_tools()
    ensure_cluster()
    ensure_operator(args.operator_version)
    ensure_fakeintake(args.fakeintake_image)
    ensure_noise(args.prom_endpoints)
    # OOTB CRDs must exist BEFORE the cluster-checks-runner starts — the
    # agent's builtin-CRD discovery runs once at check-bundle init and
    # silently drops collectors whose CRD isn't present, with no retry.
    if args.ootb_crds:
        ensure_ootb_crds()
    # DDA goes up BEFORE the filler workload so the Datadog pods grab their
    # cgroup-scope slots before the systemd contention from hundreds of
    # filler pods can starve them.
    check_runners = None if args.check_runners == "default" else int(args.check_runners)
    ensure_dda(
        agent_image=args.agent_image,
        cluster_agent_image=args.cluster_agent_image,
        check_runners=check_runners,
        clcr_replicas=args.clcr_replicas,
        ootb_crds=args.ootb_crds,
    )
    ensure_workload(args.workload_pods)
    ensure_workload_deployments(args.workload_deployments, replicas=args.workload_replicas)
    if args.workload_crs:
        if not args.ootb_crds:
            warn(
                "--workload-crs > 0 but --ootb-crds not set; CRs will be created "
                "but no orchestrator collector will be listening for them"
            )
        ensure_workload_crs(args.workload_crs)
    log("== up complete ==")
    log(
        f"fakeintake reachable via: kubectl --context=kind-{CLUSTER_NAME} "
        f"-n {NAMESPACE} port-forward svc/fakeintake {FAKEINTAKE_LOCAL_PORT}:80"
    )
    log(f"then: curl http://127.0.0.1:{FAKEINTAKE_LOCAL_PORT}/fakeintake/routestats")


def cmd_observe(args: argparse.Namespace) -> None:
    require_tools()
    window = parse_duration(args.window)
    interval = parse_duration(args.interval)
    deadline = time.time() + parse_duration(args.duration) if args.duration else None
    with port_forward() as base:
        log(f"observing orchestrator manifest rate; window={window}s interval={interval}s")
        # New columns vs the original observe:
        #   manif_batches: # of distinct check Runs in the window (gap-clustered
        #                  payload timestamps, not 15s-bucketed)
        #   manif_gap_p50_s / _p95_s / _max_s: gap between consecutive check
        #                  Runs in the window. Idle baseline should match the
        #                  scheduled interval (~10s with --ootb-crds, ~15s
        #                  otherwise). Under CLC starvation these inflate well
        #                  above the scheduled interval and are the direct
        #                  signal that the check is being dispatched late.
        print(
            "timestamp,window_seconds,"
            "orchmanif_payloads_in_window,orchmanif_batches_in_window,"
            "orch_payloads_in_window,total_orchmanif_payloads,total_orch_payloads,"
            "manif_batches,manif_gap_p50_s,manif_gap_p95_s,manif_gap_max_s,"
            "coll_batches,coll_gap_p50_s,coll_gap_p95_s,coll_gap_max_s"
        )
        while True:
            now = time.time()
            since = now - window
            manif = fakeintake_payloads(base, ORCH_MANIF_ENDPOINT)
            coll = fakeintake_payloads(base, ORCH_COLLECTOR_ENDPOINT)
            manif_starts = batch_starts_in_window(manif, since)
            coll_starts = batch_starts_in_window(coll, since)
            m_n, m_p50, m_p95, m_max = gap_stats(manif_starts)
            c_n, c_p50, c_p95, c_max = gap_stats(coll_starts)
            row = [
                int(now),
                window,
                count_in_window(manif, since),
                count_batches_in_window(manif, since),
                count_in_window(coll, since),
                len(manif),
                len(coll),
                len(manif_starts),
                f"{m_p50:.2f}",
                f"{m_p95:.2f}",
                f"{m_max:.2f}",
                len(coll_starts),
                f"{c_p50:.2f}",
                f"{c_p95:.2f}",
                f"{c_max:.2f}",
            ]
            print(",".join(str(x) for x in row), flush=True)
            if deadline and time.time() >= deadline:
                return
            time.sleep(interval)


def cmd_down(_: argparse.Namespace) -> None:
    require_tools()
    delete_cluster()


# ----------------------------------------------------------------------------
# Sweep
# ----------------------------------------------------------------------------


@dataclass
class Cell:
    """One configuration in a sweep matrix.

    Maps directly to the knobs of ``cmd_up`` plus a label for reports.
    Defaults match the script's '--up' defaults so a cell only needs to
    specify the dimensions it varies.
    """
    label: str
    prom_endpoints: int = 0
    workload_pods: int = 0
    workload_deployments: int = 0
    workload_crs: int = 0
    clcr_replicas: int = 2
    check_runners: str = "default"     # "default" → leave DD_CHECK_RUNNERS unset (=4)
    ootb_crds: bool = False


# Default sweep matrix — four cells designed to demonstrate the negative
# result that the orchestrator check is NOT dispatch-starved by CLC noise.
#
# Dimension 1: noise count (0 → 300) saturates the CLC worker pool.
# Dimension 2: worker capacity (default 8 → 1) starves the pool faster.
#
# Each cell holds the orchestrator-side load constant (100 workload
# Deployments + 500 CRs + OOTB CRDs) so any change in the orchestrator
# check's cadence is attributable to the noise/worker dimensions, not
# to a change in what the check has to collect.
DEFAULT_SWEEP_CELLS: tuple[Cell, ...] = (
    Cell("idle",        prom_endpoints=0,   workload_deployments=100, workload_crs=500, ootb_crds=True),
    Cell("noise-100",   prom_endpoints=100, workload_deployments=100, workload_crs=500, ootb_crds=True),
    Cell("noise-300",   prom_endpoints=300, workload_deployments=100, workload_crs=500, ootb_crds=True),
    Cell("saturated",   prom_endpoints=300, workload_deployments=100, workload_crs=500, ootb_crds=True,
         clcr_replicas=1, check_runners="1"),
)

# Knee sweep — scale noise count up to and past the production-scale
# scenario this lab models (~3000+ openmetrics cluster checks) while
# holding worker capacity at the production-default (2 runners × 4 workers).
# Goal: locate the point, if any, where the CLC scheduler starts
# dispatching the orchestrator check late.
#
# Cell ordering is monotonically increasing in noise count so each
# step is a scale-up; the scale-down path through ensure_noise has
# its own delete pass but stepping up keeps each transition fast.
KNEE_SWEEP_CELLS: tuple[Cell, ...] = (
    Cell("idle",        prom_endpoints=0,    workload_deployments=100, workload_crs=500, ootb_crds=True),
    Cell("noise-100",   prom_endpoints=100,  workload_deployments=100, workload_crs=500, ootb_crds=True),
    Cell("noise-300",   prom_endpoints=300,  workload_deployments=100, workload_crs=500, ootb_crds=True),
    Cell("noise-1000",  prom_endpoints=1000, workload_deployments=100, workload_crs=500, ootb_crds=True),
    Cell("noise-3000",  prom_endpoints=3000, workload_deployments=100, workload_crs=500, ootb_crds=True),
)

SWEEP_SCENARIOS: dict[str, tuple[Cell, ...]] = {
    "default": DEFAULT_SWEEP_CELLS,
    "knee":    KNEE_SWEEP_CELLS,
}


def _cell_as_dict(c: Cell) -> dict:
    return {
        "label": c.label,
        "prom_endpoints": c.prom_endpoints,
        "workload_pods": c.workload_pods,
        "workload_deployments": c.workload_deployments,
        "workload_crs": c.workload_crs,
        "clcr_replicas": c.clcr_replicas,
        "check_runners": c.check_runners,
        "ootb_crds": c.ootb_crds,
    }


def apply_cell(cell: Cell, agent_image: str, cluster_agent_image: str, operator_version: str) -> None:
    """Reconfigure the live cluster to match ``cell``.

    Reuses the existing kind cluster + operator + fakeintake. Re-applies
    the DDA CR (the operator handles the rollout), scales noise services,
    and scales the workload Deployments/CRs. Idempotent: safe to call on
    the same cell twice in a row.
    """
    log(f"→ applying cell '{cell.label}'")
    ensure_cluster()
    ensure_operator(operator_version)
    ensure_fakeintake(FAKEINTAKE_IMAGE_DEFAULT)
    ensure_noise(cell.prom_endpoints)
    if cell.ootb_crds:
        ensure_ootb_crds()
    check_runners = None if cell.check_runners == "default" else int(cell.check_runners)
    ensure_dda(
        agent_image=agent_image,
        cluster_agent_image=cluster_agent_image,
        check_runners=check_runners,
        clcr_replicas=cell.clcr_replicas,
        ootb_crds=cell.ootb_crds,
    )
    ensure_workload(cell.workload_pods)
    ensure_workload_deployments(cell.workload_deployments)
    if cell.workload_crs:
        ensure_workload_crs(cell.workload_crs)


def _observe_to_csv(
    base: str,
    out_path: pathlib.Path,
    window_s: int,
    interval_s: int,
    duration_s: int,
) -> None:
    """Run a single observe pass against an already-open fakeintake port-forward.

    Writes a CSV with the same schema as ``cmd_observe`` to ``out_path``.
    Returns when ``duration_s`` elapses.
    """
    deadline = time.time() + duration_s
    with out_path.open("w") as f:
        f.write(
            "timestamp,window_seconds,"
            "orchmanif_payloads_in_window,orchmanif_batches_in_window,"
            "orch_payloads_in_window,total_orchmanif_payloads,total_orch_payloads,"
            "manif_batches,manif_gap_p50_s,manif_gap_p95_s,manif_gap_max_s,"
            "coll_batches,coll_gap_p50_s,coll_gap_p95_s,coll_gap_max_s\n"
        )
        while True:
            now = time.time()
            since = now - window_s
            manif = fakeintake_payloads(base, ORCH_MANIF_ENDPOINT)
            coll = fakeintake_payloads(base, ORCH_COLLECTOR_ENDPOINT)
            m_starts = batch_starts_in_window(manif, since)
            c_starts = batch_starts_in_window(coll, since)
            _, m_p50, m_p95, m_max = gap_stats(m_starts)
            _, c_p50, c_p95, c_max = gap_stats(c_starts)
            row = [
                int(now), window_s,
                count_in_window(manif, since),
                count_batches_in_window(manif, since),
                count_in_window(coll, since),
                len(manif), len(coll),
                len(m_starts), f"{m_p50:.2f}", f"{m_p95:.2f}", f"{m_max:.2f}",
                len(c_starts), f"{c_p50:.2f}", f"{c_p95:.2f}", f"{c_max:.2f}",
            ]
            f.write(",".join(str(x) for x in row) + "\n")
            f.flush()
            if time.time() >= deadline:
                return
            time.sleep(interval_s)


def _fakeintake_reset(base: str) -> None:
    """Drop all stored payloads in fakeintake.

    Called between sweep cells so each cell's observe window only sees
    its own payloads, not the previous cell's residue. The reset is best-
    effort: a 404 means fakeintake is older and doesn't support it, in
    which case we tolerate cross-cell payload bleed (acceptable because
    our window is short relative to the reset gap, and idle baselines
    self-correct quickly).
    """
    try:
        req = urllib.request.Request(f"{base}/fakeintake/flushPayloads", method="POST")
        urllib.request.urlopen(req, timeout=5)
    except Exception as e:
        warn(f"fakeintake reset failed (continuing anyway): {e}")


def cmd_sweep(args: argparse.Namespace) -> None:
    """Walk a matrix of cells, capturing one observe CSV per cell.

    Writes ``runs/sweep-<ts>-manifest.json`` plus one
    ``runs/sweep-<ts>-cell-<NN>-<label>.csv`` per cell. The manifest
    is what ``report`` consumes.
    """
    require_tools()
    if args.scenario not in SWEEP_SCENARIOS:
        die(f"unknown scenario '{args.scenario}'; available: {', '.join(SWEEP_SCENARIOS)}")
    cells = list(SWEEP_SCENARIOS[args.scenario])
    runs_dir = pathlib.Path(args.runs_dir)
    runs_dir.mkdir(parents=True, exist_ok=True)
    sweep_id = f"{args.scenario}-{time.strftime('%Y%m%d-%H%M%S')}"
    settle_s = parse_duration(args.settle)
    window_s = parse_duration(args.window)
    interval_s = parse_duration(args.observe_interval)
    duration_s = parse_duration(args.observe_duration)

    cell_records: list[dict] = []
    log(f"sweep {sweep_id}: {len(cells)} cells, settle={settle_s}s, window={window_s}s, observe={duration_s}s")
    for i, cell in enumerate(cells):
        log(f"=== cell {i+1}/{len(cells)}: {cell.label} ===")
        apply_cell(cell, args.agent_image, args.cluster_agent_image, args.operator_version)
        log(f"settling for {settle_s}s before observe")
        time.sleep(settle_s)
        out_path = runs_dir / f"sweep-{sweep_id}-cell-{i:02d}-{cell.label}.csv"
        log(f"observe → {out_path}")
        with port_forward() as base:
            _fakeintake_reset(base)
            # Re-settle briefly after reset so the first sample isn't empty.
            time.sleep(min(window_s, 30))
            _observe_to_csv(base, out_path, window_s, interval_s, duration_s)
        cell_records.append({
            **_cell_as_dict(cell),
            "csv": out_path.name,
        })

    manifest_path = runs_dir / f"sweep-{sweep_id}-manifest.json"
    with manifest_path.open("w") as f:
        json.dump({
            "sweep_id": sweep_id,
            "scenario": args.scenario,
            "agent_image": args.agent_image,
            "cluster_agent_image": args.cluster_agent_image,
            "window_seconds": window_s,
            "observe_interval_seconds": interval_s,
            "observe_duration_seconds": duration_s,
            "settle_seconds": settle_s,
            "cells": cell_records,
        }, f, indent=2)
    log(f"== sweep complete ==  {manifest_path}")
    log(f"render with: ./orchestrator-starvation.py report --manifest {manifest_path}")


# ----------------------------------------------------------------------------
# Report
# ----------------------------------------------------------------------------


REPORT_HTML_TEMPLATE = """<!doctype html>
<html lang="en"><head>
<meta charset="utf-8">
<title>orchestrator-starvation sweep — {sweep_id}</title>
<style>
  body {{ font-family: -apple-system, system-ui, sans-serif; margin: 24px; max-width: 1100px; }}
  h1 {{ font-size: 1.6em; margin-bottom: 0; }}
  h2 {{ margin-top: 1.5em; }}
  .meta {{ color: #666; font-size: 0.9em; margin-bottom: 1em; }}
  table {{ border-collapse: collapse; margin: 1em 0; }}
  th, td {{ border: 1px solid #ddd; padding: 6px 12px; text-align: right; font-variant-numeric: tabular-nums; }}
  th {{ background: #f6f8fa; }}
  th.left, td.left {{ text-align: left; }}
  .key {{ background: #fffbe6; padding: 12px 16px; border-left: 4px solid #f0c020; margin: 1em 0; }}
  svg {{ background: #fafbfc; border: 1px solid #ddd; }}
  .cell-block {{ margin-bottom: 2em; }}
  .scale {{ font-size: 0.75em; fill: #666; }}
  .axis {{ stroke: #888; stroke-width: 1; }}
  .legend {{ font-size: 0.85em; color: #444; }}
  .legend span {{ display: inline-block; margin-right: 14px; }}
  .legend i {{ display: inline-block; width: 16px; height: 3px; vertical-align: middle; margin-right: 4px; }}
</style>
</head><body>
<h1>orchestrator-starvation — sweep <code>{sweep_id}</code></h1>
<div class="meta">
  agent <code>{agent_image}</code> — cluster-agent <code>{cluster_agent_image}</code><br>
  per-cell observe window {window_s}s, sampled every {interval_s}s for {duration_s}s after a {settle_s}s settle.
</div>

<div class="key">
  <strong>Key signal:</strong> <code>manif_gap_p50_s</code> / <code>_p95_s</code> / <code>_max_s</code> are the gaps
  between consecutive orchestrator-check Run()s, in seconds. Target cadence is the check's
  <code>min_collection_interval</code> (15s by default). p50 = median dispatch period,
  p95/max = tail of dispatch slip under load.
</div>

<h2>Summary</h2>
<table>
  <thead><tr>
    <th class="left">cell</th>
    <th>prom_endpoints</th>
    <th>workload_deps</th>
    <th>workload_crs</th>
    <th>clcr_replicas</th>
    <th>workers/runner</th>
    <th class="left">ootb_crds</th>
    <th>manif p50 (s)</th>
    <th>manif p95 (s)</th>
    <th>manif max (s)</th>
    <th>coll p50 (s)</th>
    <th>coll p95 (s)</th>
    <th>coll max (s)</th>
  </tr></thead>
  <tbody>
  {summary_rows}
  </tbody>
</table>

<h2>Per-cell timeseries</h2>
{cell_blocks}

</body></html>
"""


def _svg_lineplot(
    title: str,
    xs: list[float],
    series: list[tuple[str, list[float], str]],  # (label, ys, color)
    width: int = 880,
    height: int = 220,
    y_pad: float = 1.0,
) -> str:
    """Return an inline-SVG line plot of one or more series sharing an x axis.

    Self-contained: no JS, no external assets. Times on x (rebased to 0 = first
    sample), seconds on y. y axis is auto-scaled to the union of all series
    with ``y_pad`` headroom. Empty input → a small 'no data' SVG.
    """
    if not xs or not series or all(not ys for _, ys, _ in series):
        return f'<svg width="{width}" height="40"><text x="10" y="24" class="scale">{title}: no data</text></svg>'

    x0 = xs[0]
    norm_xs = [x - x0 for x in xs]
    x_max = max(norm_xs) or 1.0
    all_ys: list[float] = []
    for _, ys, _ in series:
        all_ys.extend(ys)
    y_min = max(0.0, min(all_ys) - y_pad)
    y_max = max(all_ys) + y_pad
    if y_max - y_min < 1.0:
        y_max = y_min + 1.0

    pad_l, pad_r, pad_t, pad_b = 60, 12, 22, 30
    plot_w = width - pad_l - pad_r
    plot_h = height - pad_t - pad_b

    def sx(x: float) -> float:
        return pad_l + plot_w * (x / x_max)

    def sy(y: float) -> float:
        return pad_t + plot_h * (1 - (y - y_min) / (y_max - y_min))

    parts: list[str] = [f'<svg width="{width}" height="{height}" viewBox="0 0 {width} {height}">']
    parts.append(f'<text x="{pad_l}" y="14" class="scale">{title}</text>')
    # axes
    parts.append(f'<line class="axis" x1="{pad_l}" y1="{pad_t}" x2="{pad_l}" y2="{pad_t + plot_h}"/>')
    parts.append(f'<line class="axis" x1="{pad_l}" y1="{pad_t + plot_h}" x2="{pad_l + plot_w}" y2="{pad_t + plot_h}"/>')
    # y ticks (5)
    for i in range(5):
        t = i / 4.0
        yv = y_min + t * (y_max - y_min)
        y_px = pad_t + plot_h * (1 - t)
        parts.append(f'<line class="axis" x1="{pad_l - 4}" y1="{y_px}" x2="{pad_l}" y2="{y_px}"/>')
        parts.append(f'<text x="{pad_l - 8}" y="{y_px + 3}" class="scale" text-anchor="end">{yv:.1f}s</text>')
    # x ticks (4): rounded to seconds
    for i in range(4):
        t = i / 3.0
        xv = t * x_max
        x_px = pad_l + plot_w * t
        parts.append(f'<line class="axis" x1="{x_px}" y1="{pad_t + plot_h}" x2="{x_px}" y2="{pad_t + plot_h + 4}"/>')
        parts.append(f'<text x="{x_px}" y="{pad_t + plot_h + 18}" class="scale" text-anchor="middle">+{int(xv)}s</text>')
    # series polylines
    for label, ys, color in series:
        if not ys:
            continue
        pts = " ".join(f"{sx(norm_xs[i]):.1f},{sy(ys[i]):.1f}" for i in range(len(ys)))
        parts.append(f'<polyline fill="none" stroke="{color}" stroke-width="1.6" points="{pts}"/>')
    # legend
    legend_x = pad_l
    legend_y = height - 8
    legend_items = []
    for label, _, color in series:
        legend_items.append(
            f'<span><i style="background:{color}"></i>{label}</span>'
        )
    parts.append('</svg>')
    return "".join(parts) + f'<div class="legend">{"".join(legend_items)}</div>'


def _read_csv(path: pathlib.Path) -> list[dict]:
    with path.open() as f:
        return list(csv.DictReader(f))


def _series_avg(rows: list[dict], col: str) -> float:
    """Average of a float column across rows; 0 for empty input."""
    if not rows:
        return 0.0
    return sum(float(r[col]) for r in rows) / len(rows)


def cmd_report(args: argparse.Namespace) -> None:
    """Render a sweep manifest + its per-cell CSVs into a standalone HTML file."""
    manifest_path = pathlib.Path(args.manifest)
    manifest = json.loads(manifest_path.read_text())
    runs_dir = manifest_path.parent

    summary_rows: list[str] = []
    cell_blocks: list[str] = []
    for cell in manifest["cells"]:
        csv_path = runs_dir / cell["csv"]
        rows = _read_csv(csv_path) if csv_path.exists() else []
        manif_p50 = _series_avg(rows, "manif_gap_p50_s")
        manif_p95 = _series_avg(rows, "manif_gap_p95_s")
        manif_max = max((float(r["manif_gap_max_s"]) for r in rows), default=0.0)
        coll_p50 = _series_avg(rows, "coll_gap_p50_s")
        coll_p95 = _series_avg(rows, "coll_gap_p95_s")
        coll_max = max((float(r["coll_gap_max_s"]) for r in rows), default=0.0)
        workers_per_runner = cell["check_runners"] if cell["check_runners"] != "default" else "default(4)"
        summary_rows.append(
            "<tr>"
            f"<td class='left'><code>{cell['label']}</code></td>"
            f"<td>{cell['prom_endpoints']}</td>"
            f"<td>{cell['workload_deployments']}</td>"
            f"<td>{cell['workload_crs']}</td>"
            f"<td>{cell['clcr_replicas']}</td>"
            f"<td>{workers_per_runner}</td>"
            f"<td class='left'>{'✓' if cell['ootb_crds'] else ''}</td>"
            f"<td>{manif_p50:.2f}</td><td>{manif_p95:.2f}</td><td>{manif_max:.2f}</td>"
            f"<td>{coll_p50:.2f}</td><td>{coll_p95:.2f}</td><td>{coll_max:.2f}</td>"
            "</tr>"
        )

        # Per-cell SVG. x = sample timestamp (seconds since first sample),
        # y = gap (seconds). Two series: manif and coll, each as a triplet of
        # p50/p95/max.
        xs = [float(r["timestamp"]) for r in rows]
        manif_p50_s = [float(r["manif_gap_p50_s"]) for r in rows]
        manif_p95_s = [float(r["manif_gap_p95_s"]) for r in rows]
        manif_max_s = [float(r["manif_gap_max_s"]) for r in rows]
        coll_p50_s = [float(r["coll_gap_p50_s"]) for r in rows]
        coll_p95_s = [float(r["coll_gap_p95_s"]) for r in rows]
        coll_max_s = [float(r["coll_gap_max_s"]) for r in rows]
        manif_svg = _svg_lineplot(
            f"{cell['label']}: manifest pipeline gap (s)",
            xs,
            [
                ("manif p50", manif_p50_s, "#3b6fb0"),
                ("manif p95", manif_p95_s, "#7eb1e0"),
                ("manif max", manif_max_s, "#b1d5f1"),
            ],
        )
        coll_svg = _svg_lineplot(
            f"{cell['label']}: collector pipeline gap (s)",
            xs,
            [
                ("coll p50", coll_p50_s, "#b04a3b"),
                ("coll p95", coll_p95_s, "#e08a7e"),
                ("coll max", coll_max_s, "#f1c1b1"),
            ],
        )
        cell_blocks.append(
            f"<div class='cell-block'>"
            f"<h3><code>{cell['label']}</code> &mdash; "
            f"prom_endpoints={cell['prom_endpoints']}, "
            f"workload_deps={cell['workload_deployments']}, "
            f"workload_crs={cell['workload_crs']}, "
            f"clcr_replicas={cell['clcr_replicas']}, "
            f"DD_CHECK_RUNNERS={workers_per_runner}"
            f"</h3>"
            f"{manif_svg}"
            f"{coll_svg}"
            f"</div>"
        )

    html = REPORT_HTML_TEMPLATE.format(
        sweep_id=manifest["sweep_id"],
        agent_image=manifest["agent_image"],
        cluster_agent_image=manifest["cluster_agent_image"],
        window_s=manifest["window_seconds"],
        interval_s=manifest["observe_interval_seconds"],
        duration_s=manifest["observe_duration_seconds"],
        settle_s=manifest["settle_seconds"],
        summary_rows="\n".join(summary_rows),
        cell_blocks="\n".join(cell_blocks),
    )
    out_path = pathlib.Path(args.output)
    out_path.write_text(html)
    log(f"wrote {out_path}")


# ----------------------------------------------------------------------------
# Helpers for observe / sweep
# ----------------------------------------------------------------------------


def parse_duration(s: str) -> int:
    """Parse a duration like '5m', '30s', '1h30m', or a bare integer (seconds)."""
    if s is None:
        return 0
    if s.isdigit():
        return int(s)
    total = 0
    for match in re.finditer(r"(\d+)([smhd])", s):
        n, unit = int(match.group(1)), match.group(2)
        total += n * {"s": 1, "m": 60, "h": 3600, "d": 86400}[unit]
    if total == 0:
        die(f"could not parse duration '{s}'")
    return total



# ----------------------------------------------------------------------------
# argparse
# ----------------------------------------------------------------------------


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(description=__doc__)
    sub = p.add_subparsers(dest="cmd", required=True)

    def add_common_image_flags(sp: argparse.ArgumentParser) -> None:
        sp.add_argument("--agent-image", default=AGENT_IMAGE_DEFAULT)
        sp.add_argument("--cluster-agent-image", default=CLUSTER_AGENT_IMAGE_DEFAULT)
        sp.add_argument("--fakeintake-image", default=FAKEINTAKE_IMAGE_DEFAULT)
        sp.add_argument("--operator-version", default=OPERATOR_APP_VERSION_DEFAULT)

    up = sub.add_parser("up", help="bring up the lab")
    add_common_image_flags(up)
    up.add_argument("--prom-endpoints", type=int, default=0)
    up.add_argument("--workload-pods", type=int, default=0,
                    help="replicas of a single 'filler' Deployment in the 'workload' ns. "
                         "Stresses the *node-agent* 'orchestrator_pod' check (which talks "
                         "to the kubelet for per-node Pod collection); does NOT inflate the "
                         "CLC-runner orchestrator check, because Deployment count stays at 1. "
                         "For CLC-side scaling use --workload-deployments / --workload-crs.")
    up.add_argument("--workload-deployments", type=int, default=0,
                    help="N standalone tiny Deployments (each 1 replica). Each is 3 "
                         "cluster-scoped objects (Deployment+ReplicaSet+Pod) for the CLC "
                         "orchestrator check to walk on every Run().")
    up.add_argument("--workload-replicas", type=int, default=1,
                    help="replicas per --workload-deployments Deployment. Set to 0 to scale "
                         "past kind's ~2000-pod node ceiling — the orchestrator check still "
                         "walks N Deployments + N ReplicaSets in its informer caches, just "
                         "without backing pods.")
    up.add_argument("--workload-crs", type=int, default=0,
                    help="N Flux GitRepository CRs (no controller — just stored objects) "
                         "to populate the orchestrator-check's CR informer cache. Only "
                         "effective with --ootb-crds.")
    up.add_argument("--check-runners", default="default",
                    help="'default' (4, no override) or an integer override for DD_CHECK_RUNNERS")
    up.add_argument("--clcr-replicas", type=int, default=2)
    up.add_argument("--ootb-crds", action="store_true",
                    help="enable DD_ORCHESTRATOR_EXPLORER_CUSTOM_RESOURCES_OOTB_ENABLED on "
                         "cluster-agent + CLCRs AND install the corresponding 13 CRDs "
                         "(Argo, Flux, Karpenter) so the builtin CR collectors actually "
                         "wire up. Without the CRDs the env var alone is a no-op on kind.")
    up.set_defaults(func=cmd_up)

    obs = sub.add_parser("observe", help="stream K8sCluster manifest counts from fakeintake")
    obs.add_argument("--window", default="5m")
    obs.add_argument("--interval", default="30s")
    obs.add_argument("--duration", default=None, help="stop after this much wall time")
    obs.set_defaults(func=cmd_observe)

    sw = sub.add_parser("sweep", help="walk a matrix of cells and capture one observe CSV per cell")
    add_common_image_flags(sw)
    sw.add_argument("--scenario", default="default",
                    choices=tuple(SWEEP_SCENARIOS),
                    help="which pre-defined matrix to run (default: %(default)s). "
                         "'knee' scales prom-endpoints from 0 to 3000 at production-default workers.")
    sw.add_argument("--runs-dir", default=str(pathlib.Path(__file__).parent / "runs"),
                    help="where to write per-cell CSVs and the sweep manifest")
    sw.add_argument("--settle", default="90s",
                    help="how long to wait after applying a cell before observing")
    sw.add_argument("--window", default="3m",
                    help="observe window (rolling) per sample")
    sw.add_argument("--observe-interval", default="20s",
                    help="observe sampling interval")
    sw.add_argument("--observe-duration", default="3m",
                    help="how long to observe each cell after settling")
    sw.set_defaults(func=cmd_sweep)

    rep = sub.add_parser("report", help="render a sweep manifest into a standalone HTML report")
    rep.add_argument("--manifest", required=True,
                     help="path to a sweep-<ts>-manifest.json produced by 'sweep'")
    rep.add_argument("--output", default="report.html",
                     help="path to write the standalone HTML report")
    rep.set_defaults(func=cmd_report)

    dn = sub.add_parser("down", help="tear down the kind cluster")
    dn.set_defaults(func=cmd_down)

    return p


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
