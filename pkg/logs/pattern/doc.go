// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pattern exposes the Logs pipeline's structural tokenizer to
// components that need to construct or compare log patterns.
//
// A Tokenizer is not safe for concurrent use. Tokenize returns caller-owned
// slices, so the result remains valid across later calls on the same Tokenizer.
package pattern
