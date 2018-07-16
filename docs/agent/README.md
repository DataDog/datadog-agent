# Datadog Agent

## TOC

 * [Changes][changes]
 * [Configuration options][config]
 * [Downgrade][downgrade]
 * [GUI](gui.md)
 * [Missing features][missing-features]
 * [Upgrade][upgrade]

## What is Agent 6?

Agent 6 is the latest major version of the Datadog Agent. The big difference
between Agent 5 and Agent 6 is that Agent 6 is a complete rewrite of the core
Agent in Golang. Don’t worry! We still support Python checks.

## Installation and management

To install the Agent you can either [upgrade][upgrade] from an older version, or start
fresh with a new installation - in this case, please refer to the [official
documentation](https://docs.datadoghq.com/agent/). If you want to go back to
version 5.x, please follow the instructions about [how to downgrade][downgrade].

There are a number of differences between the old major version of the Agent and
the new one - to see what changed, please refer to [this document][changes]. The
way you can configure the Agent changed too, please read [this document][config]
where all the new options are detailed. The latest Agent won't have feature parity
with the previous one at first, you can see the list of what's missing [here][missing-features].

## Secrets Managemet

If you need to retrieve secrets at run-time, please read this [document][secrets].

## Systems

We do not yet build packages for the full gamut of systems that Agent 5 targets.
While some will be dropped as unsupported, others are simply not yet supported.
Agent 6 is currently available on these platforms:

| System | Supported version |
|--------|-------------------|
| Debian x86_64 | version 7 (wheezy) and above (we do not support SysVinit)|
| Ubuntu x86_64 | version 12.04 and above |
| RedHat/CentOS x86_64 | version 6 and above |
| SUSE Enterprise Linux x86_64 | version 11 SP4 and above (we do not support SysVinit)|
| MacOS | 10.10 and above |
| Windows Server 64-bit |  2008 R2 and above |
| Docker | Version 1.12 and higher|
|Kubernetes | Version 1.3 and higher |


[changes]: changes.md
[config]: config.md
[downgrade]: downgrade.md
[missing-features]: missing_features.md
[upgrade]: upgrade.md
[secrets]: secrets.md
