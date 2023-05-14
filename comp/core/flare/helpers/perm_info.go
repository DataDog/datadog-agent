// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

// permissionsInfos holds permissions info about the files shipped
// in the flare.
// The key is the filepath of the file.
type permissionsInfos map[string]*filePermsInfo
