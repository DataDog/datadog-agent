# Datadog Agent

**Important**: the new Agent is currently in Beta, we recommend to check out [this
document](../beta.md) before starting using it.

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


[changes]: changes.md
[config]: config.md
[downgrade]: downgrade.md
[missing-features]: missing_features.md
[upgrade]: upgrade.md
