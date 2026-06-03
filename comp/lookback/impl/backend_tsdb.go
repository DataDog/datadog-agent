// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	lookback "github.com/DataDog/datadog-agent/comp/lookback/def"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// tsdbBackend implements timeSeriesBackend on top of Prometheus TSDB.
//
// Timestamp precision: TSDB stores milliseconds. Our µs timestamps are truncated
// by 3 orders of magnitude (µs → ms). At ≥1 sample/ms this causes collisions;
// in practice check intervals are seconds, so precision loss is negligible.
type tsdbBackend struct {
	db     *tsdb.DB
	mu     sync.Mutex       // guards app
	app    storage.Appender // batched appender; committed every commitInterval
	wg     sync.WaitGroup
	cancel context.CancelFunc
	log    log.Component
}

const tsdbCommitInterval = 500 * time.Millisecond

func newTSDBBackend(cfg storeConfig, l log.Component) (*tsdbBackend, error) {
	opts := tsdb.DefaultOptions()
	opts.RetentionDuration = int64(cfg.maxAge / time.Millisecond)
	opts.MaxBytes = cfg.maxDiskBytes
	blockDur := int64(cfg.rotationInterval / time.Millisecond)
	if blockDur > 0 {
		opts.MinBlockDuration = blockDur
		opts.MaxBlockDuration = blockDur
	}

	dir := filepath.Join(cfg.baseDir, "tsdb")
	db, err := tsdb.Open(dir, slog.Default(), nil, opts, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &tsdbBackend{
		db:     db,
		app:    db.Appender(ctx),
		cancel: cancel,
		log:    l,
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ticker := time.NewTicker(tsdbCommitInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.mu.Lock()
				if err := b.app.Commit(); err != nil && l != nil {
					l.Warnf("lookback tsdb: commit error: %v", err)
				}
				b.app = db.Appender(ctx)
				b.mu.Unlock()
			}
		}
	}()

	return b, nil
}

func (b *tsdbBackend) writeSample(name string, tags []string, tsUs int64, value float64) {
	lset := tagsToLabels(name, tags)
	tsMs := tsUs / 1000

	b.mu.Lock()
	if _, err := b.app.Append(0, lset, tsMs, value); err != nil && b.log != nil {
		b.log.Warnf("lookback tsdb: append error: %v", err)
	}
	b.mu.Unlock()
}

func (b *tsdbBackend) flush(
	ctx context.Context, name string, tags []string,
	startUs, stopUs, intervalUs int64,
) ([]lookback.Bucket, error) {
	// Commit pending samples so the querier sees them.
	b.mu.Lock()
	if err := b.app.Commit(); err != nil && b.log != nil {
		b.log.Warnf("lookback tsdb: pre-flush commit error: %v", err)
	}
	b.app = b.db.Appender(ctx)
	b.mu.Unlock()

	startMs := startUs / 1000
	stopMs := stopUs / 1000

	q, err := b.db.Querier(startMs, stopMs)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	matchers, err := buildMatchers(name, tags)
	if err != nil {
		return nil, err
	}

	ss := q.Select(ctx, false, nil, matchers...)
	if err := ss.Err(); err != nil {
		return nil, err
	}

	if intervalUs <= 0 {
		intervalUs = defaultIntervalUs
	}

	// Collect all (bucketTs, value) pairs grouped by series labels string.
	type seriesBuckets struct {
		name string
		tags []string
		// buckets by bucket-ts (µs boundary)
		data map[int64]*lookback.Bucket
	}
	var allSeries []seriesBuckets

	for ss.Next() {
		series := ss.At()
		lset := series.Labels()
		sName := lset.Get(labels.MetricName)
		sTags := labelsToTags(lset)

		sb := seriesBuckets{
			name: sName,
			tags: sTags,
			data: make(map[int64]*lookback.Bucket),
		}

		it := series.Iterator(nil)
		for vt := it.Next(); vt != chunkenc.ValNone; vt = it.Next() {
			if vt != chunkenc.ValFloat {
				continue
			}
			tsMs, val := it.At()
			tsUs2 := tsMs * 1000
			if tsUs2 < startUs || tsUs2 >= stopUs {
				continue
			}
			tsBucket := (tsUs2 / intervalUs) * intervalUs
			if b2, ok := sb.data[tsBucket]; ok {
				b2.Count++
				b2.Sum += val
				if val < b2.Min {
					b2.Min = val
				}
				if val > b2.Max {
					b2.Max = val
				}
			} else {
				sb.data[tsBucket] = &lookback.Bucket{
					Name:  sName,
					Tags:  sTags,
					Ts:    tsBucket,
					Count: 1,
					Sum:   val,
					Min:   val,
					Max:   val,
				}
			}
		}
		if err := it.Err(); err != nil && b.log != nil {
			b.log.Warnf("lookback tsdb: iterator error: %v", err)
		}
		if len(sb.data) > 0 {
			allSeries = append(allSeries, sb)
		}
	}
	if err := ss.Err(); err != nil {
		return nil, err
	}

	if len(allSeries) == 0 {
		return nil, lookback.ErrNoData
	}

	// Flatten and sort by (name, tags, ts) for deterministic output.
	var buckets []lookback.Bucket
	for _, sb := range allSeries {
		tss := make([]int64, 0, len(sb.data))
		for ts := range sb.data {
			tss = append(tss, ts)
		}
		slices.Sort(tss)
		for _, ts := range tss {
			buckets = append(buckets, *sb.data[ts])
		}
	}
	slices.SortFunc(buckets, func(a, b lookback.Bucket) int {
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return cmp.Compare(a.Ts, b.Ts)
	})

	return buckets, nil
}

