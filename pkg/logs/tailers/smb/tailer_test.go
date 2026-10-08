// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package smb

import (
	"context"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/processor"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client/fake"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

const testPath = "app/app.log"

type testTailer struct {
	*Tailer
	share *fake.Share
	out   chan *message.Message
}

// newTestTailer returns a started tailer of testPath on share, reading at
// offset. opts adjusts the options before the tailer is built.
func newTestTailer(t *testing.T, share *fake.Share, fileID uint64, offset int64, opts ...func(*TailerOptions)) *testTailer {
	t.Helper()
	configmock.New(t)
	source := sources.NewReplaceableSource(sources.NewLogSource("smb-test", &config.LogsConfig{
		Type:    config.SMBType,
		Path:    "app/*.log",
		Source:  "demo",
		Service: "demo-app",
		SMB:     &config.SMBConfig{Host: "Files.Example.COM", Share: "Logs", Username: "acct", Password: "hunter2-secret"},
	}))
	info := status.NewInfoRegistry()
	out := make(chan *message.Message, 100)
	options := &TailerOptions{
		Source:          source,
		Client:          share.NewSession(),
		Host:            "Files.Example.COM",
		Share:           "Logs",
		Path:            testPath,
		File:            client.Identity{FileID: fileID},
		OutputChan:      out,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
	}
	for _, opt := range opts {
		opt(options)
	}
	tl := NewTailer(options)
	tl.Start(offset)
	t.Cleanup(tl.Stop)
	return &testTailer{Tailer: tl, share: share, out: out}
}

// next returns the next forwarded message.
func (tt *testTailer) next(t *testing.T) *message.Message {
	t.Helper()
	select {
	case msg := <-tt.out:
		return msg
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no message forwarded")
		return nil
	}
}

// lines returns the content of the next n forwarded messages.
func (tt *testTailer) lines(t *testing.T, n int) []string {
	t.Helper()
	var got []string
	for range n {
		got = append(got, string(tt.next(t).GetContent()))
	}
	return got
}

// assertNoMessage stops the tailer, which flushes the decoder, and checks that
// nothing more was forwarded.
func (tt *testTailer) assertNoMessage(t *testing.T) {
	t.Helper()
	tt.Stop()
	select {
	case msg := <-tt.out:
		assert.Failf(t, "unexpected message", "%q", msg.GetContent())
	default:
	}
}

func entryOf(t *testing.T, share *fake.Share, p string) *client.Entry {
	t.Helper()
	e, ok := share.Stat(p)
	require.True(t, ok, "%s does not exist", p)
	return &e
}

// fileOf returns the identity of the file now at p.
func fileOf(t *testing.T, share *fake.Share, p string) client.Identity {
	t.Helper()
	return entryOf(t, share, p).Identity()
}

func TestIdentifier(t *testing.T) {
	assert.Equal(t, "smb://files.example.com/logs/app/app.log", Identifier("Files.Example.COM", "Logs", "app/app.log"))
	assert.Equal(t, "smb://h/s/a.log", Identifier("h", "s", "a.log"))
}

func TestOffsetEncoding(t *testing.T) {
	created := time.Date(2026, 10, 6, 12, 0, 0, 123456700, time.UTC).UnixNano()
	assert.Equal(t, "v2:100:"+strconv.FormatInt(created, 10)+":42", EncodeOffset(client.Identity{FileID: 100, Created: created}, 42))
	assert.Equal(t, "v2:100:0:42", EncodeOffset(client.Identity{FileID: 100}, 42))
	for _, tc := range []struct {
		in     string
		file   client.Identity
		offset int64
		ok     bool
	}{
		{in: EncodeOffset(client.Identity{FileID: 100, Created: created}, 42), file: client.Identity{FileID: 100, Created: created}, offset: 42, ok: true},
		{in: "v2:100:0:42", file: client.Identity{FileID: 100}, offset: 42, ok: true},
		// Earlier versions: v1 offsets carry no creation time, bare offsets
		// no FileId either.
		{in: "v1:100:42", file: client.Identity{FileID: 100}, offset: 42, ok: true},
		{in: "v1:0:7", file: client.Identity{}, offset: 7, ok: true},
		{in: "1234", file: client.Identity{}, offset: 1234, ok: true},
		{in: ""},
		{in: "v1:100"},
		{in: "v1:x:1"},
		{in: "v1:1:-3"},
		{in: "v1:1:2:3"},
		{in: "v2:100:42"},
		{in: "v2:100:x:42"},
		{in: "v2:100:1:-3"},
		{in: "v3:1:2:3"},
		{in: "-3"},
		{in: "garbage"},
	} {
		file, offset, ok := DecodeOffset(tc.in)
		assert.Equal(t, tc.ok, ok, tc.in)
		if tc.ok {
			assert.Equal(t, tc.file, file, tc.in)
			assert.Equal(t, tc.offset, offset, tc.in)
		}
	}
}

func TestPollForwardsLinesWithOffsetsAndTags(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\ntwo\n"))
	file := fileOf(t, share, testPath)
	tt := newTestTailer(t, share, id, 0)

	outcome, err := tt.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.Equal(t, OutcomeRead, outcome)
	assert.Equal(t, file, tt.Identity(), "the creation time is adopted from the first read")

	first, second := tt.next(t), tt.next(t)
	assert.Equal(t, "one", string(first.GetContent()))
	assert.Equal(t, "two", string(second.GetContent()))
	assert.Equal(t, "smb://files.example.com/logs/app/app.log", second.Origin.Identifier)
	assert.Equal(t, EncodeOffset(file, 4), first.Origin.Offset)
	assert.Equal(t, EncodeOffset(file, 8), second.Origin.Offset)
	assert.Contains(t, second.Origin.Tags(), "filename:app.log")
	assert.Contains(t, second.Origin.Tags(), "dirname:smb://files.example.com/logs/app")
	assert.True(t, tt.CaughtUp())

	share.Append(testPath, []byte("three\n"))
	_, err = tt.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)
	third := tt.next(t)
	assert.Equal(t, "three", string(third.GetContent()))
	assert.Equal(t, EncodeOffset(file, 14), third.Origin.Offset)
	assert.EqualValues(t, 14, tt.Offset())
	assert.EqualValues(t, 14, tt.Source().BytesRead.Get())
	assert.Zero(t, share.OpenHandles())
}

