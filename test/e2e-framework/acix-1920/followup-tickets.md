# Draft follow-up tickets

Scratch file, not documentation. Produced while doing ACIX-1892 / ACIX-1909 (PR C). Paste into Jira
and delete.

Three unrelated epics, deliberately kept separate because the shape of the work differs:

| Epic | Shape |
|---|---|
| [1 — Consolidate E2E documentation](#epic-1--consolidate-e2e-documentation-into-the-repository) | Docs. Audit of all ~54 pages of the `E2E Framework` tree in the ADX Confluence space. |
| [2 — Route orchestrator-pulled images through the cache](#epic-2--route-orchestrator-pulled-images-through-the-ecr-cache) | Framework and infrastructure. The K8s half of PR #47900. |
| [3 — Get the workload app images off GHCR](#epic-3--get-the-workload-app-images-off-ghcr) | Build and release. Blocked on an owner decision. |

Epics 2 and 3 were found while writing the docs for Epic 1 — `dependencies.md` currently documents
this behaviour honestly rather than solving it, which is the right order but leaves the text
describing a known hole.

---

## Epic 1 — Consolidate E2E documentation into the repository

### Context

E2E documentation is split between `docs/public/how-to/test/e2e/` (5 pages, added by ACIX-1892 /
ACIX-1909) and ~54 Confluence pages under `ADX / E2E Framework`. The Confluence tree predates two
structural changes and has not caught up with either:

- **`test-infra-definitions` was vendored** into this repo at `test/e2e-framework/`. Every page whose
  subject is the cross-repo workflow — dependency bumps, version pinning, the release train — now
  documents a mechanism with no remaining anchor: there is no `test-infra-definitions` requirement in
  `test/new-e2e/go.mod` and no `.gitlab/common/test_infra_version.yml`.
- **The framework API changed** in 2024. `e2e.Suite[e2e.FakeIntakeEnv]`, `e2e.EC2VMStackDef`,
  `ec2vm.NewEc2VM` and `client.NewVM` appear throughout the Confluence code samples and none of them
  exist. `test/new-e2e/pkg/` is gone too, so most `pkg.go.dev` links 404.

Roughly a third of the tree is dead. Nothing detects that, because prose on Confluence has no CI.
Content that lives next to the code it describes gets caught by review and by the strict docs build.

Two concrete contradictions found during the audit, in both of which the repo is right and Confluence
is wrong — this is the failure mode the epic exists to stop:

- `Getting started with E2E` tells you to generate a `PULUMI_CONFIG_PASSPHRASE` in 1Password and
  export it from your shell rc. `dda inv e2e.setup` has generated and persisted it for some time.
- `Running Manual Tests` documents `--use-fakeintake` as defaulting to `True`. It defaults to
  `False` (`tasks/e2e_framework/aws/vm.py:64`).

### Approach

Not a lift-and-shift. Each page is one of: migrate (rewritten in-repo), merge (folded into an
existing page), keep (stays on Confluence), or delete.

The split is **paired, not filtered**. `docs/public/` publishes to
`datadoghq.dev/datadog-agent`, so anything with 1Password links, access-request forms, AppGate
entitlement names, monitor IDs, SSO start URLs or secret parameter names stays on Confluence. Every
public page that has an internal counterpart names it in a single admonition, and the internal page
links back — one link each way, not scattered cross-references, so drift is visible.

AWS account IDs are already published (`running.md`, `dependencies.md`) and are not treated as
blockers. Named secret *locations*, internal dashboards and access-request flows are.

### Constraints

- **Images.** ~14 screenshots/diagrams across the pages worth keeping. Committing PNGs is acceptable;
  SVG or mermaid is preferred. Three are load-bearing: the E2E architecture diagram (`E2E Overview`),
  the NAT topology, and the S3-proxy architecture — that last page's "System description" section
  *is* the image. Two of the three stay internal anyway.
- **Don't migrate a broken procedure.** `Release a new workload apps` describes a workflow with no
  working implementation post-vendoring; see the note at the end.

### Sequencing

Ticket 1.1 is independent and highest value. Ticket 1.3 needs sign-off from the ADX page owners.

---

### Ticket 1.1 — Add the two missing E2E documentation pages

**Type:** Task · **Estimate:** M · **Blocked by:** nothing

#### Context

Two subjects have no coverage at all in `docs/public/`, while being the most actively maintained
pages on Confluence — which is a good proxy for what people actually read.

1. **CI job registration.** Zero hits for `new_e2e_template` or `.on_*_or_e2e_changes` anywhere under
   `docs/public/`. A developer who has written a passing local test has no in-repo documentation for
   how to make it run in CI.
2. **Troubleshooting.** No troubleshooting content under `docs/public/how-to/test/`, even though
   `index.md` and `running.md` walk users directly into the failure modes Confluence catalogues.

#### Scope

Create `docs/public/how-to/test/e2e/ci-jobs.md`:

- A job extending `.new_e2e_template`; the `TEAM` and `TARGETS` variables (`TARGETS` is relative to
  `test/new-e2e/`).
- Choosing `needs:` by artifact — `deploy_deb_testing-a7_x64`, `deploy_windows_testing-a7`,
  `qa_agent`, `qa_dca`, `qa_dogstatsd` — and why a missing `needs:` makes the job hang rather than
  fail cleanly.
- `!reference [.needs_new_e2e_template]` for the dependency cache.
- Writing an `.on_<team>_or_e2e_changes` rule composed with `.on_e2e_main_release_or_rc`.
- Grouping suites into jobs (≤5 per job) by artifact type, with `EXTRA_PARAMS: --run "<regex>"`.
- Correct file path: `.gitlab/test/e2e/e2e.yml`. Both source pages say `.gitlab/e2e.yaml` or
  `.gitlab/e2e/e2e.yml`; neither exists.

Create `docs/public/how-to/test/e2e/troubleshooting.md`:

- Flake triage starting from the GitLab job log (Test Visibility entry point).
- `E2E_DEV_MODE` / `e2e.WithDevMode()`; `E2E_SKIP_DELETE_ON_FAILURE` /
  `e2e.WithSkipDeleteOnFailure()`.
- Auto-flare artifacts under `e2e-output/`, and the `Diagnose` hooks on the environment types.
- Running locally against a CI pipeline's packages: `E2E_PIPELINE_ID` + `E2E_COMMIT_SHA`.
- Recovering from stuck state: `dda inv new-e2e-tests.clean -l -s`, locked stacks,
  `pulumi --project e2elocal`, leaked ECS *services* (never delete the shared cluster), leaked EKS
  capacity providers.
- Common local failures: passphrase-protected SSH key and the `privateKeyPath` field, the ssh-agent
  5-key limit, `InvalidKeyPair.NotFound`, `InvalidClientTokenId` / aws-vault keychain cache.

Sources: `3492283118`, `3492282982` (16 issues, split — see below), `4741694837`, `5005705218`,
`3506374060`, `3492282740`, `3492283215` (CI-grouping half only).

#### Out of scope / must not migrate

Keep on Confluence and link to it once from each new page:

- The 1Password shared SSH key, the Freshservice access-request ticket, the CLOUDA access form.
- AppGate: the four required entitlements, the `DataDog/appgate` PR process, the Workday subproduct
  check. This is the root cause of the `wait-cloud-init` hang, so the public page should name the
  symptom and point at the internal page for the fix.
- The SSO start URL and `$DATADOG_ROOT/devtools` / `setup-awsconfig`.
- Screenshots (7 across the two source pages).

#### Known-stale content to fix while porting, not copy

- Two entries in `Common E2E issues` say to run `inv setup` "from test-infra-definitions" — archived.
- The ECS-idempotency entry references `e2e.AgentEnv` — dead API.
- `Diagnose` is linked at `test/new-e2e/pkg/environments`; it is now
  `test/e2e-framework/testing/environments`.
- Bare `inv` throughout both sources; the repo standard is `dda inv`.
- ~60% of `5005705218` is manual `PULUMI_CONFIG_PASSPHRASE` juggling made obsolete by `e2e.setup`.

#### Acceptance criteria

- Both pages exist, are registered in `mkdocs.yml` under How-to → Test → End-to-end tests, and are
  listed in `e2e/index.md`'s "In this section".
- `dda run docs build` passes (it is `--strict`; broken internal links fail the build).
- No 1Password / Freshservice / AppGate-entitlement / monitor-ID content on the public pages.
- Every command in both pages verified against `tasks/` or `.gitlab/` at time of writing.
- The two Confluence sources are reduced to their internal-only residue and link to the new pages.

---

### Ticket 1.2 — Fold the remaining Confluence E2E content into the existing pages

**Type:** Task · **Estimate:** M · **Blocked by:** nothing (independent of 1.1)

#### Context

Twelve Confluence pages either duplicate an existing repo page or add something it is missing. The
additions are mostly the *why* behind a step, which is exactly what gets lost in a condensation.

#### Scope

**`e2e/index.md`** — from `Getting started with E2E` (`3492282517`):

- `aws-vault` (≥6.6.2) and `azure-cli` (≥2.62) version floors; `xcode-select --install`.
- Windows/PowerShell setup guidance. The repo says nothing about developing on Windows.
- `dda inv e2e.setup.debug` for diagnosing a broken setup.
- Switching `~/.test_infra_config.yaml` to the `agent-qa` account.
- **Do not port** the `PULUMI_CONFIG_PASSPHRASE` / 1Password step — obsolete and contradicts
  `index.md`.

**`e2e/index.md` or a new short page** — from `E2E Overview` (`3492282435`):

- Instance retention: `agent-sandbox` is swept daily, leftovers weekly on Saturday; CI deletes its
  own instances in `agent-qa` and a weekly job catches panics. Long-lived instances need an explicit
  ignore entry in `test-infra-cleaner`.
- Why EOL platforms (CentOS 7 etc.) are kept — the Agent supports them.
- Keep the account/permission model and the AppGate prerequisite internal.

**`e2e/running.md`** — from `3750691368`, `5910135054`, `3958866373`, `5088904823`, `3492282740`:

- **Local provisioners**, currently absent: `localkubernetes.Provisioner()` (needs `kind`, `docker`,
  free port `300080`) and `localhost.PodmanProvisioner()` (podman ≥5.3.0). Note
  `openshift_local.go` also exists and is undocumented. Include the caveat that local runs won't
  reproduce EKS or infra-level failures.
- **The retry mechanism**: `--max-retries=N`, why GitLab-native retry is worse (reprovisions infra,
  retries known-flaky tests), the filter-to-leaf-tests + drop-known-flaky + `teardownOnly` sequence,
  and the requirement that tests tolerate non-fresh infra. Cross-link
  `codereview_guideline.md`'s "Cleanup after yourself".
- `dda inv new-e2e-tests.clean -s` — `running.md` recommends dev mode but never says how to clean up
  after it.
- IDE setup: `go.work`, `go.testTimeout: "0s"`, `go.testFlags: ["-v"]`.
- Local agent image: the `dda env dev start` / `run` wrapper, `cluster-agent.hacky-dev-image-build`
  and `--cluster-agent-image` (both missing), the mapping to `ddagent:fullImagePath` /
  `ddagent:clusterAgentFullImagePath`, and the **2-day ECR retention** on the dev image repository.
- Local agent package: *why* a dev env is mandatory (omnibus writes to `/opt/datadog-agent` and will
  clobber a locally installed Agent), the explicit `dda env dev start --id … --arch amd64` /
  `shell` invocations, ~4 min build, output in `omnibus/pkg`, and that in a mixed package only the
  deb tests run while the rest fail before provisioning.
- The heuristic that Azure boots Windows VMs faster than AWS.

**`e2e/writing.md`, or a new `e2e/secrets.md`** — from `4715774450`, the cleanest page in the tree
(every API verified present):

- `secretsutils.NewClient` / `SetSecret`; wiring `secret_backend_command` with `ENC[handle]`.
- `secretsutils.WithUnixSetupScript` / `WithWindowsSetupScript` plus
  `agentparams.WithSkipAPIKeyInConfig()`.
- Refreshing via `Agent.Client.Secret(WithArgs{"refresh"})` or `secret_refresh_interval`.
- Validating rotation with fakeintake `GetLastAPIKey` (`/debug/lastAPIKey`).
- Fix the unbalanced paren in the source snippet and the prose describing the second parameter of
  `WithUnixSetupScript` (it is `allowGroupExec`).

**`e2e/custom-amis.md`** — from `E2E AMI Management` (`5611749587`). Mostly done in PR C (pinning
rationale, AMI-type taxonomy, `--latest-ami`, the two-`platforms.json` disambiguation). Remaining:
link the manual build runbook and the "Reinforce E2E AMI maintenance" RFC as internal references.

**`e2e/dependencies.md`** — from `5040342019` / `6496029043`. Conventions are done in PR C. Consider
moving the vendored-artifact inventory in-repo; 3 of its 5 entries have no recorded upstream or
version, which our own "pin everything" rule forbids. Filling those in is a prerequisite either way.

**`manual-qa/index.md` and `aws-vm.md`** — from `Running Manual Tests` (`3492282654`):

- The AppGate prerequisite (name the requirement publicly, link internally for entitlements).
- Recovering connection details after losing terminal output: `pulumi stack ls`,
  `pulumi stack output --json`. The repo only offers `dda inv aws.show-vm`.
- **Do not port** the pasted `--help` output; it is already wrong about the fakeintake default, which
  is why the repo pages use hand-maintained tables.

**`test/new-e2e/codereview_guideline.md`** — from `3492283102` and `3492283215`:

- Avoid `Sleep`; retry the assertion, act once; wait for the service under test to be ready.
- One test per platform rather than table-driven tests, because `flake.Mark` cannot target a table
  row.
- Drop the 2024 CI-cost figures, or reduce them to one sentence without numbers.
- Do not port the code samples — every one uses the dead pre-2024 API.

#### Acceptance criteria

- Each merged Confluence page is either deleted or reduced to internal-only residue with a link to
  the repo page that replaced it.
- The two known contradictions are resolved in the repo's favour and the Confluence text removed.
- `dda run docs build` passes.
- Every ported command verified against the current tree.

---

### Ticket 1.3 — Retire the dead and superseded E2E Confluence pages

**Type:** Chore · **Estimate:** S · **Blocked by:** 1.1 and 1.2 for the superseded ones

#### Context

17 pages are dead, abandoned, or superseded. Nine are empty or near-empty. Leaving them costs more
than deleting them: they rank in search and they contain instructions that actively mislead — the
`test-infra-definitions` bump procedure reads as current and is a no-op.

#### Scope

Delete, or archive if the space policy requires it. Needs ADX page-owner sign-off.

**Empty or stub (9):**

| Page | ID | Note |
|---|---|---|
| E2E - [WIP] Add an option to the agent | `3492283159` | empty, v1, untouched since 2024-02-21 |
| E2E - [WIP] Create a new cloud account | `3492283173` | empty, same batch |
| E2E - [WIP] Custom environment | `3492283187` | empty; covered by `examples/customenv_with_*_test.go` |
| E2E - [WIP] File management in e2e tests | `3492283201` | empty; covered by `Host.CopyFolder` |
| WIP (folder) | `4414177531` | container for the four above |
| E2E infrastructure | `5162009267` | children-macro nav page only |
| Systems documentation | `5114495100` | empty container |
| E2E - How to | `3492283082` | children-macro hub; `e2e/index.md` is the hub now |
| Run E2E tests with your local changes | `5088806709` | two links, no content |

**Dead workflow (3)** — verified: no `test-infra-definitions` in `test/new-e2e/go.mod`, no
`.gitlab/common/test_infra_version.yml`, source repo archived:

| Page | ID |
|---|---|
| Framework releasing | `5601986390` |
| Bump test-infra-definitions | `5603230019` |
| E2E - Work with test-infra-definitions and datadog-agent | `3492283241` |

**Superseded or rotted (5):**

| Page | ID | Superseded by |
|---|---|---|
| Pulumi 101 | `3492282886` | `test/e2e-framework/README.md`; all imports and APIs dead |
| E2E - Add a fakeintake parser | `3492283145` | `test/fakeintake/AGENTS.md` § "Adding a new payload type", which is more complete |
| E2E - Use our Dockerhub mirror registry | `3740697756` | `e2e/dependencies.md` |
| Create an EC2 VM using the AWS console | `5326373984` | `manual-qa/aws-vm.md` |
| NPM / CWS / NDM E2E functional tests | `3238101159`, `3354525801`, `3539011155` | abandoned 2024 scoping docs; two coverage tables never filled in, CWS's is a completed Kitchen→Pulumi backlog |

Before deleting the NDM page, salvage one line: the `networkstatic/nflow-generator` and `snmpsim`
load-generator container names are useful and recorded nowhere else. Do not salvage the copied
`FlowPayload` struct — it belongs in Go.

Before deleting `3740697756`, confirm `e2e/dependencies.md` covers its two non-obvious points: that
`public.ecr.aws` direct is not an approved route, and the in-cluster pull behaviour. (Both added in
PR C.)

#### Explicitly staying on Confluence

Not part of this ticket, listed so the tree makes sense afterwards: `E2E Framework` (`3492282413`) as
the internal hub; account access (`4887512622`); cloud infra (`4090234402`, `4157440516`,
`5114691659`, `6876792626`); external-repo onboarding (`3492282864` + `5769429257` + `5946541855`,
the latter two folded in); on-call (`5582390640`); the functional-test-scoping template
(`3211886742`); the workshop (`3492283023`); the wins log (`3252847308`); the doggo/GitLab UI
walkthrough (`3492283267`); workload apps (`3536257272`, `5602902788`).

Three of those need fixing in place, independent of this epic:

- `5114691659` links `test-infra-definitions/components/datadog/apps/s3-proxy-nginx`, which no longer
  exists in either repo. Where did the nginx image source go?
- `3536257272` and `5602902788` describe adding and releasing workload app images via a repo that is
  archived. See below.
- `4887512622`, `3492282517`, `3958866373` say `sso-…-account-admin`; the code uses
  `…-account-admin-8h`.

---

### Out of scope for Epic 1

The two workload-app pages (`3536257272`, `5602902788`) cannot be rewritten until Epic 3 resolves
where those images are built. Leave them as-is, flagged.

---

## Epic 2 — Route orchestrator-pulled images through the ECR cache

### Context

PR #47900 moved image references onto the ECR pull-through cache for compose files, ECS task
definitions, podman builds and `docker run` calls. Its title says it plainly — *"use ECR pull-through
cache for **non-K8s** image refs"*. This epic is the half that was deferred.

Images pulled by an orchestrator rather than by us still leave the VPC. Current state, counting
references under `test/` and excluding markdown:

| Registry | Refs | Mirrored today? |
|---|---|---|
| `ghcr.io/datadog/apps-*` | 94 (52 files) | No — see Epic 3 |
| other `ghcr.io` | 8 | No |
| `docker.io` / bare | ~49 (23 files) | Partially: containerd mirror → `mirror.gcr.io` |
| non-mirror `gcr.io` | 30 | No |
| `registry.k8s.io` | 4 | No |
| `quay.io` | 3 | No, though a `quay/` cache namespace exists |

Plus remotely-hosted manifests, which `codereview_guideline.md` already forbids and which are still
present: flannel from `raw.githubusercontent.com` (`components/kubernetes/kubeadm.go:231`) and Helm
repositories (`components/kubernetes/nvidia/nvidia.go:289`).

### The key insight: three environments, three difficulties

The problem is usually described as "Kind can't pull from ECR". That is too broad — the managed
environments are already solved by IAM and need nothing but a reference rewrite.

| Environment | Who pulls | Can it authenticate to ECR? |
|---|---|---|
| **EKS / EKS Fargate** | kubelet on our nodes | **Yes.** The node role already has `AmazonEC2ContainerRegistryReadOnly` (`resources/aws/eks/role.go:29`). Rewrite the ref and it works — no secret, no `imagePullSecret`. |
| **ECS / Fargate** | the ECS agent | **Yes**, via the task execution role. Rewrite the ref. |
| **Kind** | containerd inside the Kind node container | **No.** Two independent reasons, below. |

Why Kind can't, specifically:

1. **No credential helper.** containerd's CRI registry config has no helper hook. Docker has one, and
   `docker-credential-ecr-login` *is* prebaked into the `-e2e` AMIs — so the VM can authenticate
   while the Kind node cannot. `hosts.toml` can only carry a static `Authorization` header.
2. **No IMDS.** `resources/aws/ec2/vm.go:65-66` sets `HttpTokens: required` but leaves
   `HttpPutResponseHopLimit` at the AWS default of 1, so containers on the VM — including the Kind
   node — cannot reach `169.254.169.254` at all. Any in-node mechanism relying on the instance role
   is blocked before it starts.

`imagePullSecrets` are not the answer for the hard case: they work in-cluster and are already used for
Agent images via `ddagent:imagePull*`, but only when the reference is already an ECR URL. They do
nothing for a `docker.io` reference inside a third-party chart.

### Ticket 2.1 — Rewrite image references in EKS, EKS Fargate and ECS

**Type:** Task · **Estimate:** S · **Blocked by:** nothing

The easy win, and it should land first to shrink the surface before anyone touches Kind.

Route every third-party reference in the EKS / EKS Fargate / ECS paths through
`e.ImagePullRegistry()` (Pulumi components) or the parameter store
(`parameters.ImagePullRegistry`, tests), using the `strings.SplitN(reg, ",", 2)[0]` idiom. Covers the
`quay.io`, non-mirror `gcr.io` and `registry.k8s.io` references in those paths.

Follow the existing patterns: `components/datadog/apps/etcd/k8s.go` (quay from a component),
`tests/installer/host/host.go` (mirrored-with-upstream-fallback pair).

Excludes the 94 `apps-*` references (Epic 3) and anything Kind-only (2.2).

**Acceptance:** no third-party registry hostname literal remains in the EKS/ECS code paths; EKS and
ECS suites green; `dda inv new-e2e-tests.run` against one EKS and one ECS suite shows pulls resolving
to the cache.

### Ticket 2.2 — Give the Kind node authenticated access to the cache

**Type:** Spike then Task · **Estimate:** L · **Blocked by:** nothing

The real work. Start as a spike, because the four options have materially different operational cost
and the choice should be deliberate.

| Option | Covers | Cost / risk |
|---|---|---|
| **A. Registry proxy on the VM** — run a pull-through registry container on the EC2 host, authenticating with the instance profile; point the node's containerd at `http://<host-ip>:5000` | Everything, including third-party charts | No credential inside the cluster. One more moving part per VM, and a new failure mode to diagnose. **Precedent: the installer tests already operate an nginx S3 proxy for the same reason.** Recommended. |
| **B. Static ECR token in `hosts.toml`** | Everything | Smallest diff. Needs a token minted at provision time and written into the node config; 12h expiry is comfortably longer than any test. Puts a live credential in the Pulumi program and on the VM's disk. |
| **C. kubelet credential provider** — install `ecr-credential-provider` plus `--image-credential-provider-config` into the node, as the EKS AMIs do | Everything pulled by kubelet | The "correct" upstream answer, but needs the binary in the node image **and** raising `HttpPutResponseHopLimit` to 2, which exposes IMDS to every container on the VM. Security review required. |
| **D. `kind load docker-image`** | Only an enumerable set | VM's Docker pulls (authenticated), side-loads into the node; no auth in-cluster at all. Cheap and robust for a known list, useless for third-party charts. Viable as a stopgap for the `apps-*` images. |

Deliverables: a decision record, then implementation behind whichever mechanism is chosen, applied to
**both** containerd config shapes — the `certs.d` / `hosts.toml` path used by Kind ≥ v0.27 and the
older `containerdConfigPatches` mirror stanza in `kind-cluster.yaml`. Both currently hardcode
`mirror.gcr.io`.

Keep the `mirror.gcr.io` fallback for GCP: there is no pull-through cache there,
`InternalDockerhubMirror()` correctly returns `registry-1.docker.io`, and `mirror.gcr.io` is
in-network for GCP. This ticket must not regress that.

**Acceptance:** a Kind suite on AWS pulls a `docker.io` image with no egress outside the VPC;
`components/kubernetes/hosts.toml` no longer names a public mirror on the AWS path; GCP Kind runs
unaffected; `docs/public/how-to/test/e2e/dependencies.md` § "Kubernetes and Kind" updated, since it
currently documents the gap.

### Ticket 2.3 — Vendor the remotely-hosted Kubernetes manifests

**Type:** Task · **Estimate:** S · **Blocked by:** nothing

`kubeadm.go:231` applies flannel straight from `raw.githubusercontent.com`; `nvidia.go:289` resolves a
Helm repository at provision time. Both fetch a manifest *and* the images it references, and both are
explicitly called out as anti-patterns in `codereview_guideline.md`.

Vendor the manifests into the repo, pin them, and rewrite their image references through the cache.

**Acceptance:** no `http(s)://` manifest fetch remains in `components/kubernetes/`; kubeadm and GPU
suites green.

---

## Epic 3 — Get the workload app images off GHCR

### Context

`components/datadog/apps/` defines the shared workload apps used by K8s, ECS and Fargate tests. Every
one is referenced as `ghcr.io/datadog/apps-<name>:` + `apps.Version` — **94 references across 52
files**, currently pinned at `v0.0.6`.

GHCR is not a supported pull-through upstream, so these cannot simply be rewritten. And the release
procedure documented on Confluence no longer exists:

> merge the app change → ask #agent-devx-help to `git tag v0.0.x` on **test-infra-definitions**,
> which triggers the image release pipeline → bump `components/datadog/apps/version.go` → bump
> test-infra-definitions in datadog-agent

The Go definitions were vendored into this repo; **the image sources were not**. Verified: no
Dockerfiles under `test/e2e-framework/components/datadog/apps/`, no `images/` directories, and no
build or publish job for `ghcr.io/datadog/apps-*` anywhere in `datadog-agent`. The final step of the
procedure is gone outright, and the tag-triggers-build step lives in an archived repository.

**Net effect: editing a workload app image today has no working path.** The images are frozen at
whatever `v0.0.6` contained.

This is a live risk, not just a docs problem: a security fix in one of these images cannot currently
be shipped.

### Ticket 3.1 — Decide where workload app images are built and published

**Type:** Spike · **Estimate:** M · **Blocked by:** owner decision

Options to evaluate:

1. Recover the Dockerfiles from the archived `test-infra-definitions` history, add them under
   `components/datadog/apps/<app>/images/`, and publish from `datadog-agent` CI to **ECR in
   `agent-qa`** — the natural target, since every consumer can already authenticate to it (node role,
   task execution role, and whatever Ticket 2.2 chooses for Kind).
2. Same, but keep publishing to GHCR. Cheapest migration, but leaves 94 references on an upstream we
   cannot cache — so it does not close the CI-internet requirement.
3. Replace the bespoke apps with off-the-shelf images from a supported upstream where one exists.
   Reduces what we maintain; not all of them have an equivalent.

Deliverables: a decision record; confirmation that the Dockerfiles are recoverable from git history;
the versioning scheme (keep the single `apps.Version` for all apps, or version per app).

### Ticket 3.2 — Restore the build and publish pipeline

**Type:** Task · **Estimate:** L · **Blocked by:** 3.1

Dockerfiles in-repo, a CI job that builds and pushes on tag or on change, and a documented way to bump
the pinned version. Include the multi-arch matrix the apps need.

**Acceptance:** a change to one app image can be built, published and consumed end to end by a test,
with the procedure documented in-repo rather than on Confluence.

### Ticket 3.3 — Move the 94 references behind the cache

**Type:** Task · **Estimate:** M · **Blocked by:** 3.2, and Ticket 2.2 for the Kind path

Resolve app image references through `e.ImagePullRegistry()` / the parameter store instead of a
hardcoded `ghcr.io` host, keeping a documented fallback for local and non-AWS runs.

**Acceptance:** no `ghcr.io` literal in `components/datadog/apps/`; K8s, ECS and Fargate suites green;
`docs/public/how-to/test/e2e/dependencies.md` updated.

### Ticket 3.4 — Rewrite the two Confluence workload-app pages

**Type:** Chore · **Estimate:** S · **Blocked by:** 3.2

`How to add a docker workload app` (`3536257272`) and `Release a new workload apps` (`5602902788`)
both document the dead procedure. Once 3.2 lands, replace them with an in-repo page — the Go half of
the "add an app" instructions survived the vendoring and is still accurate, so this is mostly about
the image half.

Until then, both pages should carry a warning that the procedure does not work.
