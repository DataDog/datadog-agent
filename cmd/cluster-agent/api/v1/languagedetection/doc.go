// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

/*
Package languagedetection implements the language detection API handler.
The language detection API handler is responsible for processing requests received from the language detection client.
It parses the requests, extracts detected languages, and pushes them to workload metadata store on the appropriate resource type.
For instance, for a pod that is a child of a deployment, the API handler will push its detected languages to the corresponding deployment entity in workload metadata store.

For more information about the language detection and library injection feature, refer to [this] document.

[this]: https://github.com/DataDog/datadog-agent/blob/main/pkg/languagedetection/util/README.md
*/
package languagedetection
