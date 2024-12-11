// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ignore

package ditypes

/*
#include "../codegen/c/types.h"
*/
import "C"

type BaseEvent C.struct_base_event

const SizeofBaseEvent = C.sizeof_struct_base_event
