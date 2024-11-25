// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef DI_MACROS_H
#define DI_MACROS_H

#define MAX_STRING_SIZE {{ .InstrumentationInfo.InstrumentationOptions.StringMaxSize}}
#define PARAM_BUFFER_SIZE {{ .InstrumentationInfo.InstrumentationOptions.ArgumentsMaxSize}}
#define STACK_DEPTH_LIMIT 10
#define MAX_SLICE_SIZE 1800
#define MAX_SLICE_LENGTH 20

#endif