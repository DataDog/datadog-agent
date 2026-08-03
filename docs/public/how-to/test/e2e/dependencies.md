# Test dependencies

E2E tests run on real VMs and clusters. Anything a test fetches from the public internet at test time is a dependency on infrastructure nobody at Datadog controls — DockerHub throttles, distro mirrors go down, and `apt install <pkg>` quietly installs something different six months from now. CI will also lose outbound internet access.

/// info
The **rules** — what is allowed, in what order of preference, and the requirement to pin versions — are in <<<repo("test/new-e2e/codereview_guideline.md", match="^### Avoiding external dependencies$")>>>, which is what review checks a PR against. It also carries the checklist for <<<repo("test/new-e2e/codereview_guideline.md", "spotting a runtime dependency", match="^#### Spotting a runtime dependency$")>>> in a diff.

This page is the *how*: the mechanisms behind each alternative, the commands, and the parts that bite.
///

## Where each kind of dependency goes

| What you need | Mechanism |
|---|---|
| A public container image | [ECR pull-through cache](#ecr-pull-through-cache) |
| A CLI tool or system package on the VM | [Prebake it into the machine image](#prebake-into-the-machine-image) |
| A third-party binary, installer, or tarball | [S3 artifact bucket](#s3-artifact-bucket) |
| A tool that ships as a container image | Run it from the cache rather than installing it on the host |
| Something that genuinely cannot be prebaked yet | [Quarantine the install](#when-you-genuinely-cannot-prebake-yet) |

## ECR pull-through cache

/// info
It is a **cache, not a mirror**. There is no "request that an image be added" process — the first pull of a tag populates it transparently. Rewriting the reference is the whole task.
///

--8<-- "test/new-e2e/codereview_guideline.md:registries"

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

### Kubernetes and Kind

Two different pulls happen, and only one of them is solved:

- **The Kind node image** is proxied. `components/kubernetes/kind.go` builds it from `env.InternalDockerhubMirror()`, which resolves to the pull-through cache on AWS and falls back to `registry-1.docker.io` elsewhere. Use that helper rather than writing the registry host yourself.
- **Images pulled by workloads inside the cluster** are not. The containerd host config the framework installs (`components/kubernetes/hosts.toml`) points `docker.io` at `mirror.gcr.io`, because giving in-cluster containerd credentials for a private ECR registry is unsolved.

So inside a Kind cluster, prefer the cache where you control the reference — and expect the remaining `docker.io` path to still leave the VPC. If your test depends on that, say so in the test rather than assuming the mirror is internal.

### Running such a test locally

Kubernetes workloads need credentials for the cache, which the local runner does not set by default:

```bash
dda inv new-e2e-tests.run --targets ./tests/gpu \
  -c ddagent:imagePullRegistry=669783387624.dkr.ecr.us-east-1.amazonaws.com \
  -c ddagent:imagePullUsername=AWS \
  -c ddagent:imagePullPassword=$(aws-vault exec sso-agent-qa-read-only -- aws ecr get-login-password)
```

If you see `User: arn:aws:sts::… is not authorized to perform: ecr:BatchGetImage`, you are authenticated against the wrong account — re-run `aws-vault login sso-agent-sandbox-account-admin-8h`. Note that those `ddagent:imagePull*` flags are for the in-cluster Agent; an EC2 host pulls with its own instance profile.

### Outside AWS

`InternalDockerhubMirror()` resolves to the cache on AWS and falls back to `registry-1.docker.io` in GCP, Azure and locally, so framework code that uses the helper degrades gracefully. Test code that hardcodes a cache URL does not.

## S3 artifact bucket

For a third-party artifact that is not a container image and cannot be prebaked — a profiler, a JDK, a debugging tool — vendor it into `s3://agent-e2e-s3-bucket` and pull it from there. Hosts authenticate with their instance profile, so no credentials are involved in the test.

```go
err := v.Env().RemoteHost.HostArtifactClient.Get("toto.txt", "toto.txt")
```

`test/new-e2e/examples/vmenv_artifacts_test.go` is the runnable example. Objects are namespaced by owning team — `windows-products/xperf-5.0.8169.zip`, `processes/DiskSpd.zip` — and the bucket URL is prepended for you, so the path in `Get` is the key, not a URL.

Pin the version in the key (`xperf-5.0.8169.zip`, not `xperf.zip`) so a re-upload can never change what an existing test downloads, and record where the artifact came from when you request the upload. Some existing objects have no recorded upstream or version, which makes them impossible to reproduce or audit — don't add to that.

/// info
`HostArtifactClient` is implemented for AWS hosts only — the supported Linux flavors and Windows Server. On Azure and GCP it returns `not implemented`.
///

Uploading is **not self-service**: write access is held by `agent-qa` admins, because IAM cannot express "this team may write only under its own prefix" at the granularity we would want. Ask #agent-devx-help with the artifact, its upstream URL, its version, and the key you want it at. See [E2E - Use a third party artifact in test](https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/5040342019/E2E+-+Use+a+third+party+artifact+in+test) on Confluence for the current inventory.

## Prebake into the machine image

If a test needs a tool present on a bare VM, the right place for it is the machine image, not the test. The `-e2e` AMI variants exist for exactly this and are already the default for Linux — `UbuntuDefault` is `Ubuntu2204E2E`, not `Ubuntu2204`.

Check what is already there before adding anything. Every Linux `-e2e` image ships Docker, `docker-compose`, `amazon-ecr-credential-helper`, the AWS CLI v2, `jq`, `python3` with `pip`, `ansible` with the `datadog.dd` collection pre-cached, and an `ab` client (`apache2-utils` / `httpd-tools`). Ubuntu additionally has `php`, `stress`, and Node.js 20; RHEL has `fapolicyd`.

See [Custom AMIs](custom-amis.md) for how images are selected and how to get something new baked in.

## When you genuinely cannot prebake yet

Some matrices are hard to prebake — the GPU suite runs on NVIDIA driver images that are bare Ubuntu plus CUDA, with no `-e2e` variant yet. The accepted pattern there is to **quarantine, not scatter**: `test/new-e2e/tests/gpu/runtime_installs.go` holds every runtime install for that suite in one obviously-named file, with a header stating why the file exists, what will replace it, and the tracking ticket. Its call sites are individually commented.

Do the same if you have no other option. One file with a ticket in its header is trackable; three `Execute("apt-get install …")` calls scattered across a provisioner are not.

The related shape for clouds without prebaked images is an explicit opt-in helper rather than an implicit install. `test/e2e-framework/components/docker/{InstallDocker,InstallCompose}` are exactly that: no-ops nobody calls on AWS, called deliberately by the Azure and GCP provisioners.
