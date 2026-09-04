# Test dependencies

E2E tests run on real VMs and clusters. Anything a test fetches from the public internet at test time is a dependency on infrastructure we don't and can't control.
This is one of the biggest source of unreliability in e2e tests: DockerHub will rate-limit us, distro package mirrors often go down down, and `apt install <pkg>` might quietly install a different version six months from now.

--8<-- "test/new-e2e/codereview_guideline.md:avoiding-external-deps"

/// info
The rules above — what is allowed, in what order of preference, and the requirement to pin versions — are what review checks a PR against, straight from <<<repo("test/new-e2e/codereview_guideline.md", "the guideline")>>>. This page is the *how*: the mechanisms behind each alternative, the commands, and the parts that bite.
///

## Common ways to remove external deps

| Type of dep | Preferred mechanism |
|---|---|
| A public container image | [ECR pull-through cache](#ecr-pull-through-cache) |
| A CLI tool or system package on the VM | [Prebake it into the machine image](#prebake-into-the-machine-image) |
| A third-party binary, installer, or tarball | [S3 artifact bucket](#s3-artifact-bucket) |

/// warning
Most of these methods only work for tests that run on **AWS** - if your test runs on Azure or GCP, [your options will be more limited.](#azure-and-gcp)
///

## ECR pull-through cache

/// info
It is a **cache, not a mirror**. There is no "request that an image be added" process — the first pull of a tag populates it transparently.
///

--8<-- "test/new-e2e/codereview_guideline.md:registries"

/// details | Avoid hardcoding the registry adress
    type: tip
    open: False

Don't hardcode the registry host. Read it from the runner parameter store, so the same test works in a different account:

```go
reg, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
if reg != "" {
    busyboxImage = strings.SplitN(reg, ",", 2)[0] + "/dockerhub/library/busybox:1.37.0"
}
```

The `strings.SplitN(reg, ",", 2)[0]` is not decoration: the parameter may hold a comma-separated list, and only the first entry is a usable host. Inside a Pulumi component the same value is on the environment: `e.ImagePullRegistry()`.

Real examples to copy from:

- `test/new-e2e/tests/containers/docker_test.go` — DockerHub, from a test
- `test/new-e2e/tests/agent-platform/tests/install_script_test.go` — `ecr-public`
- `test/e2e-framework/components/datadog/apps/etcd/k8s.go` — `quay`, from a Pulumi component
- `test/new-e2e/tests/ndm/snmp/compose/snmpCompose.yaml` — a compose file with a rewritten `image:`
- `test/new-e2e/tests/installer/host/host.go` — the mirrored-with-upstream-fallback pair, for when you need both forms
///

### Kubernetes and Kind

Two different pulls happen, with two different defaults:

- **The Kind node image** (the VM image the cluster itself boots from) is proxied through the ECR cache by default. `components/kubernetes/kind.go` builds its reference from `env.InternalDockerhubMirror()`, which resolves to the cache on AWS and falls back to `registry-1.docker.io` elsewhere. Use that helper rather than writing the registry host yourself.
- **Images pulled by workloads *inside* the cluster** (your test's pods) are **not** proxied by default. The containerd host config the framework installs (`components/kubernetes/hosts.toml`) only redirects the bare `docker.io` host to `mirror.gcr.io` — a public, unauthenticated mirror, not the ECR cache — because giving every pod containerd credentials for a private registry isn't solved cluster-wide.

A workload *can* pull through the authenticated ECR cache, but it takes two steps together, both opt-in: rewrite the pod's image reference to point at the cache prefix (same idiom as the parameter-store snippet above), and set the `imagePullRegistry`/`imagePullUsername`/`imagePullPassword` runner parameters so the framework attaches a matching `ImagePullSecret` to the pod (`common/utils/kubernetes.go`, `NewImagePullSecret`). Do one without the other and the pull fails: a rewritten reference with no secret can't authenticate, and a secret with no rewritten reference has nothing to authenticate against.

/// details | Example
   type: info
   open: False
`test/new-e2e/tests/gpu/gpu_test.go`, for example, does both:

```go
var dockerRegistry = func() string {
	reg, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
	if reg != "" {
		return strings.SplitN(reg, ",", 2)[0] + "/dockerhub"
	}
	return "docker.io"
}()

var cuda12DockerImage = fmt.Sprintf("%s/nvidia/cuda:%s-base-ubuntu22.04", dockerRegistry, defaultCudaVersion)
```

The `ImagePullSecret` half is handled separately, by the framework, once `e.ImagePullRegistry()` is non-empty — see the flags this test needs locally right below.

If your test doesn't opt in, its workload images still leave the VPC through `mirror.gcr.io` — say so in the test rather than assuming that path is fully internal.
///

/// details | Running a k8s-based test locally 
    type: tip
    open: False

Kubernetes workloads need credentials for the cache, which are not set by default:

```bash
dda inv new-e2e-tests.run --targets ./tests/gpu \
  -c ddagent:imagePullRegistry=669783387624.dkr.ecr.us-east-1.amazonaws.com \
  -c ddagent:imagePullUsername=AWS \
  -c ddagent:imagePullPassword=$(aws-vault exec sso-agent-qa-read-only -- aws ecr get-login-password)
```

If you see `User: arn:aws:sts::… is not authorized to perform: ecr:BatchGetImage`, you are authenticated against the wrong account — re-run `aws-vault login sso-agent-sandbox-account-admin-8h`. Note that those `ddagent:imagePull*` flags are for the in-cluster Agent; an EC2 host pulls with its own instance profile.
///

## Prebake into the machine image

If a test needs a specific tool or system package in a VM, avoid installing it at test time at all costs.
The better alternative is to:

- Use a Docker image containing that tool that you can then pull the [ECR pull-through cache](#ecr-pull-through-cache).
- "Bake" the dependency into the VM machine image (AMI).

We already provide `-e2e` AMI variants for most OSes for this purpose, that contain some common dependencies: docker itself, `jq`, `python3` etc.
These `-e2e` variants are already the default for tests running on AWS.

See [Custom AMIs](custom-amis.md) for more details on how to do this.

## S3 artifact bucket

For a more miscellaneous third-party artifact, vendor it into `s3://agent-e2e-s3-bucket` and pull it from there. Hosts authenticate with their instance profile, so no credentials are involved in the test.

```go
err := v.Env().RemoteHost.HostArtifactClient.Get("toto.txt", "toto.txt")
```

`test/new-e2e/examples/vmenv_artifacts_test.go` is the runnable example. Objects are namespaced by owning team — `windows-products/xperf-5.0.8169.zip`, `processes/DiskSpd.zip` — and the bucket URL is prepended for you, so the path in `Get` is the key, not a URL.

/// tip
Pin the version in the key (`xperf-5.0.8169.zip`, not `xperf.zip`) so a re-upload can never change what an existing test downloads, and record where the artifact came from when you request the upload.

Some existing objects have no recorded upstream or version, which makes them impossible to reproduce or audit — this is a pattern to avoid.
///

/// info
`HostArtifactClient` is implemented for AWS hosts only — the supported Linux flavors and Windows Server. On Azure and GCP it returns `not implemented`.
///

/// warning
Uploading is **not self-service**: write access is held by `agent-qa` admins, because IAM cannot express "this team may write only under its own prefix" at the granularity we would want. Ask #agent-devx-help with the artifact, its upstream URL, its version, and the key you want it at. See [E2E - Use a third party artifact in test](https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/5040342019/E2E+-+Use+a+third+party+artifact+in+test) on Confluence for the current inventory.
///

## Azure and GCP

None of the three mechanisms above exist on Azure or GCP:

- **Container images**: no pull-through cache (`InternalDockerhubMirror()` falls back to plain `registry-1.docker.io`). Use the most reliable mirror available instead — `mirror.gcr.io` on GCP; Azure has no blessed equivalent, ask #agent-devx-help.
- **Prebaked images**: no `-e2e` AMI equivalent. `docker.InstallDocker` / `docker.InstallCompose` (`test/e2e-framework/components/docker/install.go`) install Docker at provision time as the accepted exception for these clouds.
- **S3 artifact bucket**: `HostArtifactClient` returns `not implemented` (see above), with no fallback.
