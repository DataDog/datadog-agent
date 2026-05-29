// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checksketchsource contains e2e tests verifying that distribution metrics
// submitted by a check carry the correct origin metadata (OriginCategory) in the
// sketch payload delivered to fakeintake.
//
// This is a regression test for the bug in CheckSampler.newSketchSeries where
// ctx.source was not copied to the SketchSeries, causing all check-submitted
// distributions to arrive with OriginCategory=0 regardless of the check's
// actual MetricSource.
package checksketchsource
