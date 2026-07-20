# E2E Test writing guidelines
The following guidelines are written in rough "importance" order - but you should follow _all_ of them to the best of your ability.

## Making tests reliable
### Avoiding external dependencies
One of the biggest factors making e2e tests flaky / susceptible to unrelated failures is their reliance on _external dependencies_, i.e. everything outside the AWS/GCP/Azure account the test is running in and internal DD systems.

Moreoever, in the near-ish future, we will start blocking _all_ internet access to CI for security reasons. Better to start preparing now !

These can take many shapes, and each has a separate way of avoiding them.
#### Docker image pulls
It is very easy to "accidentally" pull images from `docker.io` / DockerHub. Incidentally, they have very restrictive rate limiting policies, and pulls from DockerHub are a very big source of flakiness in e2e tests.
Watch out for things like:
- `docker run ...` without specifying a registry
- In a Dockerfile, `FROM ...` without specifying a registry
- In a compose file / k8s definition: `image: ...` without specifying a registry.

The better alternative is to use the pull-through cache via ECR that is setup in the `datadog-agent-qa` account: `669783387624.dkr.ecr.us-east-1.amazonaws.com/{dockerhub, ecr-public, quay}/<your-image>:<your-tag>`.
> Notice that only DockerHub, public ECR and Quay are currently supported as upstreams: GHCR is not. If the upstream for your uncached image is not supported, check if an equivalent image exists in one of the supported ones.

> If not running in AWS, try to use your provider's registry, which is often not rate limited. Alternatively, you can ask #agent-devx-help to setup a pull-through cache in that other Cloud environment.

#### System package installs (apt, yum, dnf, zypper, ...)
If your test requires a package not available not available on a bare VM, either:
- Try to avoid using it (e.g. you usually don't need `jq`: the JSON parsing can be done in go)
- Use a containerized environment that you can cache via the previous method. Many tools have prebuilt container images you can just run instead.
- Prebake it into a custom machine image. This is what is done for example with the `Ubuntu2204E2E` OS flavor. This is documented over on [ami-builder](https://github.com/DataDog/ami-builder/ami/images/e2e)
- Store your package installer on an internal package repository. See [Other dependencies](#other-dependencies)

This is because running package managers on the VM exposes you to both rate limiting from the upstream package mirrors, and to any _changes_ made on those mirror's packages - they can be removed, renamed, or incompatible versions be pushed. Also see [Pin your dependencies](#pin-your-dependencies)

#### Other dependencies / Internet accesses
In general, avoid making web requests to external websites (i.e. avoid `ping some-website.com` or `curl some-website.com`). If you need to download a tarball, installer, or package from a website and none of the other previous solutions are acceptable, the best way forward is to vendor in that artifact in our purpose-made S3 bucket - you can use `RemoteHost.HostArtifactClient` for this. See [Confluence](https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/5040342019/E2E+-+Use+a+third+party+artifact+in+test) for more details.

### Pin your depencies
If you must depend on something external, at least make sure you pin down which version you are using (and preferably use a sha256sum verification) to avoid hard-to-track behavior changes.

Watch out: unpinned dependencies can sneak in from unexpected places. For example, `apt install <package>` and similar commands almost always install the latest version of a package.
> This is made even more annoying because upstream package mirrors often don't store all past versions of all packages.

Similarly, not specifying a tag for a Docker image will default to `latest`. Same story for dependencies pulled via a hardcoded `curl ...` - make sure a pinned version appears in the URL fragment

### Think about timing
E2E tests are executed in real-time, on real infra: you are exposed to timing issues in the same way integration tests or things like playwright on frontend are.
Thus, never synchronously assert a property that might take some time to become true. For example, if you expect a payload to reach the fakeintake, use `EventuallyWithT` rather than a direct, synchronous check, to avoid failures due to timing problems.

### Cleanup after yourself
Try to write tests in such a way that they can be retried without error _without needing to reprovision an entire host_: cleanup any generated artifacts, revert any temporary changes made etc.

We have some custom retry logic that will, if possible, retry tests on _the same infra_, making test retries faster and more reliable as we avoid extra re-provisioning flakiness.
If your test is not able to be retried in this way, an expensive "full test retry" will be used instead, incurring extra speed and reliability costs due to the infra reprovisioning.

## Using the framework
The framework in `test/e2e-framework` already handles and abstracts a lot of provisioning and setup logic for you.
When writing a new test, ALWAYS check to see if the provisioning logic is already handled by a helper in the framework
> common examples:
> - creating a VM with the agent and a fakeintake
> - creating a k8s (kind/EKS) cluster
> - creating a VM with docker runtime and deploy a docker-compose schema
> - etc.

In other words: avoid using custom provisioners if at all possible.
When in doubt, feel free to ask #agent-devx-help. If your usecase is truly uncovered, we can consider adding it to the framework so others can also profit !

## Structuring tests
If your test suite makes use of parent and child tests, you need to make sure every child is TRULY INDEPENDENT.
DO NOT ASSUME execution order - subtestB should never depend on the results/actions/changes/setup made by subtestA. Write them as if they were randomly scheduled.
Put any common setup between subtests in the _parent_ test.

## Keeping tests fast
### Kubernetes tests
Prefer using a kind cluster rather than a proper EKS cluster unless absolutely necessary: it is much cheaper and faster to provision, as well as more reliable.
Usually tests running on EKS will only run on `main` and/or nightly.

### General speed guidelines
If your test runs on:
- All PRs: _this should pretty much never happen_.
- A subset of PRs based on changed files: <= 15mn.
- `main` or nightly: much more lax, but try to keep it under ~30-40mn.


## Improving debugability and observability
Since the provisioned infra is destroyed after the test finishes, debugging a failure can be complex if the test is not instrumented properly.
Locally this can be solved by setting `E2E_DEV_MODE=true`, which avoids infra teardown.
But in CI, this is not possible - and this is often where issues crop us. Thus:
- Make sure your test logs everything required for a future debug run.
- This can even extend to running things like `journalctl` on failure to make sure the required info is logged. Remember the VM, and any maybe relevant info, is destroyed on test failure !
- HOWEVER, avoid spamming gitlab logs with endless useless context. For complex dumps prefer creating a log artifact and uploading it instead of pasting everything directly.

## Wiring the test into CI
Make sure you add appropriate `needs:` blocks for things your test depends on (ex: the Agent/Cluster Agent docker images). Otherwise the test might hang or fail outright while waiting for the artifact to be pushed to a registry.
