# Dell PowerFlex lab (`aws/dell-powerflex`)

All-in-one E2E lab for the released `dell_powerflex` Datadog integration. By
default it boots a **turnkey golden AMI** that comes up as a complete, live
PowerFlex deployment — the Agent emits the full metric set (~556 metrics / 80
families: `system`, `sds`, `storage_pool`, `volume`, `device`,
`protection_domain`, `snapshot`, plus bandwidth/IOPS/latency) under continuous
I/O load, with no bootstrap.

## Architecture

A single framework-provisioned **`m5.metal`** host runs everything via nested
libvirt/KVM (bare metal → hardware `/dev/kvm`; appgate only routes
framework-provisioned hosts, so an ad-hoc host would be unreachable). All
nested-VM disk state lives on a **1.5 TiB gp3 root** so a golden AMI captures
the whole cluster.

Nested on that host:

- **PFMP 4.6.2.1 management platform** — a 3-node MVM (`pfmp-1/2/3`) on an
  isolated libvirt NAT network. Serves the PowerFlex Gateway/REST API behind a
  MetalLB VIP **`10.55.0.40`** (Keycloak realm `powerflex`, client `powerflexUI`).
- **PowerFlex block cluster** — 1 MDM + 3 SDS (`mdm`, `sds-1/2/3`, build
  `4.5-4000.111`) with a protection domain, storage pool, and a volume; deployed
  by PFMP's *Deploy-With-Installation-File* (software-only) flow onto the
  existing-OS nodes (no imaging; version-matched, AMS-seeded gateway).
- **Load generator** — the volume is mapped to an SDC (`scini.ko` built for the
  node kernel) and a `pflexload.service` runs continuous `fio` so the
  performance metrics are non-zero.
- **Datadog Agent (released) on the host** with the `dell_powerflex` check
  pointed at the gateway VIP, emitting the full metric set.

Boot-persistence: all 7 nested VMs `autostart`; `scini`/`fio`/PowerFlex node
services are enabled — so a fresh launch auto-recovers the cluster + load.

Full build rationale and the from-scratch runbook (PFMP appliance bring-up, the
SLES-imaging vs software-only paths, the gateway timeout / MDM cert / SDC
version findings) are in `.agint/labs/dell-powerflex/research/metal-nested.md`
and `.agint/labs/dell-powerflex/exploration/*.md`.

## Files

- `run.go` — scenario `Run`, registered as `aws/dell-powerflex`. Single
  `m5.metal` host, exported as `dd-Host-powerflex`. Defaults to the golden AMI.
- `config/dell_powerflex.yaml` — the check config template (used on the
  from-scratch path; the golden AMI already has a working conf baked in).
- `../../../components/integration/dell-powerflex/` — host virt-stack install,
  NAT network, and the staged `bootstrap.sh` (from-scratch path only).

## Tasks

The single host role is **`aws-powerflex`** (`get_host` resolves
`dd-Host-powerflex`; the aws namer prefixes `aws-`).

```bash
dda inv aws.dell-powerflex.create        # launch the golden AMI on m5.metal
dda inv aws.dell-powerflex.status        # full `datadog-agent status` on the host
dda inv aws.dell-powerflex.check         # run the dell_powerflex check once
dda inv aws.dell-powerflex.reload-check  # restart agent + run the check
dda inv aws.dell-powerflex.exec --role aws-powerflex --command '<cmd>'
dda inv aws.dell-powerflex.ssh  --role aws-powerflex
dda inv aws.dell-powerflex.destroy
```

After `create`, the nested cluster auto-recovers (rke2/PFMP → MDM/SDS → SDC/load
→ gateway); allow a few minutes before the full metric set appears.

## Image selection

- **Default (golden AMI):** `create` launches `defaultGoldenAMI`
  (`ami-06fd51beaaa00b88d`, us-east-1) — the complete working lab.
- **Override the image:** `DELL_POWERFLEX_GOLDEN_AMI=<ami-id> dda inv aws.dell-powerflex.create`.
- **From-scratch build:** `DELL_POWERFLEX_FROM_SCRATCH=1 dda inv aws.dell-powerflex.create`
  provisions a vanilla RHEL9 host + the virt stack + the staged
  `bootstrap.sh`, then the nested PFMP + block cluster are brought up
  interactively per the `.agint` runbook. From-scratch check config is
  env-overridable: `PFMP_GATEWAY_URL` → `powerflex_gateway_url`,
  `POWERFLEX_USERNAME` → `powerflex_username`, `POWERFLEX_PASSWORD` →
  `powerflex_password`.

> The golden AMI is region-specific (us-east-1) and tagged
> `Usage:integration-lab`. To rebuild it, follow the `.agint` runbook and
> `aws ec2 create-image` from a quiesced host.

## Source artifacts (S3)

`s3://dd-vmimport-custom-ami-bucket/pfmp/` (PFMP OVAs) and
`s3://dd-vmimport-custom-ami-bucket/powerflex-rpms/el9/` (ScaleIO RPMs) — used by
the from-scratch path.

This lab is a real E2E lab: no fakeintake.
