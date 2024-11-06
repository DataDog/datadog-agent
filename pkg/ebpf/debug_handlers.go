// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ebpf

import (
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HandleBTFLoaderInfo responds with where the system-probe found BTF data (and
// if it was in a pre-bundled tarball, where within that tarball it came from)
func HandleBTFLoaderInfo(w http.ResponseWriter, _ *http.Request) {
	info, err := GetBTFLoaderInfo()
	if err != nil {
		log.Errorf("unable to get ebpf_btf_loader info: %s", err)
		w.WriteHeader(500)
		return
	}

	io.WriteString(w, info)
}
