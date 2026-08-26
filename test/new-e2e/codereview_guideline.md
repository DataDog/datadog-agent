# E2E Test writing guidelines
These guidelines are in rough "importance" order, but follow _all_ of them.

## Making tests reliable
### Avoiding external dependencies
<!-- --8<-- [start:avoiding-external-deps] -->
A major source of flaky / unrelated failures is reliance on _external dependencies_: anything outside the AWS/GCP/Azure account the test runs in and internal DD systems.

We will soon block _all_ internet access from CI for security reasons, so prepare now.

#### Spotting a runtime dependency
| Smell | Example |
|---|---|
| A package manager on the host | `vm.Execute("sudo apt-get install -y jq")`, `yum install`, `zypper`, `choco` |
| A download from a public host | `curl https://…`, `wget`, `Invoke-WebRequest`, `msiexec /i https://…` |
| An image reference with no registry | `docker run busybox`, `FROM ubuntu:22.04`, `image: redis` |
| A language package manager | `pip install`, `npm i`, `gem install`, `cargo install` |
| A remotely-hosted manifest | `kubectl apply -f https://…`, a Helm `repository:` URL, a remote kustomize base |
<!-- --8<-- [end:avoiding-external-deps] -->

#### Docker image pulls
It is easy to "accidentally" pull images from `docker.io` / DockerHub, a major source of flakiness due to its restrictive rate limiting.
Watch out for:
- `docker run ...` without specifying a registry
- In a Dockerfile, `FROM ...` without specifying a registry
- In a compose file / k8s definition: `image: ...` without specifying a registry.

<!-- --8<-- [start:registries] -->
Use the ECR pull-through cache set up in the `datadog-agent-qa` account (`669783387624`, `us-east-1`):

| Upstream | Prefix |
|---|---|
| DockerHub | `669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/…` |
| Public ECR | `669783387624.dkr.ecr.us-east-1.amazonaws.com/ecr-public/…` |
| Quay | `669783387624.dkr.ecr.us-east-1.amazonaws.com/quay/…` |

- DockerHub official images keep their `library/` path: `busybox:1.37.0` becomes `…/dockerhub/library/busybox:1.37.0`.
- Only those three upstreams are supported. **GHCR is not** — if your image is only on GHCR, use an equivalent from a supported upstream if possible, otherwise leave unproxied.
- Pulling from `public.ecr.aws/…` or `mirror.gcr.io` **directly is not an approved alternative**, even though neither is rate limited today. Route it through the prefixes above. Existing code that does otherwise is debt, not precedent.
- There is no pull-through cache in GCP or Azure. For a test that only ever runs on GCP, the provider's registry or `mirror.gcr.io` is acceptable — do not import that habit into an AWS test. Ask #agent-devx-help before relying on any other public mirror.
<!-- --8<-- [end:registries] -->

