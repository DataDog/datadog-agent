# E2E Test writing guidelines
These guidelines are in rough "importance" order, but follow _all_ of them.

## Making tests reliable
### Avoiding external dependencies
A major source of flaky / unrelated failures is reliance on _external dependencies_: anything outside the AWS/GCP/Azure account the test runs in and internal DD systems.

We will soon block _all_ internet access from CI for security reasons, so prepare now.

#### Docker image pulls
It is easy to "accidentally" pull images from `docker.io` / DockerHub, a major source of flakiness due to its restrictive rate limiting.
Watch out for:
- `docker run ...` without specifying a registry
- In a Dockerfile, `FROM ...` without specifying a registry
- In a compose file / k8s definition: `image: ...` without specifying a registry.

Use the ECR pull-through cache set up in the `datadog-agent-qa` account instead: `669783387624.dkr.ecr.us-east-1.amazonaws.com/{dockerhub, ecr-public, quay}/<your-image>:<your-tag>`.
> Only DockerHub, public ECR and Quay are supported as upstreams (GHCR is not). If your image's upstream is unsupported, use an equivalent image from a supported one.

> Outside AWS, use your provider's registry, which is often not rate limited. Otherwise, ask #agent-devx-help to set up a pull-through cache in that Cloud environment.

#### System package installs (apt, yum, dnf, zypper, ...)
If your test requires a package unavailable on a bare VM, in order of preference:
- Avoid it (e.g. you rarely need `jq`: parse JSON in go)
- Use a containerized environment cached via the previous method. Many tools ship prebuilt container images you can run directly.
- Prebake it into a custom machine image, as done for the `Ubuntu2204E2E` OS flavor. See [ami-builder](https://github.com/DataDog/ami-builder/ami/images/e2e).
- Store your package installer on an internal package repository. See [Other dependencies](#other-dependencies)

Running package managers on the VM exposes you to rate limiting from upstream mirrors and to _changes_ in their packages - removed, renamed, or incompatible versions. Also see [Pin your dependencies](#pin-your-dependencies).

#### Other dependencies / Internet accesses
Avoid web requests to external websites (`ping some-website.com`, `curl some-website.com`). If you must download a tarball, installer, or package and no previous solution applies, vendor that artifact in our purpose-made S3 bucket via `RemoteHost.HostArtifactClient`. See [Confluence](https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/5040342019/E2E+-+Use+a+third+party+artifact+in+test).

Remotely-hosted Kubernetes resources (Helm charts, CNI manifests like flannel, remote kustomize bases...) are a common hidden source of Internet access - both the manifest and the images it references are pulled at runtime. Vendor the manifest locally and rewrite its image references to the ECR pull-through cache.

### Pin your dependencies
When depending on something external, pin the version to avoid hard-to-track behavior changes, and verify a sha256sum where possible.

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
