# KAI Scheduler fractional-GPU fixtures

Real Kubernetes objects captured from a cluster running KAI Scheduler with
fractional GPUs, for use as test fixtures by the KAI support work (AXT-94).

Captured for AXT-107. No agent code changes were made to produce these.

## Environment

| Component | Version |
| --- | --- |
| KAI Scheduler | v0.18.5 |
| KAI GPU sharing mode | `NonMemoryEnforced` (chart default) |
| Kubernetes | v1.34.12 (kubeadm, single node) |
| containerd | 2.4.1 |
| NVIDIA GPU Operator | v26.7.1 |
| NVIDIA driver | 595.91.07 (CUDA 13.2) |
| Datadog Agent | 7.84.2 (chart datadog-3.254.1) |
| Node | 4x NVIDIA Tesla T4, 15360 MiB each |

KAI's `gpusharing` plugin was enabled with `cdiEnabled: "true"`; `hamicore` and
`nvfractions` were disabled.

## Ground truth: pod -> GPU UUID

Derived independently from `nvidia-smi -L` run inside each pod, and confirmed to
match the `runai-gpu-group` -> reservation pod -> `run.ai/reserve_for_gpu_index`
chain for every pod.

| Pod | `gpu-fraction` | GPU UUID | GPU index |
| --- | --- | --- | --- |
| `frac-a` | 0.5 | `GPU-0b5693b2-d023-de74-f5c9-bb2c1cad110d` | 2 |
| `frac-b` | 0.5 | `GPU-0b5693b2-d023-de74-f5c9-bb2c1cad110d` | 2 |
| `frac-c` | 0.7 | `GPU-32a93c9b-38dc-933a-192a-58a0a2f92a19` | 0 |
| `frac-cuda` | 0.5 | `GPU-be38b139-9eb5-692d-593c-b1041f029e26` | 3 |
| `frac-multi` | 0.5 x 2 devices | `GPU-be38b139-9eb5-692d-593c-b1041f029e26`, `GPU-d7bea0fc-e039-0702-46b6-97510ff81958` | 3, 1 |

`frac-a` and `frac-b` share GPU index 2. `frac-cuda` shares GPU index 3 with
`frac-multi`, which also holds half of GPU index 1. `frac-cuda` is the only pod
that executes CUDA (the DataDog `apps-cuda-basic` image in a loop); the others
just sleep, so they generate no GPU activity at all.

### Reservation pods

| Reservation pod | `runai-gpu-group` | `run.ai/reserve_for_gpu_index` |
| --- | --- | --- |
| `gpu-reservation-6d8e72c077c10ec9` | `c3ced09c-c7e0-4b95-9ba7-59fc43afa1e8` | `GPU-0b5693b2-d023-de74-f5c9-bb2c1cad110d` |
| `gpu-reservation-757d4ae74ae8a0f2` | `0350dd42-d924-4f06-b897-3be2da6998e1` | `GPU-32a93c9b-38dc-933a-192a-58a0a2f92a19` |
| `gpu-reservation-c39b814206ba355f` | `0247250f-e9cb-4445-8a61-23ffd7e6fd53` | `GPU-d7bea0fc-e039-0702-46b6-97510ff81958` |
| `gpu-reservation-1e6c83a8ebe11f36` | `4c5ff560-a6ca-4f10-820c-e183cad56ea2` | `GPU-be38b139-9eb5-692d-593c-b1041f029e26` |

## What the agent reports today

Captured with agent 7.84.2, before any KAI support exists.

- **Fractional pods** have an empty `Allocated Resources` and no `GPUVendor`.
  They do not request `nvidia.com/gpu`, so nothing appears for them in the
  kubelet PodResources API, and `containers.HasGPUs` returns false for them.
- **Reservation pods** carry `GPUVendor: [nvidia]` and
  `Allocated Resources: nvidia.com/gpu, ID: <UUID>`, so device metrics are
  attributed to the reservation pod rather than to the real workload.
- Process-level metrics (`gpu.process.*`) are attributed correctly, with
  `pod_name`, `kube_namespace` and the right `gpu_uuid`, but only for pods that
  actually execute CUDA.

## Files

| File | Contents |
| --- | --- |
| `kubelet-pods.json` | `/api/v1/nodes/<node>/proxy/pods`, trimmed to the `kai-test` and `kai-resource-reservation` namespaces. This is the JSON the agent parses. |
| `pods-kai-test.yaml` | The five fractional pods. |
| `pods-kai-resource-reservation.yaml` | The four reservation pods created by KAI's binder. |
| `bindrequests.yaml` | `bindrequests.scheduling.run.ai` |
| `podgroups.yaml` | `podgroups.scheduling.run.ai` |

Service-account token volumes, `imagePullSecrets` and `managedFields` were
removed. The only remaining matches for "secret" are the standard
`/var/run/secrets/kubernetes.io/serviceaccount` mount paths.

## Reproducing

```bash
helm upgrade -i kai-scheduler oci://ghcr.io/kai-scheduler/kai-scheduler/kai-scheduler \
  -n kai-scheduler --create-namespace --version v0.18.5
```

Then apply pods with `schedulerName: kai-scheduler`, the label
`kai.scheduler/queue: default-queue` and a `gpu-fraction` annotation. KAI's
binder creates the matching reservation pods automatically.
