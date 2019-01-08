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

To install the Agent you can either upgrade from an older version, or start
fresh with a new installation - in both cases, please refer to the [official
documentation](https://docs.datadoghq.com/agent/) for your system. If you want to go back to
version 5.x, please follow the instructions about [how to downgrade][downgrade].

There are a number of differences between the old major version of the Agent and
the new one, please refer to:
* [the doc on the general Agent changes][changes]
* [the doc on the differences in the Agent config options][config]
* [the list of Agent 5 features that are not ported to Agent 6][missing-features]

## Secrets Management

If you need to retrieve secrets at run-time, please read this [document][secrets].

## Systems

Agent 6 is currently available on these platforms:

| System | Supported version |
|--------|-------------------|
| Debian x86_64 | version 7 (wheezy) and above (SysVinit support in agent >=6.6.0)|
| Ubuntu x86_64 | version 12.04 and above |
| RedHat/CentOS x86_64 | version 6 and above |
| SUSE Enterprise Linux x86_64 | version 11 SP4 and above (we do not support SysVinit)|
| MacOS | 10.12 and above |
| Windows Server 64-bit |  2008 R2 and above |
| Docker | Version 1.12 and higher|
|Kubernetes | Version 1.3 and higher |


[changes]: changes.md
[config]: config.md
[downgrade]: downgrade.md
[missing-features]: missing_features.md
[upgrade]: upgrade.md
[secrets]: secrets.md
