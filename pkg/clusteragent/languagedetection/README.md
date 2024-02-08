# Language Detection Patcher

[![CircleCI](https://circleci.com/gh/DataDog/datadog-agent/tree/main.svg?style=svg)](https://circleci.com/gh/DataDog/datadog-agent/tree/main)
[![Build status](https://ci.appveyor.com/api/projects/status/kcwhmlsc0oq3m49p/branch/main?svg=true)](https://ci.appveyor.com/project/Datadog/datadog-agent/branch/main)
[![GoDoc](https://godoc.org/github.com/DataDog/datadog-agent?status.svg)](https://godoc.org/github.com/DataDog/datadog-agent)

The language detection patcher is a component of the language detection feature that is responsible for patching pod owners (such as deployments, statefulsets, daemonsets, etc.) with language annotations based on languages detected by the process agent.

The language detection patcher subscribes to workloadmeta events. When it receives a notification from workloadmeta it does the following:
- Reads the entity data contained in the event
- Extracts the `detected_languages` and `injectable_languages` from the entity data
- Constructs the annotations patch
- Patches the owner (i.e. deployment, statefulset, etc.)

