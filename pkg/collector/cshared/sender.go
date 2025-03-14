// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

/*
#include <stdlib.h>
#include "sender.h"

sender_manager_t *new_sender_manager(void *handle) {
	sender_manager_t *manager = malloc(sizeof(sender_manager_t));
	manager->handle = handle;
	return manager;
}
*/
import "C"
