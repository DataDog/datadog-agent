# e2ectl: EKS scenario implementation plan

> **Category C — pending feature; not implemented in e2ectl.** Existing framework EKS
> support is reusable, but the CLI schema/driver and standalone mixed-OS installation
> remain proposals. See the [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** proposal only; no EKS implementation or infrastructure changes made.
**Baseline:** current working tree, including the locally implemented typed-config layer.
**Proposed environment ID:** `eks`.

## 1. Goal and scope

Add a reusable EKS environment to `e2ectl` using the existing framework's Pulumi
scenario, rather than writing a second EKS provisioning program.

It must support:

- Linux managed nodes, enabled by default.
- Optional Windows managed nodes alongside Linux nodes.
- **Windows cannot be enabled without Linux.** Explicitly invalid input is rejected,
  not silently corrected by enabling infrastructure the operator disabled.
- The established lifecycle: provision infrastructure, install/update the Agent
  independently, inspect fakeintake, and destroy the environment.
- Typed parameters shared by CLI and executor, automatic examples/defaults/validation,
  explicit registration, and no Pulumi imports in the core CLI or installer.

Separate two deliverables:

1. **Infrastructure support:** EKS control plane, selected node groups, networking,
   Kubernetes access and optional fakeintake, with no Agent installed by Pulumi.
2. **Complete scenario support:** the standalone installer correctly installs Linux
   Agents and, when requested, Windows Agents using the same Cluster Agent.

Do not label a mixed-OS scenario fully supported after only the first deliverable.
Merely starting Windows nodes does not make the current Helm installer Windows-capable.

### Recommended first-release boundaries

| Area | Recommendation |
|---|---|
| Node combinations | Linux-only or Linux + Windows |
| Both OS flags disabled | Reject; this is a usable QA cluster, not a control-plane-only product |
| Node architectures | Linux amd64 and Windows amd64 initially |
| Node images | Existing AL2023 Linux and Windows Server Core 2022 helpers |
| Capacity | One node per enabled group; keep at least one Linux node available |
| Cluster version | Explicit resolved version in typed config; initially default to the repository's current `1.34` |
| VPC, subnets, IAM/profile | Reuse the existing AWS runner/environment configuration |
| Kubernetes Fargate | Disable for this scenario; not needed for the two supported topologies |
| Fakeintake | Optional ECS Fargate fakeintake, owned by the Pulumi scenario |
| Agent artifacts | Released versions first; defer local-image build/push and custom mixed-OS images |
| Infrastructure mutation | No version/topology edits through `install` or Agent `update` |

Rejecting both flags disabled is a **recommended additional product decision**, not
something implied solely by `windows => linux`. A future control-plane-only mode can
be designed separately if needed.

AWS documents a Linux-node **or Fargate** option for Linux-only system pods. Our contract
is deliberately stricter: `nodes.linux: true` means an actual Linux managed node group,
not an implicit Fargate substitute. The Cluster Agent also runs on Linux.

## 2. Current implementation: reuse and gaps

Paths below are relative to `test/e2e-framework/` unless noted otherwise.

| Existing code | What we can reuse | What needs attention |
|---|---|---|
| `scenarios/aws/eks/args.go` | `WithLinuxNodeGroup`, `WithWindowsNodeGroup`, `WithoutFargate` | Current low-level params do not enforce our topology contract |
| `scenarios/aws/eks/run_args.go` | `WithEKSOptions`, `WithoutAgent`, `WithoutFakeIntake` | Scenario options and provisioner options are different layers |
| `scenarios/aws/eks/run.go` | Cluster export and ECS Fargate fakeintake | Agent-dependent workloads must remain disabled |
| `scenarios/aws/eks/cluster.go` | Private EKS endpoint, IAM integration, CNI configuration, node group selection | Fargate defaults, DNS readiness and network prerequisites need deliberate handling |
| `resources/aws/eks/nodeGroups.go` | AL2023/Windows managed nodes, Windows taint, pinned AMI releases | Scaling is currently hardcoded to desired=1, max=1, min=0 |
| `resources/aws/eks/nodes_versions.json` | Per-OS Kubernetes-to-AMI release table | Validate selected version for every enabled OS; do not duplicate this table |
| `testing/provisioners/aws/kubernetes/eks/` | `Provisioner`, `WithRunOptions`, `WithExtraConfigParams`, diagnostics | Return it through `fromTyped[environments.Kubernetes]` |
| `cmd/internal/configschema/` | Defaults, strict decode, generated examples, optional validators, `DecodeResolved` | No new schema engine needed |
| `cmd/e2ectl/internal/driver/` | Typed lifecycle adapter and explicit registry | Register one EKS implementation |
| `cmd/e2ectl-worker/` | Generic job dispatch and typed snapshot writer | Add one EKS builder/registration |
| `testing/installers/kubernetes/helm/helm.go` | Standalone Helm lifecycle, credentials, fakeintake integration | Currently Linux-only, one release, fresh token per invocation, unpinned chart lookup |
| `components/datadog/agent/kubernetes_helm.go` | Existing Linux/Windows release policy and Windows-to-Linux DCA joining | Contains Pulumi types; extract shared pure policy rather than importing it into the CLI |
| `components/outputs/components.go` | Cluster output and Linux/Windows Agent object references | Existing output types suffice; do not introduce an EKS-specific environment union |

### Important source-level details

1. The typed EKS provisioner calls `eks.GetRunParams(...)` and `eks.RunWithEnv(...)`.
   It is not the same entry point as `eks.Run`, which derives options through
   `ParamsFromEnvironment`. Select node groups explicitly; do not expect unrelated
   profile booleans to provide the CLI's topology.
2. Both provisioner and scenario expose a method called `WithExtraConfigParams`.
   The inspected typed provisioner passes **its own** `params.extraConfigParams` into
   `NewTypedPulumiProvisioner`. Use the provisioner-level option for version overrides;
   do not assume the similarly named run option is consumed there.
3. `RunWithEnv` defaults to no example workloads with `GetRunParams` but still deploys
   the VPA CRD. Define that as part of the inherited baseline or add a narrowly scoped
   option to omit it. Do not promise a completely addon-free cluster.
4. Keep `WithDeployTestWorkload` and `WithDeployDogstatsd` off. Some workload paths use
   the Pulumi-installed Agent/Cluster Agent token; they are not safe defaults with
   `WithoutAgent()` and are not needed for an empty reusable environment.
5. The existing fakeintake path checks whether `fakeintakeOptions` is nil, but constructs
   a separate fixed option list (CPU 1024, memory 6144). The common on/off toggle works;
   arbitrary `WithFakeIntakeOptions` settings are not all forwarded by this code path.
   Do not advertise configurable retention/LB/sizing without fixing and testing that.
6. Existing node helpers use `WINDOWS_CORE_2022_x86_64`, not an arbitrary Windows OS
   choice. The checked-in AMI table currently has entries for `1.32`, `1.33`, and `1.34`.
   These are local implementation facts, not a promise of regional AWS availability.

## 3. Desired configuration and validation

### User configuration

Linux-only, the generated default:

```yaml
schema: 1
environment:
  base: eks
  fakeintake: true
  eks:
    version: "1.34"
    nodes:
      linux: true
      windows: false
agent:
  install: helm
  version: "7.69.0"
```

Mixed OS:

```yaml
schema: 1
environment:
  base: eks
  fakeintake: true
  eks:
    version: "1.34"
    nodes:
      linux: true
      windows: true
agent:
  install: helm
  version: "7.69.0"
```

The Agent version is an example, not a new runtime default or a verified image
compatibility claim. Implementation acceptance must select and test a released image
with the required Linux and Windows variants.

Do not add `agent.windows: true`: that would duplicate the environment's topology and
allow contradictory requests. Installation targets come from the provisioned environment.

### One shared type and one shared semantic rule

Proposed file: `cmd/internal/envconfig/eks/config.go`.

```go
type Nodes struct {
    Linux   bool `yaml:"linux" default:"true" description:"Enable Linux managed nodes; required for Windows support."`
    Windows bool `yaml:"windows" default:"false" description:"Enable Windows managed nodes alongside Linux."`
}

type Config struct {
    Version string `yaml:"version" default:"1.34" pattern:"^1[.][0-9]+$" description:"EKS Kubernetes minor version."`
    Nodes   Nodes  `yaml:"nodes" default:"{}" description:"Managed node groups to provision."`
}

type Rules struct{}

func (Rules) Validate(p Config) error {
    if p.Nodes.Windows && !p.Nodes.Linux {
        return fmt.Errorf("nodes.windows requires nodes.linux to be true")
    }
    if !p.Nodes.Linux && !p.Nodes.Windows {
        return fmt.Errorf("nodes: enable at least one node group")
    }
    return nil
}

var Schema = configschema.Must[Config](Rules{})
```

The parent `default:"{}"` matters: it causes nested defaults to be materialized even
when the entire `nodes` mapping is omitted. Without it, an omitted optional struct can
leave both Go booleans false. Include that exact case in the tests.

The second semantic check implements the recommended rejection of an empty topology.
The hook stays in the **shared schema**, not only on the CLI driver: the executor must
enforce the same rule on direct requests.

### Truth table after defaulting

| Linux | Windows | Result |
|---|---|---|
| true | false | Valid Linux-only cluster |
| true | true | Valid mixed cluster |
| false | true | Invalid: Windows requires Linux |
| false | false | Invalid under the recommended usable-cluster policy |

Also test raw-input presence semantics:

- No `eks` section: declared defaults supply version and the Linux-only topology.
- No `nodes` mapping, or `nodes: {}`: Linux=true, Windows=false.
- Only `nodes.windows: true`: Linux defaults to true; valid mixed cluster.
- Explicit Linux=false, Windows=true: error; **do not override false**.
- `null`, string booleans, unknown/duplicate fields: automatic schema errors.

Syntax validation is offline. Version/AMI compatibility comes from the existing catalog
in executor-side provisioning preflight; regional availability, permissions, quotas and
connectivity remain runtime checks. Avoid a second static version enum in the CLI.

## 4. Ownership and process boundaries

```text
shareable YAML
    |
    v
e2ectl: common parsing + eks.Schema + optional semantic rule
    |
    +--> normalized EKS params and explicit common fixtures
    |        |
    |        v
    |    e2ectl-worker: eks.Schema.DecodeResolved
    |        |
    |        v
    |    existing typed EKS provisioner / Pulumi scenario
    |        |
    |        +--> control plane, IAM/network resources, selected node groups
    |        +--> Windows CNI/IPAM configuration when selected
    |        +--> optional ECS Fargate fakeintake
    |        +--> private snapshot with component bindings
    |
    +--> later install/update: standalone Helm + Kubernetes clients
             |
             +--> Linux release / Cluster Agent
             +--> optional Windows release joined to that Cluster Agent
```

- Pulumi owns infrastructure and cloud fakeintake, **not the Agent releases**.
- The core CLI owns discovery, desired config, environment state and installation.
- Reuse `environments.Kubernetes`; it already describes this environment.
- Shared config and common installer code stay Pulumi-free.
- `environments`/`init` remain offline, even with no AWS CLI, credentials or executor.
- No automatic `init()` registration, new dynamic plugin mechanism or base-specific
  switch in discovery commands.

## 5. Executor integration and Pulumi mapping

### Registration

Add `BaseEKS = "eks"` to the existing shared base identifiers and an explicit entry in
the worker registry. Put the builder in a focused file such as
`cmd/e2ectl-worker/scenario_eks.go` rather than expanding the EC2 builder.

The builder:

1. Calls `eksconfig.Schema.DecodeResolved` on `Job.Params`.
2. Receives the already-validated common `fixtures.Config` separately.
3. Selects Linux/Windows `sceneks.Option` values explicitly.
4. Adds `sceneks.WithoutFargate()` for this product's baseline.
5. Always adds `sceneks.WithoutAgent()`.
6. Adds `sceneks.WithoutFakeIntake()` only when the common toggle is false.
7. Wraps `proveks.Provisioner(...)` with `fromTyped[environments.Kubernetes]`.

The principal existing calls are:

```go
nodeOptions := []sceneks.Option{sceneks.WithoutFargate()}
if p.Nodes.Linux {
    nodeOptions = append(nodeOptions, sceneks.WithLinuxNodeGroup())
}
if p.Nodes.Windows {
    nodeOptions = append(nodeOptions, sceneks.WithWindowsNodeGroup())
}

runOptions := []sceneks.RunOption{
    sceneks.WithEKSOptions(nodeOptions...),
    sceneks.WithoutAgent(),
}
if !fixtureConfig.FakeIntake {
    runOptions = append(runOptions, sceneks.WithoutFakeIntake())
}
```

This is a mapping sketch, not the complete implementation: capacity policy, effective
version checks and preflight below must also be implemented.

### Version handling

Initially map `Config.Version` into the provisioner-level `infraconfig.ConfigMap` key
`ddinfra:kubernetesVersion`, built from the existing constants rather than a new magic
string. That feeds both the cluster version and node release lookup.

**Precedence hazard:** `infraconfig.BuildStackParameters` documents
profile < scenario < environment/CLI overrides. An ambient override could therefore
replace the version in the typed file. Choose one of these explicit behaviors:

- Recommended narrow implementation: compare the effective AWS environment version with
  `p.Version` at the beginning of the Pulumi provisioning callback, before `NewCluster`,
  and fail on conflict with an actionable message.
- Longer-term: introduce a typed scenario version option used consistently by the control
  plane and every node helper, avoiding this ambient override path.

Do not silently claim that the file's version was provisioned if an override won.
Resolve/check AMI releases for every enabled OS before creating a paid cluster. A matching
catalog entry is necessary but not sufficient: validate actual regional availability.
Keep creation-only catalog/availability checks off the destroy path.

### Node-group policy

Reuse the existing managed-node resources. Linux uses the AL2023 amd64 helper; Windows
uses the existing Server Core 2022 amd64 helper.

Current helpers set min=0, desired=1, max=1. For the new scenario, add a small optional
scaling-policy override so enabled groups use min=1, desired=1, max=1. Preserve existing
callers' defaults instead of changing every E2E stack's scaling settings globally.

Do not expose arbitrary node counts, AMIs, instance types or a list of heterogeneous
node pools in the first config. Those require real extensions to lower-level helpers,
not merely more struct tags. Verify the inherited instance type is amd64-compatible,
available, and adequate for both OS images; report the resolved capacity in diagnostics.

The invariant is also operational: a successful mixed deployment needs a Ready Linux
node, not just a true boolean in a file. A node group with minimum size one prevents
routine scale-down to zero, but does not guarantee availability during a node failure.

### Linux/Fargate/bootstrap ordering

Disabling the EKS Fargate profile is distinct from disabling ECS Fargate fakeintake.
The existing `gensim-eks` scenario is an example of using `WithoutFargate`.

Inspect the Pulumi EKS component's addon/node dependency graph before relying on this
mode. `cluster.go` describes CoreDNS-on-Fargate as a bootstrap choice. With Fargate off,
ensure node creation does not wait on an addon that itself needs a node. Wait for Linux
capacity, DNS and relevant CNI readiness before declaring the environment ready. Do not
paper over a dependency cycle with larger timeouts or an undocumented Fargate fallback.

### Windows prerequisites to retain and verify

- IPv4 cluster/network configuration supported by Windows.
- Cluster IAM permissions including the VPC resource controller policy.
- Windows node role authentication, including the provider's Windows kube-proxy mapping
  or corresponding access-entry handling. Verify the generated configuration for the
  pinned provider; creating a role is not proof the node can join correctly.
- `amazon-vpc-cni` configuration with `enable-windows-ipam: "true"`, applied before
  Windows nodes depend on it.
- No assumption that Linux custom pod networking applies to Windows. The current
  implementation intentionally uses ordinary subnet IPs for Windows.
- Windows nodes become Ready and can resolve cluster DNS/reach the Cluster Agent and
  fakeintake through the relevant security groups and routes.

Keep existing Windows taint behavior unless deliberately changing it. The helper uses
`node.kubernetes.io/os=windows:NoSchedule`; installer tolerations must match that exact
key/value/effect, not a guessed `os=windows` taint.

## 6. CLI driver, access, state and teardown

Proposed package: `cmd/e2ectl/internal/drivers/eks`.

Implement the existing `driver.Implementation[eksconfig.Config]` methods:

- `ID()` -> `eks`.
- `Description()` -> EKS managed cluster, optional Windows nodes and ECS fakeintake.
- `Start(params, cfg, entry, store)` -> cloud job, output persistence and readiness.
- `Stop(params, cfg, entry, store)` -> destroy the owned Pulumi stack.
- `Installers()` -> the shared Helm implementation adapted for the installed topology.

Register `Define(eksconfig.Schema, "helm", &eks.Driver{})` explicitly. The mandatory
Linux/Windows rule does not need a duplicate driver-local `Validate` method.

### Start

1. Validate/default the file before creating state or invoking the executor.
2. Persist stack identity and the normalized infrastructure specification in private,
   driver-owned state. Record it before a provisioning attempt can leave paid resources.
3. Send normalized `Params`, common fixture settings and the protocol version through
   `workerclient`; do not re-marshal with `omitempty`.
4. Reuse the generic snapshot writer and automatic `_bindings`.
5. Read `kubernetesCluster` through its binding and validate cluster name/kubeconfig.
6. Materialize `entry.KubeconfigPath()` privately and atomically, without changing the
   user's global kubeconfig. The existing command can then print the path.
7. Read the bound fakeintake endpoint if enabled; disabled fixtures require no endpoint.
8. Use the existing Kubernetes client to verify selected managed-node OS groups are
   Ready, API access works, and DNS/CNI are usable. With fakeintake enabled, check health.
9. Mark Ready only after these checks. No Agent release should exist yet.

Use the EC2 driver's job/snapshot handling as a pattern, but extract only genuinely
shared helpers. Do not create a generic state struct with host SSH fields and EKS node
flags side by side.

The private EKS state should contain stack identity, normalized infrastructure params,
resolved topology/version and relevant resource identifiers—not an additional independent
user configuration. Existing `ClusterOutput` remains the reusable cluster connection
output. Persist any driver-specific information under a clearly owned, versioned state
record rather than teaching `envstore.Meta` every provider field.

### Access prerequisites

The current EKS scenario creates a **private-only API endpoint** in existing subnets.
Both the executor's Pulumi Kubernetes provider and later CLI Helm/client calls need
network/DNS access to it. “Pulumi succeeded from CI” does not imply a developer laptop
can install or inspect that cluster.

Document the supported execution environment/VPN/network route, AWS CLI exec-credential
requirements, profile selection, EKS access authorization and renewable credentials.
Do not export a short-lived authentication token as the long-lived solution. Verify
re-attachment after token refresh with the same existing Kubernetes client stack.
Discovery and config generation must not attempt these checks.

### Frozen infrastructure and Agent-only changes

The current command path checks the base but does not compare the full infrastructure
section with the originally provisioned one. For EKS, `install --config` or Agent
`update --config` must not accept a topology/version/fakeintake change and save it as
though Pulumi had applied it.

Add a small CLI-owned optional compatibility contract, invoked by both install and update
before image work, Helm mutation or saving config. The EKS implementation compares the
candidate's normalized infrastructure spec to its persisted provisioned spec. Compare
normalized values, not raw YAML formatting. Shared schema helpers can be used to decode
these values without duplicating validation rules.

The installer must use the provisioned topology. A valid new file containing
Linux=true, Windows=true is not proof that the existing Linux-only cluster has Windows
nodes. Reject that mismatch and direct the operator to create another environment until
a deliberate infrastructure-reconcile command exists.

### Stop and failure handling

- Use the persisted stack identity and original infrastructure state, not a recomputed
  identity from an arbitrary replacement Agent config.
- Never let missing Agent installation, an absent snapshot after partial provisioning,
  or failed Windows readiness prevent an attempt to destroy an existing owned stack.
- Do not require current AMI availability or successful Kubernetes readiness to destroy.
- Do not delete local recovery state until cloud teardown succeeds.
- Keep installation-owned cleanup distinct from Pulumi ownership: remove Helm releases
  and any installer-created objects where possible before cluster deletion. If API access
  is unavailable, preserve diagnostics and still allow an explicit cluster-destroy path;
  an optional Helm cleanup failure must not leave paid infrastructure permanently trapped.
- Keep AWS authentication requirements for deletion; this is not permission bypass.
- Report partial creation/deletion clearly, with stack identity and log locations.

General envstore locking/path validation remains a broader hardening effort. EKS must at
least avoid concurrent start/stop/install mutations of the same entry and preserve enough
private state to recover a partially created, expensive environment.

## 7. Standalone Helm: the main integration work

### Why simply registering the existing installer is insufficient

The current `testing/installers/kubernetes/helm/helm.go`:

- Installs only `dda-linux` and exports only Linux workload references.
- Has no Windows target parameter.
- Regenerates the Cluster Agent token on each call.
- Looks up the chart without pinning its version.
- Uses one minimal values builder, including Linux-oriented `useHostNetwork`.

The CLI wrapper similarly treats Kubernetes environments as differing only in image
transport. That assumption is no longer sufficient for mixed OS. Update that documentation
when implementing this plan; the current code has not acquired these capabilities yet.

### Shared, Pulumi-free installation policy

Extend the existing framework installer rather than adding a separate EKS Helm client.

Proposed additions:

- A small typed target description: Linux enabled, Windows enabled.
- Explicit chart version and configurable bounded timeouts.
- Stable release/namespace naming, retaining `dda-linux` for existing local kind installs.
- Pure Linux/Windows values composition and output-reference construction.
- A release-operation abstraction so ordered installs, failures and retries can be tested
  without a cluster.

Extract the reusable OS-specific policy from
`components/datadog/agent/kubernetes_helm.go` into a neutral package, for example
`components/datadog/agent/helmvalues`. Both Pulumi and standalone adapters should consume
it. Keep unresolved Pulumi outputs/resource creation on the Pulumi side; extract plain
settings, reference names, selectors and ownership policy, not a function that requires
Pulumi input values in the CLI. Differential/rendered-manifest tests should protect the
existing Pulumi path from a behavior change during extraction.

Do not copy the large Windows values map into a second implementation. Conversely, a
complete redesign of every Helm feature is not a prerequisite: extract the policy needed
for these two OS targets first.

### Two releases, one Cluster Agent

| Release | Workloads/ownership |
|---|---|
| `dda-linux` | Linux node Agents, Linux Cluster Agent, shared chart resources/CRDs |
| `dda-windows` | Windows node Agents only; joins the Linux Cluster Agent |

Install or upgrade Linux first, wait for the shared services/credentials, then Windows.
For the Windows release:

- `targetSystem: windows`.
- Explicit Windows OS/amd64 scheduling and a toleration for the existing Windows taint.
- Disable a second Cluster Agent and cluster-checks runner.
- Use `existingClusterAgent.join` and the actual Linux service/token-secret names.
- Disable duplicate operator/CRD and kube-state-metrics ownership where required by the
  selected chart; verify rendered manifests, not only input values.
- Do not inherit Linux host networking, container runtime paths or Linux-only features.
- Configure the same cluster identity and the intended fakeintake endpoints.

Use actual chart/release names for references instead of guessing selectors. Existing
`KubernetesAgentOutput` already has a `WindowsNodeAgent` field. Populate it when installed;
leave Windows Cluster Agent fields empty because there is no Windows Cluster Agent.

### Credentials, images and partial upgrades

1. Reuse the Linux release's Cluster Agent token Secret, or create a clearly installer-
   owned shared Secret with stable identity. Repeated installs must not independently
   rotate the token and strand one OS release after a partial failure.
2. Resolve API/application keys through the current runner secret store. Avoid storing
   credential values in shareable config, generic metadata or diagnostic messages.
3. Pin a tested chart version. Extract any shared chart-version constant into a neutral
   package rather than importing the Pulumi Agent package.
4. Select a released Agent image that actually has the required Windows Server Core
   compatible variant, plus the Linux variant. A semver-looking tag is not enough.
   The existing Pulumi `WithWindowsImage` path is useful reference behavior.
5. Keep the Cluster Agent image Linux-compatible and versioned deliberately.
6. Use bounded, cancellable operations with a Windows-appropriate readiness budget;
   image pulls/bootstrap can take longer than the current five-minute Helm timeout.
7. If Linux succeeds and Windows fails, keep per-release outcome/recovery information,
   do not claim the complete installation succeeded, and support a safe retry. Do not
   destructively roll back unrelated working infrastructure.

For the first supported slice, reject `agent.image` for EKS with an actionable
“released versions only” message. Local Docker images cannot be loaded into EKS using
`kind load`, and a local Linux build cannot run as a Windows container. Remote registry
publication, pull credentials, platform manifests, Windows builds and artifact-specific
options deserve their own later implementation.

### Install/update command integration

Supply the installer its targets from persisted EKS state using a small adapter or an
explicit target resolver. Do not insert `if base == "eks"` in generic Helm code or
independently parse a second EKS parameter type there.

The existing Kubernetes installer implements `Updatable`. Before exposing it for EKS:

- Fix `cmdUpdate` so released-version updates do not invoke the local image build path.
  It currently builds whenever `--skip-build` is absent, even with no image specified.
- Keep kind's development-image flow intact.
- Upgrade both OS releases consistently; preserve their shared credentials.
- Update metadata and snapshot outputs to the versions actually installed, including
  partial outcomes. Do not leave `AgentVersion` describing an older release.
- Prove no executor invocation or Pulumi update is involved.

If this work is not included in the initial slice, return an install-only adapter that
**does not implement `Updatable`**. Do not accidentally advertise update through an
embedded installer whose behavior is not supported.

The current Helm wrapper also ignores some common Agent config/integration fields.
For EKS, either implement the supported mappings with tests or reject those fields
explicitly before changing the cluster; do not silently promise configuration application.
A complete installer-owned schema remains separate follow-up work.

## 8. Files and ordered implementation work

Paths below are proposed where marked **new**; helper/interface names are design choices,
not APIs that already exist.

| Phase | Principal files | Deliverable / exit criterion |
|---|---|---|
| 1. Input contract | **new** `cmd/internal/envconfig/eks/config.go`, tests, BUILD | Truth table, nested defaults, shared hook, generated example and resolved roundtrip pass offline |
| 2. Infra policy/preflight | `scenarios/aws/eks/{args,cluster}.go`, `resources/aws/eks/nodeGroups.go`, related tests | Optional per-scenario capacity floor; existing callers unchanged; OS/version prerequisites checked before cluster creation |
| 3. Worker builder | **new** `cmd/e2ectl-worker/scenario_eks.go`, tests; worker registry; `workerclient/workerclient.go` | Exact node/fixture mapping, Agent/workloads off, snapshot bindings; no EC2 regression |
| 4. CLI lifecycle | **new** `cmd/e2ectl/internal/drivers/eks/`; driver registry; narrowly shared helpers | Start/stop, private kubeconfig/state, readiness/error handling, offline discovery |
| 5. Compatibility guard | CLI-owned optional contract, `commands.go`, EKS driver tests | Agent commands cannot silently alter provisioned infrastructure intent |
| 6. Shared Helm policy | **new** neutral values-policy package; existing Pulumi Helm adapter and tests | Common mixed-OS policy, stable chart/release references, no Pulumi dependency in neutral package |
| 7. Standalone installation | `testing/installers/kubernetes/helm/`, `cmd/e2ectl/internal/installer/`, tests | Linux + optional Windows releases, shared token, correct outputs, recoverable partial failures |
| 8. Agent update capability | `commands.go`, installer/metadata tests | Version-only updates skip builds and Pulumi; or explicitly omit `Updatable` |
| 9. Integration and docs | CLI README, framework guidance, generated examples, scoped CI configuration | Authorized Linux/mixed-OS smoke evidence and documented cost/access/cleanup procedure |

Keep dependency changes narrowly scoped. No new per-provider fields on `workerclient.Job`,
no shared VM/Kubernetes params union, and no changes to generic discovery just for EKS.
Adding a base normally does not require a wire-format change; bump the protocol if the
handoff contract itself changes. A same-protocol older worker may lack the new scenario:
report that clearly and rebuild both binaries; capability negotiation can be added later.

## 9. Validation plan

### A. Fast unit tests: no Docker, AWS or Pulumi execution

- All four topology combinations and every omission/null case from section 3.
- Shared semantic rule invoked by CLI decode, generated examples and executor decode.
- Normalized `windows: false` and defaulted `linux: true` survive the JSON job envelope.
- Unknown fields/duplicate keys/wrong types rejected before state creation or worker use.
- `init --base eks` generates Linux-only YAML, with descriptions and no credentials.
- Registry includes EKS and only supported installer/update capabilities.
- Core and shared config dependency graphs contain no Pulumi packages.
- Installer candidate rejects a different normalized version/topology/fixture setting,
  while accepting equivalent YAML formatting and Agent-only edits.
- Unsupported EKS image/config inputs fail before build, Helm mutation or config save.

### B. Worker and resource-plan tests

Use pure option mapping tests and Pulumi mocks where appropriate; constructors returning
closures alone do not prove resource selection.

Assert:

- Linux-only creates the Linux node group and no Windows node group/IPAM patch.
- Mixed OS creates both groups, the Windows taint, IPAM configuration and dependencies.
- No ARM/Bottlerocket/GPU node groups or EKS Fargate profile appear accidentally.
- Enabled capacity uses the new floor without changing legacy callers' defaults.
- Requested control-plane version and both node release selections agree.
- Conflicting effective version is rejected, rather than silently accepted.
- No Agent Helm resources or Agent-dependent sample workloads are in the Pulumi plan.
- Fakeintake disabled means no fakeintake resources/output; enabled means ECS Fargate.
- Snapshots export/import `kubernetesCluster` and optional `fakeIntake` through bindings.
- A failed creation remains destroyable even without a finished snapshot or Ready nodes.

Mocks cannot prove AWS version support, private routing, DNS readiness, image pullability
or Windows node joining. Those belong in the real smoke tests.

### C. Helm policy and lifecycle tests

- Render both releases with the pinned chart and compare selectors, tolerations,
  service/Secret names and the absence of duplicate cluster-scoped ownership.
- Verify Linux and Windows node Agents target the correct OS; DCA stays on Linux.
- Confirm Windows uses the existing Linux Cluster Agent, not a nonexistent Windows DCA.
- Both releases use the intended fakeintake configuration, including any supported
  non-metric signal endpoints; preserve existing no-fakeintake behavior.
- Token identity survives a second install and a partial-upgrade retry.
- Linux-only creates no Windows release; mixed installation creates Windows after Linux.
- Simulated second-release failure returns an error and records actual partial progress.
- Successful snapshot outputs identify the installed workloads and versions.
- Version-only update invokes neither Docker build/delivery nor the executor.
- Existing kind install/update tests remain unchanged in intended behavior.

### D. Authorized real-cloud smoke matrix

Run only after explicit authorization for the AWS account, budget and cleanup window.

| Case | Required evidence |
|---|---|
| Linux-only, fakeintake on | Ready Linux managed node, no Windows nodes, no Agent after start; Linux Agent/DCA and attributed metrics after install |
| Linux + Windows, fakeintake on | Both OS groups Ready; two node-Agent releases, one DCA; Windows-to-DCA connectivity; metrics attributable to each OS's node |
| Fakeintake off | No fakeintake resource/output; Agent installs through the supported normal endpoint path |
| Invalid Windows-only config | Local validation error; no state/resource creation or worker invocation |
| Reattach/retry | A new CLI process can use exported kubeconfig, renew AWS authentication and retry an install |
| Agent update | Versions change on both targets without changing EKS/node-group resource identities |
| Partial failure and stop | Diagnostics retained; successful destroy removes owned cluster/node/fakeintake resources, leaving shared VPC resources intact |

Correlate emitted metrics with the actual Linux/Windows node identities. Merely finding
one generic metric in fakeintake does not prove Windows collection worked. Reuse existing
framework clients and the relevant assertions/workloads from
`test/new-e2e/tests/containers/eks_test.go` and its suite helpers where suitable, but do not
call the broad existing EKS suite evidence for this standalone CLI/installer path.

Use a small dedicated lifecycle smoke suite or harness for this path, gated/manual initially.
CI needs private-network reachability, AWS/Pulumi credentials and deliberate cost approval.
Do not provision EKS as part of normal unit tests or every unrelated config-schema change.
If adding a new-e2e suite, follow `test/new-e2e/AGENTS.md`, the `write-e2e` guidance and
include the actual GitLab rules that will exercise the changed path.

### Planned local build/test checks

From the repository root, after implementation and BUILD regeneration:

```bash
bazel run //:gazelle -- test/e2e-framework/cmd test/e2e-framework/testing/installers/kubernetes/helm
bazel test //test/e2e-framework/cmd/internal/... //test/e2e-framework/cmd/e2ectl/... //test/e2e-framework/cmd/e2ectl-worker:e2ectl-worker_test --test_output=errors
bazel test //test/e2e-framework/testing/installers/kubernetes/helm/... --test_output=errors
bazel build //test/e2e-framework/cmd/e2ectl:e2ectl //test/e2e-framework/cmd/e2ectl-worker:e2ectl-worker
bazel query 'filter("pulumi", deps(//test/e2e-framework/cmd/e2ectl:e2ectl))'
```

Also regenerate/test the new neutral policy and changed scenario/resource packages, run
buildifier on affected BUILD files, and replace both local executables before smoke testing.
These are planned checks, not commands run for this documentation task.

## 10. Acceptance criteria and deferred work

The EKS scenario is ready when:

1. Linux-only and mixed-OS configurations work; Windows-only is rejected at both boundaries.
2. Generated config, validation, defaults and runtime parameters come from one shared type.
3. EKS and optional fakeintake are Pulumi-owned; Agent installs/updates are not.
4. Private kubeconfig/bindings support fresh-process attachment without provisioning again.
5. A mixed install produces Linux and Windows node Agents sharing one Linux Cluster Agent.
6. Disabled fakeintake and explicit boolean values are honored throughout the pipeline.
7. Agent commands cannot lie about applying a topology change or overwrite recovery state.
8. A failed start/install can be retried or cleaned up without creating a second cluster.
9. Core discovery/install code remains Pulumi-free, and offline commands remain fast.
10. Tests cover both the new contract and actual mixed-OS behavior, with explicit cloud costs.

Deferred: Linux ARM/Bottlerocket/GPU pools, arbitrary Windows versions/AMIs, arbitrary
scaling, custom VPC topology, EKS Auto Mode/Fargate workloads, local Windows image builds,
remote image publication, infrastructure reconciliation and full resolved reproduction
manifests. Add each through existing extension points when needed, not as speculative
fields the scenario cannot honor.

## 11. Decisions to confirm before implementation

The plan can proceed with these recommendations, but make them explicit in approval:

- Reject both OS flags disabled (rather than supporting an empty control plane).
- Limit initial nodes to AL2023 amd64 + Windows Server Core 2022 amd64.
- Disable EKS Fargate; keep ECS Fargate fakeintake independently optional.
- Use one node per enabled group with minimum one for the QA scenario.
- Expose a resolved version starting from the current repository default, with conflict
  detection for ambient overrides and checks for the enabled OS catalog entries.
- Ship released-image installation first; either finish released-version update support
  or omit the update capability explicitly.
- Select the actual AWS execution location/network, pinned chart and tested Agent release
  before running the first paid smoke test.

## 12. Sources

Local source paths throughout this document are the implementation baseline; proposed
extensions are identified as such. Public references consulted for platform requirements:

- [AWS: Deploy Windows nodes on EKS clusters](https://docs.aws.amazon.com/eks/latest/userguide/windows-support.html)
  — Linux system-pod capacity, Windows IPAM, IPv4/custom-networking limitations.
- [AWS: Running heterogeneous workloads](https://docs.aws.amazon.com/eks/latest/best-practices/windows-scheduling.html)
  — OS-aware scheduling and mixed-OS isolation.
- [Datadog: Set up the Cluster Agent](https://docs.datadoghq.com/containers/cluster_agent/setup/)
  — separate Linux/Windows Helm installations and joining the existing Linux Cluster Agent.
- [Datadog: Further configure the Agent on Kubernetes](https://docs.datadoghq.com/containers/kubernetes/configuration/)
  — shared token configuration and Kubernetes installation options.

Confirm exact AWS/AMI/chart behavior against the versions selected during implementation;
a checked-in default or general documentation page is not a substitute for that test.
