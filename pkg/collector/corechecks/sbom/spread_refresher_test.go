// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || windows

package sbom

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// TestSpreadRefresherCoversEveryImage checks that spreading the refresh over
// spreadSteps steps still refreshes every image once per period. Dividing the
// image count by spreadSteps used to truncate, so a node with fewer than
// spreadSteps images refreshed nothing at all and inUse never became false
// again once a container had stopped.
func TestSpreadRefresherCoversEveryImage(t *testing.T) {
	cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{
		"sbom.cache_directory": t.TempDir(),
	})
	if sbomscanner.GetGlobalScanner() == nil {
		wmeta := fxutil.Test[option.Option[workloadmeta.Component]](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))
		_, err := sbomscanner.CreateGlobalScanner(cfg, wmeta)
		assert.Nil(t, err)
	}

	for _, imageCount := range []int{0, 1, 9, 10, 15, 100} {
		t.Run(fmt.Sprintf("%d images", imageCount), func(t *testing.T) {
			store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				fx.Provide(func() log.Component { return logmock.New(t) }),
				fx.Provide(func() configcomp.Component { return configcomp.NewMock(t) }),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			for i := 0; i < imageCount; i++ {
				store.Set(&workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainerImageMetadata,
						ID:   fmt.Sprintf("sha256:%064d", i),
					},
				})
			}

			p, err := newProcessor(store, workloadfilterfxmock.SetupMockFilter(t), mocksender.NewMockSender(t, ""), taggerfxmock.SetupFakeTagger(t), cfg, 1, 50*time.Millisecond, time.Second)
			assert.Nil(t, err)
			defer p.stop()

			refresher := newSpreadRefresher(time.Hour, store, p)
			defer refresher.stop()

			for i := 0; i < spreadSteps; i++ {
				refresher.step()
			}

			refreshed := 0
			for _, at := range refresher.refreshTimes {
				if !at.IsZero() {
					refreshed++
				}
			}
			assert.Equal(t, imageCount, refreshed, "every image should be refreshed within one period")
		})
	}
}
