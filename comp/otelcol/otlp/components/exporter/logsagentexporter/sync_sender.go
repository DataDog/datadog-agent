// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logsagentexporter

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

// SyncSender sends log messages to the logs intake in the calling goroutine.
//
// Send returns one error per message, in the order of msgs. A nil error means the message was
// delivered or filtered out by a processing rule. An error is worth retrying when it wraps a
// client.RetryableError or a context error; any other error is permanent.
type SyncSender interface {
	Send(ctx context.Context, msgs []*message.Message) []error
}

// NewExporterWithSyncSender returns an exporter that sends logs through sender, so that ConsumeLogs
// reports delivery errors, instead of handing them to a logs agent channel.
func NewExporterWithSyncSender(
	set component.TelemetrySettings,
	cfg *Config,
	logSource *sources.LogSource,
	sender SyncSender,
	attributesTranslator *attributes.Translator,
) (*Exporter, error) {
	e, err := NewExporter(set, cfg, logSource, nil, attributesTranslator)
	if err != nil {
		return nil, err
	}
	e.syncSender = sender
	return e, nil
}

const syncBatchFlushTimeout = 200 * time.Millisecond

// SyncSenderQueueConfig adapts the sending queue settings of an exporter that uses a SyncSender.
// The asynchronous pipeline batched records from all requests and sent the batches concurrently; a
// SyncSender sends every request as it comes.
func SyncSenderQueueConfig(queue configoptional.Optional[exporterhelper.QueueBatchConfig], numConsumers, batchSize int) configoptional.Optional[exporterhelper.QueueBatchConfig] {
	if !queue.HasValue() {
		return queue
	}
	cfg := *queue.Get()
	if cfg.NumConsumers == exporterhelper.NewDefaultQueueConfig().NumConsumers {
		cfg.NumConsumers = numConsumers
	}
	if !cfg.Batch.HasValue() {
		cfg.Batch = configoptional.Some(exporterhelper.BatchConfig{
			FlushTimeout: syncBatchFlushTimeout,
			Sizer:        exporterhelper.RequestSizerTypeItems,
			MinSize:      int64(batchSize),
			MaxSize:      int64(batchSize),
		})
	}
	return configoptional.Some(cfg)
}

// sendSync sends msgs through the sync sender. records[i] is the position, in the iteration order of
// ld, of the log record msgs[i] was built from. rejected holds the errors of the log records that
// could not be turned into messages; they fail permanently. When some messages failed with an error
// worth retrying, the returned error carries only their records, so that exporterhelper does not
// resend the delivered ones.
func (e *Exporter) sendSync(ctx context.Context, ld plog.Logs, msgs []*message.Message, records []int, rejected []error) error {
	if len(msgs) == 0 && len(rejected) == 0 {
		return nil
	}
	var causes []error
	var messages []string
	// Records that fail for the same reason, such as messages sent in the same payload, share their error.
	addCause := func(err error) {
		if msg := err.Error(); !slices.Contains(messages, msg) {
			messages = append(messages, msg)
			causes = append(causes, err)
		}
	}
	for _, err := range rejected {
		addCause(err)
	}
	failed := len(rejected)
	retry := make([]bool, ld.LogRecordCount())
	retried := 0
	var errs []error
	if len(msgs) > 0 {
		errs = e.syncSender.Send(ctx, msgs)
	}
	for i, err := range errs {
		if err == nil {
			continue
		}
		failed++
		addCause(err)
		if isRetryable(err) {
			retry[records[i]] = true
			retried++
		}
	}
	if failed == 0 {
		return nil
	}

	err := scrubError(fmt.Errorf("failed to send %d of %d log records: %w", failed, len(msgs)+len(rejected), errors.Join(causes...)))
	if retried == 0 {
		return consumererror.NewPermanent(err)
	}
	if dropped := failed - retried; dropped > 0 {
		e.set.Logger.Warn("dropping log records rejected permanently, retrying the others",
			zap.Int("dropped_log_records", dropped),
			zap.Int("retried_log_records", retried),
			zap.Error(err))
	}
	return consumererror.NewLogs(err, selectRecords(ld, retry))
}

func isRetryable(err error) bool {
	var retryable *client.RetryableError
	return errors.As(err, &retryable) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// selectRecords returns a copy of ld that holds only the log records whose position, in iteration
// order, is set in keep.
func selectRecords(ld plog.Logs, keep []bool) plog.Logs {
	out := plog.NewLogs()
	position := 0
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		var outRL plog.ResourceLogs
		hasRL := false
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			var outSL plog.ScopeLogs
			hasSL := false
			for k := 0; k < sl.LogRecords().Len(); k++ {
				if keep[position] {
					if !hasRL {
						outRL = out.ResourceLogs().AppendEmpty()
						rl.Resource().CopyTo(outRL.Resource())
						outRL.SetSchemaUrl(rl.SchemaUrl())
						hasRL = true
					}
					if !hasSL {
						outSL = outRL.ScopeLogs().AppendEmpty()
						sl.Scope().CopyTo(outSL.Scope())
						outSL.SetSchemaUrl(sl.SchemaUrl())
						hasSL = true
					}
					sl.LogRecords().At(k).CopyTo(outSL.LogRecords().AppendEmpty())
				}
				position++
			}
		}
	}
	return out
}
