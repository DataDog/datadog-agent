// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package apps

// v0.0.5's images (published before the test-infra-definitions -> datadog-agent/test/e2e-framework
// repo merge) still carry the old org.opencontainers.image.source label, which fails every
// git.repository_url tag assertion in test/new-e2e/tests/containers (those were already updated to
// expect the post-merge URL). v0.0.6 is main's current pin and should carry the corrected label.
const Version = "v0.0.6"
