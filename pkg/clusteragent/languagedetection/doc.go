// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

/*
Package languagedetection implements the language detection patcher.
The patcher is a component of the language detection feature that is responsible for patching pod owner resources (such as deployments, statefulsets, daemonsets, etc.) with language annotations based on languages detected by the process agent.

The language detection patcher subscribes to workloadmeta events. When it receives a notification from workloadmeta it does the following:
  - Reads the entity data contained in the event
  - Extracts the `detected_languages` and `injectable_languages` from the entity data
  - Constructs the annotations patch
  - Patches the owner resource with language annotations (i.e. deployment, statefulset, etc.)

For more information about the language detection and library injection feature, refer to [this] document.

[this]: https://github.com/DataDog/datadog-agent/blob/main/pkg/languagedetection/util/README.md
*/
package languagedetection