func TestPollJoinsALineSplitAcrossPolls(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("hel"))
	tt := newTestTailer(t, share, id, 0)

	_, err := tt.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)
	share.Append(testPath, []byte("lo\n"))
	_, err = tt.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)

	assert.Equal(t, []string{"hello"}, tt.lines(t, 1))
}

func TestPollReadsInChunksWithinItsBudget(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("aaaa\nbbbb\ncccc\n")) // 15 bytes
	tt := newTestTailer(t, share, id, 0, func(o *TailerOptions) {
		o.ChunkSize = 5
		o.PollBudget = 10
	})

	_, err := tt.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.EqualValues(t, 10, tt.Offset(), "the poll budget bounds one poll")
	assert.Equal(t, 2, share.Calls(fake.OpReadAt))
	assert.False(t, tt.CaughtUp())

	_, err = tt.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.EqualValues(t, 15, tt.Offset())
	assert.Equal(t, 3, share.Calls(fake.OpReadAt), "no extra read to discover the end of the file")
	assert.True(t, tt.CaughtUp())
	assert.Equal(t, []string{"aaaa", "bbbb", "cccc"}, tt.lines(t, 3))
}

func TestPollSkipsUnchangedListingsAndForcesReads(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\n"))
	tt := newTestTailer(t, share, id, 0, func(o *TailerOptions) { o.ForceReadEvery = 3 })
	ctx := context.Background()

	_, err := tt.Poll(ctx, entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.Equal(t, 1, share.Calls(fake.OpReadAt))

	// The listing keeps reporting the size already read, although the file
	// grew: a stale size, as for a file another client holds open.
	stale := *entryOf(t, share, testPath)
	share.Append(testPath, []byte("two\n"))
	for range 2 {
		outcome, err := tt.Poll(ctx, &stale)
		require.NoError(t, err)
		assert.Equal(t, OutcomeUnchanged, outcome)
	}
	assert.Equal(t, 1, share.Calls(fake.OpReadAt), "no read while the listing shows nothing new")

	outcome, err := tt.Poll(ctx, &stale)
	require.NoError(t, err)
	assert.Equal(t, OutcomeRead, outcome, "every third poll reads anyway")
	assert.Equal(t, 2, share.Calls(fake.OpReadAt))
	assert.Equal(t, []string{"one", "two"}, tt.lines(t, 2))
}

func TestPollReadsWhenTheListedSizeShrinks(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\ntwo\n"))
	tt := newTestTailer(t, share, id, 0)
	ctx := context.Background()

	_, err := tt.Poll(ctx, entryOf(t, share, testPath))
	require.NoError(t, err)
	require.NoError(t, share.Truncate(testPath, 0))
	outcome, err := tt.Poll(ctx, entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.Equal(t, OutcomeTruncated, outcome)
	assert.EqualValues(t, 8, tt.Offset(), "a truncation is reported, not handled, by the tailer")
}

func TestPollReportsAnIdentityChangeWithoutForwarding(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\n"))
	tt := newTestTailer(t, share, id, 0)

	// Replaced after the listing, before the read: the listing entry still
	// shows the old FileId.
	listed := entryOf(t, share, testPath)
	share.Recreate(testPath, 0)
	share.Append(testPath, []byte("other file\n"))

	outcome, err := tt.Poll(context.Background(), listed)
	require.NoError(t, err)
	assert.Equal(t, OutcomeIdentityChanged, outcome)
	assert.Zero(t, tt.Offset())
	assert.Equal(t, id, tt.FileID())

	// A listing that already shows the new file is reported without a read.
	reads := share.Calls(fake.OpReadAt)
	outcome, err = tt.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.Equal(t, OutcomeIdentityChanged, outcome)
	assert.Equal(t, reads, share.Calls(fake.OpReadAt))
	tt.assertNoMessage(t)
}

func TestPollAdoptsTheFileIDOfTheFirstRead(t *testing.T) {
	share := fake.New()
	share.SetListingFileIDs(false)
	id := share.Write(testPath, []byte("one\n"))
	tt := newTestTailer(t, share, 0, 0)

	entries, err := share.NewSession().ListDir(context.Background(), "app")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	_, err = tt.Poll(context.Background(), &entries[0])
	require.NoError(t, err)
	assert.Equal(t, id, tt.FileID())
	assert.Equal(t, fileOf(t, share, testPath), tt.Identity())
	assert.Equal(t, EncodeOffset(fileOf(t, share, testPath), 4), tt.next(t).Origin.Offset)
}

// TestPollReportsAFileIDReusedByANewFile covers a file deleted and recreated
// under the same FileId, as Samba does with inode numbers, holding more than
// the tailer read: the creation time shows it is another file, whether the
// listing or only the read shows it.
func TestPollReportsAFileIDReusedByANewFile(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\n"))
	tt := newTestTailer(t, share, id, 0)
	ctx := context.Background()
	_, err := tt.Poll(ctx, entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.Equal(t, []string{"one"}, tt.lines(t, 1))

	// Listed with a line the tailer did not read yet, then replaced before
	// the read.
	share.Append(testPath, []byte("two\n"))
	listed := entryOf(t, share, testPath)
	share.Recreate(testPath, id)
	share.Append(testPath, []byte("other file\n"))
	require.Equal(t, id, entryOf(t, share, testPath).FileID)

	reads := share.Calls(fake.OpReadAt)
	outcome, err := tt.Poll(ctx, entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.Equal(t, OutcomeIdentityChanged, outcome, "the listing shows the new creation time")
	assert.Equal(t, reads, share.Calls(fake.OpReadAt), "without a read")

	outcome, err = tt.Poll(ctx, listed)
	require.NoError(t, err)
	assert.Equal(t, OutcomeIdentityChanged, outcome, "the read shows the new creation time")
	assert.EqualValues(t, 4, tt.Offset())
	tt.assertNoMessage(t)
}

func TestPollResumesAfterAnErrorWithoutResending(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\n"))
	tt := newTestTailer(t, share, id, 0)
	ctx := context.Background()

	_, err := tt.Poll(ctx, entryOf(t, share, testPath))
	require.NoError(t, err)
	share.Append(testPath, []byte("two\n"))
	share.FailNext(fake.OpReadAt, fake.ErrTransient)
	_, err = tt.Poll(ctx, entryOf(t, share, testPath))
	require.Error(t, err)
	assert.Equal(t, client.ErrTransient, client.Classify(err))
	assert.EqualValues(t, 4, tt.Offset())

	_, err = tt.Poll(ctx, entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two"}, tt.lines(t, 2))
	tt.assertNoMessage(t)
}

func TestDrainingMessagesCommitNoOffset(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\n"))
	tt := newTestTailer(t, share, id, 0)
	assert.Equal(t, "smb://files.example.com/logs/app/app.log", tt.GetID())
	tt.StartDraining()
	assert.Equal(t, "smb://files.example.com/logs/app/app.log (rotated, FileId "+strconv.FormatUint(id, 10)+")", tt.GetID(),
		"a drain is listed apart from the path's new tailer")
	assert.Equal(t, "smb://files.example.com/logs/app/app.log", tt.Identifier())
	assert.Contains(t, tt.GetInfo().Rendered(), "Draining Since")
	require.NoError(t, share.Rename(testPath, "app/app.log.1"))
	tt.SetReadPath("app/app.log.1")

	// A draining tailer reads even when the listing shows nothing new.
	listed := entryOf(t, share, "app/app.log.1")
	listed.Size = 0
	_, err := tt.Poll(context.Background(), listed)
	require.NoError(t, err)

	msg := tt.next(t)
	assert.Equal(t, "one", string(msg.GetContent()))
	assert.Empty(t, msg.Origin.Identifier)
	assert.Empty(t, msg.Origin.Offset)
	assert.Contains(t, msg.Origin.Tags(), "filename:app.log", "tags keep the original file name")
	assert.Equal(t, "app/app.log.1", tt.ReadPath())
	assert.Equal(t, testPath, tt.Path())
}

func TestDrainCommitsUnderTheIdentifierItIsGiven(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\ntwo\n"))
	file := fileOf(t, share, testPath)
	tt := newTestTailer(t, share, id, 4)
	assert.Equal(t, tt.Identifier(), tt.CommitIdentifier())
	tt.StartDraining()
	assert.Empty(t, tt.CommitIdentifier())

	require.NoError(t, share.Rename(testPath, "app/app.log.1"))
	tt.SetReadPath("app/app.log.1")
	rotated := "smb://files.example.com/logs/app/app.log.1"
	tt.CommitTo(rotated)
	_, err := tt.Poll(context.Background(), entryOf(t, share, "app/app.log.1"))
	require.NoError(t, err)

	msg := tt.next(t)
	assert.Equal(t, "two", string(msg.GetContent()))
	assert.Equal(t, rotated, msg.Origin.Identifier)
	assert.Equal(t, EncodeOffset(file, 8), msg.Origin.Offset, "an offset in the drained file, which sits at that path")
	assert.Contains(t, msg.Origin.Tags(), "filename:app.log", "tags keep the original file name")
	assert.Equal(t, "smb://files.example.com/logs/app/app.log (rotated, FileId "+strconv.FormatUint(id, 10)+")", tt.GetID())
	assert.EqualValues(t, 8, tt.Offset())
}

// TestCommitted checks that Committed tells whether a message forwarded
// commits under the tailer's own identifier, which the launcher relies on to
// know what the registry entry of the tailer's path will hold.
func TestCommitted(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\n"))
	active := newTestTailer(t, share, id, 0)
	assert.False(t, active.Committed())
	_, err := active.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)
	active.next(t)
	assert.True(t, active.Committed())

	drained := newTestTailer(t, share, id, 0)
	drained.StartDraining()
	drained.CommitTo("smb://files.example.com/logs/app/app.log.1")
	_, err = drained.Poll(context.Background(), nil)
	require.NoError(t, err)
	assert.NotEqual(t, drained.Identifier(), drained.next(t).Origin.Identifier)
	assert.False(t, drained.Committed(), "its message commits under another identifier")

	drained.CommitTo(drained.Identifier()) // its file is back at its path
	share.Append(testPath, []byte("two\n"))
	_, err = drained.Poll(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, drained.Identifier(), drained.next(t).Origin.Identifier)
	assert.True(t, drained.Committed())
}

// keepContent is a processor encoder that leaves messages as they are.
type keepContent struct{}

func (keepContent) Encode(*message.Message, string) error { return nil }

// processorKeeps reports whether a logs processor with the global processing
// rules globalRules sends msg on, rather than dropping it before the auditor
// commits its offset. It changes msg as the processor does.
func processorKeeps(globalRules []*config.ProcessingRule, msg *message.Message) bool {
	in := make(chan *message.Message, 1)
	out := make(chan *message.Message, 1)
	host, _ := hostnamemock.NewMock("test-host")
	p := processor.New(nil, in, out, globalRules, keepContent{}, &diagnostic.NoopMessageReceiver{}, host, metrics.NewNoopPipelineMonitor(""), "")
	in <- msg
	p.Flush(context.Background())
	return len(out) == 1
}

func compiledRules(t *testing.T, rules ...*config.ProcessingRule) []*config.ProcessingRule {
	t.Helper()
	require.NoError(t, config.CompileProcessingRules(rules))
	return rules
}

// TestCommittedCountsOnlyMessagesTheProcessorKeeps checks that a message the
// processing rules drop does not count for Committed: the processor drops it
// before the auditor commits its offset, so the registry entry of the
// tailer's path does not change. Each case's lines are checked against a real
// logs processor.
func TestCommittedCountsOnlyMessagesTheProcessorKeeps(t *testing.T) {
	for _, tc := range []struct {
		name        string
		globalRules []*config.ProcessingRule
		sourceRules []*config.ProcessingRule
		maxSize     int
		writes      []string // each decoded into one message
		committed   []bool   // Committed after each write
	}{
		{
			name:        "exclude_at_match",
			sourceRules: []*config.ProcessingRule{{Type: config.ExcludeAtMatch, Name: "x", Pattern: "^skip"}},
			writes:      []string{"skip one\n", "keep two\n", "skip three\n"},
			committed:   []bool{false, true, true},
		},
		{
			name:        "global exclude_at_match",
			globalRules: []*config.ProcessingRule{{Type: config.ExcludeAtMatch, Name: "x", Pattern: "^skip"}},
			writes:      []string{"skip one\n", "keep two\n"},
			committed:   []bool{false, true},
		},
		{
			name:        "global mask_sequences before the source's exclude_at_match",
			globalRules: []*config.ProcessingRule{{Type: config.MaskSequences, Name: "m", Pattern: "token=\\w+", ReplacePlaceholder: "token=[masked]"}},
			sourceRules: []*config.ProcessingRule{{Type: config.ExcludeAtMatch, Name: "x", Pattern: "token=abc"}},
			writes:      []string{"token=abc\n"},
			committed:   []bool{true},
		},
		{
			name:        "include_at_match",
			sourceRules: []*config.ProcessingRule{{Type: config.IncludeAtMatch, Name: "i", Pattern: "^keep"}},
			writes:      []string{"skip one\n", "keep two\n"},
			committed:   []bool{false, true},
		},
		{
			name: "mask_sequences before exclude_at_match",
			sourceRules: []*config.ProcessingRule{
				{Type: config.MaskSequences, Name: "m", Pattern: "token=\\w+", ReplacePlaceholder: "token=[masked]"},
				{Type: config.ExcludeAtMatch, Name: "x", Pattern: "token=abc"},
			},
			writes:    []string{"token=abc\n"},
			committed: []bool{true},
		},
		{
			name:        "exclude_truncated",
			sourceRules: []*config.ProcessingRule{{Type: config.ExcludeTruncated, Name: "t"}},
			maxSize:     16,
			// The decoder sends the first 16 bytes of the line as a
			// truncated message, then the rest as a message of its own.
			writes:    []string{"a 16-byte chunk!", "short\n"},
			committed: []bool{false, true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			share := fake.New()
			id := share.Write(testPath, nil)
			globalRules := compiledRules(t, tc.globalRules...)
			tt := newTestTailer(t, share, id, 0, func(o *TailerOptions) {
				o.ProcessingRules = globalRules
				cfg := o.Source.UnderlyingSource().Config
				cfg.ProcessingRules = compiledRules(t, tc.sourceRules...)
				if tc.maxSize > 0 {
					cfg.MaxMessageSizeBytes = &tc.maxSize
					o.Decoder = decoder.NewDecoderFromSource(o.Source, o.Info)
				}
			})
			kept := false
			for i, write := range tc.writes {
				share.Append(testPath, []byte(write))
				_, err := tt.Poll(context.Background(), nil)
				require.NoError(t, err)
				msg := tt.next(t)
				keeps := keptByRules(msg, globalRules, tc.sourceRules)
				require.Equal(t, processorKeeps(globalRules, msg), keeps, "the processor and keptByRules agree on %q", msg.GetContent())
				kept = kept || keeps
				assert.Equal(t, tc.committed[i], kept, "the processor sent a message of the writes up to %q", write)
				assert.Equal(t, tc.committed[i], tt.Committed(), "after %q", write)
			}
			tt.assertNoMessage(t)
		})
	}
}

// stuckDecoder never accepts input, like a decoder whose pipeline is blocked.
type stuckDecoder struct {
	in  chan *message.Message
	out chan *message.Message
}

func newStuckDecoder() *stuckDecoder {
	return &stuckDecoder{in: make(chan *message.Message), out: make(chan *message.Message)}
}

func (d *stuckDecoder) Start()                             {}
func (d *stuckDecoder) Stop()                              { close(d.out) }
func (d *stuckDecoder) GetLineCount() int64                { return 0 }
func (d *stuckDecoder) GetDetectedPattern() *regexp.Regexp { return nil }
func (d *stuckDecoder) InputChan() chan *message.Message   { return d.in }
func (d *stuckDecoder) OutputChan() chan *message.Message  { return d.out }

// TestNoHandleHeldWhileThePipelineIsBlocked checks that a tailer whose
// pipeline does not accept data never keeps the file open while it waits,
// and that a cancelled poll does not count the bytes the decoder did not
// accept.
func TestNoHandleHeldWhileThePipelineIsBlocked(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\ntwo\n"))
	tt := newTestTailer(t, share, id, 0, func(o *TailerOptions) { o.Decoder = newStuckDecoder() })

	ctx, cancel := context.WithCancel(context.Background())
	entry := entryOf(t, share, testPath)
	polled := make(chan error, 1)
	go func() {
		_, err := tt.Poll(ctx, entry)
		polled <- err
	}()
	// Once the read returned, Poll can only be waiting for the decoder.
	require.Eventually(t, func() bool {
		return share.Calls(fake.OpReadAt) == 1 && share.OpenHandles() == 0
	}, 5*time.Second, time.Millisecond)
	select {
	case err := <-polled:
		require.FailNow(t, "Poll returned while the pipeline is blocked", "%v", err)
	default:
	}

	cancel()
	require.ErrorIs(t, <-polled, context.Canceled)
	assert.Zero(t, tt.Offset(), "bytes the decoder did not accept are read again next time")
	assert.Zero(t, share.OpenHandles())
	assert.Equal(t, 1, share.Calls(fake.OpReadAt))
}

func TestRecordMissedBytes(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	share := fake.New()
	id := share.Write(testPath, []byte("one\n"))
	tt := newTestTailer(t, share, id, 0)

	_, err := tt.Poll(context.Background(), entryOf(t, share, testPath))
	require.NoError(t, err)
	assert.Zero(t, tt.RecordMissedBytes("test"), "nothing unread")

	// The listing saw 6 more bytes, which were never read.
	share.Append(testPath, []byte("lost!\n"))
	listed := entryOf(t, share, testPath)
	share.FailNext(fake.OpReadAt, fake.ErrSharing)
	_, err = tt.Poll(context.Background(), listed)
	require.Error(t, err)

	before := metrics.BytesMissed.Value()
	assert.EqualValues(t, 6, tt.UnreadBytes())
	assert.EqualValues(t, 6, tt.RecordMissedBytes("test"))
	assert.EqualValues(t, 6, metrics.BytesMissed.Value()-before)
	snapshot := metrics.MissedBytesSnapshot()
	require.Len(t, snapshot, 1)
	assert.Equal(t, "demo", snapshot[0].Source)
	assert.Equal(t, "demo-app", snapshot[0].Service)
	assert.EqualValues(t, 6, snapshot[0].Bytes)
}

// TestAssumeSize checks that a tailer counts as unread the bytes another
// tailer of the file saw past its offset, before it sees the file itself.
func TestAssumeSize(t *testing.T) {
	share := fake.New()
	id := share.Write(testPath, []byte("one\ntwo\n"))
	tt := newTestTailer(t, share, id, 4)
	assert.Zero(t, tt.UnreadBytes())
	tt.AssumeSize(8)
	assert.EqualValues(t, 4, tt.UnreadBytes())
	tt.AssumeSize(6) // a smaller size changes nothing
	assert.EqualValues(t, 4, tt.UnreadBytes())
}

func TestStopWithoutStart(t *testing.T) {
	configmock.New(t)
	source := sources.NewReplaceableSource(sources.NewLogSource("smb-test", &config.LogsConfig{Type: config.SMBType}))
	info := status.NewInfoRegistry()
	tl := NewTailer(&TailerOptions{
		Source:          source,
		Client:          fake.New().NewSession(),
		Host:            "h",
		Share:           "s",
		Path:            "a.log",
		OutputChan:      make(chan *message.Message),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
	})
	done := make(chan struct{})
	go func() {
		tl.Stop()
		tl.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Stop blocked")
	}
	assert.Equal(t, "smb", tl.GetType())
	assert.Equal(t, "smb://h/s/a.log", tl.GetID())
	assert.Contains(t, tl.GetInfo().Rendered(), "SMB File")
}

func TestOutcomeString(t *testing.T) {
	assert.Equal(t, "truncated", OutcomeTruncated.String())
	assert.Equal(t, "outcome(42)", Outcome(42).String())
}
