# e2ectl: Windows host environment plan

> **Category C — pending feature; deliberately not registered yet.** The Windows EC2
> provisioner fits the existing e2ectl pattern exactly, but no Windows-capable agent
> installer exists in the CLI, and a base whose every agent source is rejected can
> never parse a config (derivation runs at parse time). Registering it now would list
> an unusable base. See the [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** design only. Decision (this session): document-and-stop, per the task's
escape clause — do not force an untested Windows installer subsystem.
**Proposed environment ID:** `windows-host`.

## 1. What already fits the pattern (infrastructure is easy)

The Windows EC2 scenario is a plain `TypedProvisioner`, so the worker side is a
direct copy of the ec2-host recipe:

- Provisioner: `winawshost.Provisioner(...)` returns
  `provisioners.TypedProvisioner[environments.WindowsHost]`
  (`testing/provisioners/aws/host/windows/host.go`).
- Scenario: `scenarios/aws/ec2/windows` `RunWithEnv` — EC2 VM (Windows Server
  2016–2025 or Windows client AMIs), optional ECS Fargate fakeintake, Defender
  disabled by default, optional test-signing / FIPS / Active Directory.
- Worker builder (sketch, mirrors `buildEC2Host` in `cmd/e2ectl-worker/scenarios.go`):

  ```go
  func buildWindowsHost(params string, fixtureConfig fixtures.Config) (Executor, error) {
      p, err := winconfig.Schema.DecodeResolved([]byte(params), "environment.windows-host")
      if err != nil {
          return Executor{}, err
      }
      opts := []windows.RunOption{windows.WithoutAgent()}
      if !fixtureConfig.FakeIntake {
          opts = append(opts, windows.WithoutFakeIntake())
      }
      return fromTyped[windowshost.WindowsHost](winawshost.Provisioner(
          winawshost.WithRunOptions(opts...),
          // OS selection is stack config the scenario reads itself (InfraOSDescriptor):
          // ddinfra:osDescriptor, e.g. "windows-server:2022-e2e" — the same
          // extraConfigParams mechanism the eks builder uses for kubernetesVersion.
          winawshost.WithExtraConfigParams(osDescriptorConfig(p.OS)),
      )), nil
  }
  ```

- CLI driver: embed the shared lifecycle (`cmd/e2ectl/internal/drivers/pulumiworker`)
  like `dockerhost` does; the snapshot exports the same `remoteHost` SSH output.
- Typed schema sketch (`cmd/internal/envconfig/windowshost`):

  ```go
  type Config struct {
      OS           string `yaml:"os" default:"windows-server:2022-e2e" enum:"windows-server:2016-e2e,windows-server:2019-e2e,windows-server:2022-e2e,windows-server:2025-e2e" description:"Windows Server AMI (e2e variants)."`
      InstanceType string `yaml:"instance-type,omitempty" description:"Optional EC2 instance type."`
  }
  ```

## 2. The blocker: no Windows agent installer in e2ectl

Derivation runs at config-parse time; a base with no derivable installer cannot
parse any config, so `windows-host` is **not registered** until one of these exists:

1. **`script` (install script)**: the shared `HostScript` installer runs a
   bash `curl | bash` command (`testing/installers/host/installscript`). Windows
   needs a PowerShell variant (the framework's own Windows path is MSI-only —
   `components/datadog/agent/host_windowsos.go` downloads the MSI from
   `installers_v2.json`/S3 and runs `msiexec` inside Pulumi, which e2ectl
   deliberately keeps out of provisioning). A Windows installer needs:
   - an MSI artifact provider (pipeline S3 download and/or `installers_v2.json`
     version lookup — the Windows install-test suites resolve
     `CURRENT_AGENT_MSI_URL`/`CURRENT_AGENT_PIPELINE`/`CURRENT_AGENT_SOURCE_VERSION`
     today, see `test/new-e2e/tests/windows/AGENTS.md`);
   - `msiexec` over the SSH host client in PowerShell (the Windows VM speaks
     OpenSSH; commands render through the OS runner — no WinRM needed);
   - Windows configuration handling (`C:\ProgramData\Datadog\datadog.yaml`,
     service control instead of systemd masks) in the `configure` package;
   - routing/receiver parity for the Windows agent config paths.
2. **`package` (MSI)**: `localpackage.Target()` explicitly rejects
   Windows-family hosts today (DEB/apt only). An MSI target would reuse the same
   pipeline provider as (1) with the package installer's verified-digest flow.

Recommended derivation once either exists (task contract):

- `version: X` → the Windows installer (MSI from the version table), default
  source `version: <DefaultVersion>`;
- `pipeline: N` → `package` from the pipeline provider if MSI pipelines are
  supported, otherwise rejected with a pointer to `version`;
- `source: true` → rejected (remote source builds not automated, same as the
  other host bases);
- `values` → rejected (no Helm on this base).

## 3. Verification constraints

None of this can be tested without cloud (a Windows VM and a pipeline MSI).
The registration should land together with the installer and be validated with
the same live-run commands as the eks/docker-host bases, plus a Windows-suite
attach (`test/new-e2e/tests/windows/install-test/` style) once the mutator seams
from the migration gap analysis exist.
