// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

// commonRoot holds the common root path for the application package model.
// When set, all path getters will return paths relative to this root.
// This is set automatically from the DD_COMMON_ROOT environment variable during init().
var commonRoot string
