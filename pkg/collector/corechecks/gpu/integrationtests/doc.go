// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

/*
Package integrationtests contains integration tests for the GPU core check
that require real GPU hardware to run. These tests are separated from the unit
tests in the parent package to allow running them on dedicated GPU runners.

These tests validate the GPU check's interaction with real NVML library and
GPU hardware, including metrics collection and device enumeration.

These tests are NOT run as part of the regular CI pipeline on main branch.
They run:
  - On nightly pipelines
  - On feature branches when GPU-related code changes
  - Manually when triggered

To run these tests locally, you need a machine with:
  - A supported NVIDIA GPU
  - NVIDIA drivers installed
  - NVML library available
*/
package integrationtests
