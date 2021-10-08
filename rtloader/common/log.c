// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "log.h"

// these must be set by the Agent
static cb_log_t cb_log = NULL;

void _set_log_cb(cb_log_t cb)
{
    cb_log = cb;
}

// Logs a message to the agent logger. Caller is in charge of freeing the
// message if needed.
void agent_log(log_level_t log_level, char *message) {
    if (cb_log == NULL || message == NULL) {
        return;
    }
    cb_log(message, log_level);
}