// startRotationTimer is a no-op for TSDB — it manages its own compaction.
func (b *tsdbBackend) startRotationTimer() {}

func (b *tsdbBackend) stop(_ context.Context) error {
	b.cancel()
	b.wg.Wait()
	b.mu.Lock()
	_ = b.app.Commit()
	b.mu.Unlock()
	return b.db.Close()
}

// tagsToLabels converts a metric name and "key:value" tag slice to labels.Labels.
// Tags that do not contain ":" are appended as label name with empty value.
func tagsToLabels(name string, tags []string) labels.Labels {
	lb := labels.NewBuilder(labels.EmptyLabels())
	lb.Set(labels.MetricName, name)
	for _, t := range tags {
		idx := strings.IndexByte(t, ':')
		if idx < 0 {
			lb.Set(t, "")
		} else {
			lb.Set(t[:idx], t[idx+1:])
		}
	}
	return lb.Labels()
}

// labelsToTags converts labels.Labels back to "key:value" tag slice,
// skipping the __name__ label.
func labelsToTags(lset labels.Labels) []string {
	var tags []string
	lset.Range(func(l labels.Label) {
		if l.Name == labels.MetricName {
			return
		}
		if l.Value == "" {
			tags = append(tags, l.Name)
		} else {
			tags = append(tags, l.Name+":"+l.Value)
		}
	})
	return tags
}

// buildMatchers constructs Prometheus label matchers from name and tag filter.
func buildMatchers(name string, tags []string) ([]*labels.Matcher, error) {
	var matchers []*labels.Matcher

	// Name matcher: exact or glob-derived regex.
	if strings.ContainsAny(name, "*?[") {
		re, err := globToRegex(name)
		if err != nil {
			return nil, err
		}
		m, err := labels.NewMatcher(labels.MatchRegexp, labels.MetricName, re)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, m)
	} else {
		matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, name))
	}

	// Tag matchers: each "key:value" tag becomes MatchEqual.
	for _, t := range tags {
		idx := strings.IndexByte(t, ':')
		if idx < 0 {
			matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, t, ""))
		} else {
			matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, t[:idx], t[idx+1:]))
		}
	}
	return matchers, nil
}

// globToRegex converts a glob pattern (*, ?, [...]) to a full-match regex string.
func globToRegex(glob string) (string, error) {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(glob); i++ {
		ch := glob[i]
		switch ch {
		case '*':
			sb.WriteString("[^.]*") // Datadog metric names use dots as separators
		case '?':
			sb.WriteString("[^.]")
		case '[':
			// Pass character class through verbatim.
			end := strings.IndexByte(glob[i:], ']')
			if end < 0 {
				return "", fmt.Errorf("lookback: unclosed '[' in glob pattern %q", glob)
			}
			sb.WriteString(glob[i : i+end+1])
			i += end
		default:
			sb.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	sb.WriteString("$")
	return sb.String(), nil
}