Do not hardcode the registry host; read it from the runner parameter store. See [ECR pull-through cache](../../docs/public/how-to/test/e2e/dependencies.md#ecr-pull-through-cache) for the idiom, the Kind caveat, and how to run such a test locally.

#### System package installs (apt, yum, dnf, zypper, ...)
If your test requires a package unavailable on a bare VM, in order of preference:
- Avoid it (e.g. you rarely need `jq`: parse JSON in go)
- Use a containerized environment cached via the previous method. Many tools ship prebuilt container images you can run directly.
- Prebake it into a custom machine image, as done for the `Ubuntu2204E2E` OS flavor. See [Custom AMIs](../../docs/public/how-to/test/e2e/custom-amis.md), which also lists what the `-e2e` images already ship.
- Store your package installer on an internal package repository. See [Other dependencies](#other-dependencies)

Running package managers on the VM exposes you to rate limiting from upstream mirrors and to _changes_ in their packages - removed, renamed, or incompatible versions. Also see [Pin your dependencies](#pin-your-dependencies).

#### Language package installs (pip, npm, gem, cargo, ...)
These pull from their own public registries and are subject to the same rate limiting and drift risks as system package installs. The same alternatives apply.

#### Other dependencies / Internet accesses
Avoid web requests to external websites (`ping some-website.com`, `curl some-website.com`). If you must download a tarball, installer, or package and no previous solution applies, vendor that artifact in our purpose-made S3 bucket via `RemoteHost.HostArtifactClient`. See [S3 artifact bucket](../../docs/public/how-to/test/e2e/dependencies.md#s3-artifact-bucket) for the read API, and [Confluence](https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/5040342019/E2E+-+Use+a+third+party+artifact+in+test) for the upload side.

Remotely-hosted Kubernetes resources (Helm charts, CNI manifests like flannel, remote kustomize bases...) are a common hidden source of Internet access - both the manifest and the images it references are pulled at runtime. Vendor the manifest locally and rewrite its image references to the ECR pull-through cache.

### Pin your dependencies
When depending on something external, pin both the version and a sha256sum to avoid hard-to-track behavior changes which can cause unexpected breakages.

Unpinned dependencies sneak in from unexpected places. `apt install <package>` and similar commands install the latest version of a package.
> Worse, upstream mirrors often don't keep all past versions of all packages.

A Docker image without a tag defaults to `latest`. Dependencies pulled via a hardcoded `curl ...` need a pinned version in the URL fragment. Remotely-hosted Kubernetes resources (Helm charts, CNI manifests like flannel...) referenced by a branch URL rather than a pinned tag/version are unpinned in the same way.

### Think about timing
E2E tests run in real-time on real infra, so they hit timing issues like integration tests or frontend playwright tests.
Never synchronously assert a property that may take time to become true. To check a payload reached the fakeintake, use `EventuallyWithT` rather than a direct synchronous check.

### Cleanup after yourself
Write tests so they can be retried _without reprovisioning the host_: clean up generated artifacts, revert temporary changes, etc.

Our custom retry logic retries tests on _the same infra_ when possible, making retries faster and more reliable. Otherwise it falls back to an expensive "full test retry" that reprovisions the infra, costing extra time and reliability.

## Using the framework
The framework in `test/e2e-framework` handles and abstracts most provisioning and setup logic.
When writing a new test, ALWAYS check whether a framework helper already handles the provisioning.
> common examples:
> - creating a VM with the agent and a fakeintake
> - creating a k8s (kind/EKS) cluster
> - creating a VM with docker runtime and deploying a docker-compose schema
> - etc.

Avoid custom provisioners.

## Structuring tests
When a test suite uses parent and child tests, every child must be TRULY INDEPENDENT.
DO NOT ASSUME execution order - subtestB must never depend on the results/actions/changes/setup of subtestA. Write them as if randomly scheduled.
Put common setup in the _parent_ test.

## Keeping tests fast
### Kubernetes tests
Use a kind cluster rather than an EKS cluster unless absolutely necessary: it is cheaper, faster to provision, and more reliable.
EKS tests usually run only on `main` and/or nightly.

### General speed guidelines
If your test runs on:
- All PRs: _this should almost never happen_.
- A subset of PRs based on changed files: <= 15mn.
- `main` or nightly: more lax, but keep it under ~30-40mn.


## Improving debugability and observability
The provisioned infra is destroyed after the test finishes, so a poorly instrumented failure is hard to debug.
Locally, set `E2E_DEV_MODE=true` to skip infra teardown. This is impossible in CI, which is often where issues crop up. So:
- Log everything required for a future debug run.
- Run things like `journalctl` on failure to capture the required info. The VM, and any relevant info, is destroyed on test failure.
- HOWEVER, don't spam gitlab logs with useless context. For complex dumps, create a log artifact and upload it instead of pasting everything inline.

## Wiring the test into CI
Add appropriate `needs:` blocks for things your test depends on (e.g. the Agent/Cluster Agent docker images). Otherwise the test may hang or fail while waiting for the artifact to be pushed to a registry.
