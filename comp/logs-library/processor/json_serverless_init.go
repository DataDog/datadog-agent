// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// JSONServerlessInitEncoder is a custom encoder used by serverless-init
// (Google Cloud Run, Azure Container Apps, etc.) for improved performance.
var JSONServerlessInitEncoder Encoder = &jsonServerlessInitEncoder{}

// jsonServerlessInitEncoder transforms a message into a JSON byte array.
// It caches the tags string since tags are constant in serverless-init environments.
// Call SetServerlessInitTagCache when tags change at runtime (e.g. after
// a MicroVM /run hook adds lambda_microvm_id).
//
// cachedTags uses atomic.Pointer so that SetServerlessInitTagCache (called from
// the lifecycle server goroutine on /run) and Encode (called from log processor
// goroutines) are race-free without a lock on the hot read path.
// nil means "not yet initialized"; Encode initialises it with a plain Store on first use.
type jsonServerlessInitEncoder struct {
	cachedTags atomic.Pointer[string]
}

// primeTagsFromMessage atomically primes the cache from a message's computed
// tags, but only if the cache is currently nil. This must be a CAS rather
// than a plain Store: it can race with a concurrent SetServerlessInitTagCache
// call (e.g. from a MicroVM /run hook), and a plain Store could overwrite that
// authoritative, possibly newer, value with this message's stale snapshot.
// The CAS only ever transitions nil -> primed, so it can never clobber a
// value SetServerlessInitTagCache already wrote.
func (j *jsonServerlessInitEncoder) primeTagsFromMessage(tagsStr string) {
	s := tagsStr
	j.cachedTags.CompareAndSwap(nil, &s)
}

// SetServerlessInitTagCache pre-populates the JSONServerlessInitEncoder's cache
// with the provided tags. This must be called (instead of clearing the cache)
// whenever the log tag set changes at runtime (e.g. after /run appends
// lambda_microvm_id). Setting the cache directly prevents in-flight pre-run
// messages — whose origin.tags were snapshotted before the update — from being
// encoded first and re-priming the cache with stale tags.
//
// Pass nil or an empty slice to reset the cache (equivalent to the old
// InvalidateServerlessInitTagCache behaviour; the next Encode call will
// re-populate from the message).
func SetServerlessInitTagCache(tags []string) {
	if enc, ok := JSONServerlessInitEncoder.(*jsonServerlessInitEncoder); ok {
		if len(tags) == 0 {
			enc.cachedTags.Store(nil) // reset: next Encode re-derives from the message
			return
		}
		s := strings.Join(tags, ",")
		enc.cachedTags.Store(&s)
	}
}

// JSON representation of a message for serverless-init.
type jsonServerlessInitPayload struct {
	Message   ValidUtf8Bytes `json:"message"`
	Status    string         `json:"status"`
	Timestamp int64          `json:"timestamp"`
	Hostname  string         `json:"hostname"`
	Service   string         `json:"service,omitempty"`
	Source    string         `json:"ddsource"`
	Tags      string         `json:"ddtags"`
}

// Encode encodes a message into a JSON byte array.
func (j *jsonServerlessInitEncoder) Encode(msg *message.Message, hostname string) error {
	if msg.State != message.StateRendered {
		return errors.New("message passed to encoder isn't rendered")
	}

	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}

	// Hot path: a single atomic load is all that's needed once the cache is primed.
	var tagsStr string
	if p := j.cachedTags.Load(); p != nil {
		tagsStr = *p
	} else {
		// Cache uninitialised (startup or post-reset): derive from the message
		// and prime it for subsequent calls.
		tagsStr = msg.TagsToString()
		j.primeTagsFromMessage(tagsStr)
	}

	encoded, err := json.Marshal(jsonServerlessInitPayload{
		Message:   ValidUtf8Bytes(msg.GetContent()),
		Status:    msg.GetStatus(),
		Timestamp: ts.UnixNano() / nanoToMillis,
		Hostname:  hostname,
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      tagsStr,
	})

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}
