// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This assertion lives in an external test package because pkg/serverless/trace
// imports cmd/serverless-init/cloudservice, which imports this package: the
// production packages cannot reference each other in this direction.
package trace_test

import (
	serverlessinittrace "github.com/DataDog/datadog-agent/cmd/serverless-init/trace"
	serverlesstrace "github.com/DataDog/datadog-agent/pkg/serverless/trace"
)

// The trace agent is matched against SpanModifierSetter at runtime (see
// CloudRunJobs.setSpanModifier), so a signature drift between the two would not
// fail the build — it would silently stop reparenting Cloud Run Jobs spans under
// the job span. This assertion turns that into a compile error.
var _ serverlessinittrace.SpanModifierSetter = (serverlesstrace.ServerlessTraceAgent)(nil)
