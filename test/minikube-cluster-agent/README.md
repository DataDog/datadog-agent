# Minikube cluster agent test (with secret-generic-connector)

This directory contains manifests and config to run the Datadog cluster agent (and secret-generic-connector) in minikube against a fake intake.

## Prerequisites

- minikube
- kubectl
- Docker (minikube docker-env)
- `dda` and project venv: `source ~/venv/bin/activate`

## Architecture note (Mac / Apple Silicon)

If you are on **macOS (darwin)** and minikube is **linux/arm64**, the binaries produced by `dda inv cluster-agent.build` are **darwin/arm64**. The container runs **linux/arm64**, so you get `exec format error` unless you use **Linux** binaries.

Options:

1. **Run the test on a Linux host or in CI** (recommended): build and run there so binaries match the container (linux/amd64 or linux/arm64).
2. **Build for Linux inside Docker**: use a Linux build image to produce `bin/datadog-cluster-agent/datadog-cluster-agent` and `bin/secret-generic-connector/secret-generic-connector` for the target arch, then build the image and deploy as below.
3. **Use amd64 minikube on Mac**: e.g. `minikube start --arch=amd64` and build with `GOOS=linux GOARCH=amd64` (cross-compile may hit dependency issues).

## Steps

1. **Start minikube**

   ```bash
   minikube start
   eval $(minikube docker-env)
   ```

2. **Build cluster agent (and secret-generic-connector)**

   From the **repo root**:

   ```bash
   source ~/venv/bin/activate
   dda inv cluster-agent.build --skip-assets
   ```

3. **Build the Docker image**

   From the **repo root** (build context must be repo root so `bin/` and `test/minikube-cluster-agent/` are available):

   ```bash
   eval $(minikube docker-env)
   docker build -f test/minikube-cluster-agent/Dockerfile -t zork:latest .
   ```

4. **Deploy fake intake, secrets, and cluster agent**

   ```bash
   kubectl apply -f test/minikube-cluster-agent/fake-intake.yaml
   kubectl apply -f test/minikube-cluster-agent/secrets.yaml
   kubectl apply -f test/minikube-cluster-agent/cluster-agent-pod.yaml
   ```

5. **Verify**

   - Pod running: `kubectl get pod cluster-agent-test`
   - Logs: `kubectl logs cluster-agent-test -c agent -f`
   - Secret-generic-connector in image:  
     `kubectl exec cluster-agent-test -c agent -- /opt/datadog-agent/bin/secret-generic-connector --version`

## Files

- `datadog.yaml` – cluster agent config (secrets via `secret_backend_command`, fake intake)
- `Dockerfile` – image that includes cluster agent + secret-generic-connector
- `fake-intake.yaml` – fake-datadog Pod and Service
- `secrets.yaml` – `agent-secrets` Secret (mainkee, kee1)
- `cluster-agent-pod.yaml` – cluster agent Pod + RBAC (Role, RoleBinding, ConfigMap)
