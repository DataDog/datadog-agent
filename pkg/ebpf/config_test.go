// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func TestBTFOutputDirPinned(t *testing.T) {
	for _, source := range []string{"default", "config", "environment", "pinned"} {
		t.Run(source, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			var output bytes.Buffer
			logger, err := log.LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat(&output, log.WarnLvl)
			require.NoError(t, err)
			log.SetupLogger(logger, "warn")
			t.Cleanup(func() { log.SetupLogger(log.Default(), "info") })
			btfOutputDirDeprecationOnce = sync.Once{}
			t.Cleanup(func() { btfOutputDirDeprecationOnce = sync.Once{} })
			switch source {
			case "config":
				cfg.Set("system_probe_config.btf_output_dir", "/custom/btf", model.SourceFile)
			case "environment":
				t.Setenv("DD_SYSTEM_PROBE_BTF_OUTPUT_DIR", "/custom/btf")
			case "pinned":
				cfg.Set("system_probe_config.btf_output_dir", BTFCacheDir, model.SourceFile)
			}
			for range 2 {
				require.Equal(t, BTFCacheDir, NewConfig().BTFOutputDir)
			}
			wantWarnings := 0
			if source == "config" || source == "environment" {
				wantWarnings = 1
			}
			require.Equal(t, wantWarnings, strings.Count(output.String(), "btf_output_dir is deprecated and ignored"), output.String())
		})
	}
}
