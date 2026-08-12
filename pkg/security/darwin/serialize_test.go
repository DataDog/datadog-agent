// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package darwin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// This file is //go:build darwin rather than unix because it exercises the
// darwin implementation of pkg/security/serializers; on Linux that symbol is the
// Linux serializer and the assertions would not mean the same thing. It
// therefore runs locally via dda inv test but not in Linux CI.

func testScrubber(t *testing.T) *utils.Scrubber {
	t.Helper()
	s, err := utils.NewScrubber(nil, nil)
	require.NoError(t, err)
	return s
}

// TestSerializeExecProducesLinuxShapedPayload guards the backend contract: the
// staging reducer has never seen a darwin event, so the payload must use the
// same top-level keys a Linux CWS event uses.
func TestSerializeExecProducesLinuxShapedPayload(t *testing.T) {
	tr := newTestTranslator(t)

	_, err := tr.Translate(execMessage(t, 900, 1, "/usr/local/bin/npm", []string{"npm", "install"}))
	require.NoError(t, err)
	_, err = tr.Translate(forkMessage(t, 901, 900, "/usr/local/bin/npm"))
	require.NoError(t, err)
	ev, err := tr.Translate(execMessage(t, 901, 900, "/bin/sh", []string{"sh", "-c", "echo hi"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	raw, err := serializers.MarshalEvent(ev, testScrubber(t))
	require.NoError(t, err)
	require.NotEqual(t, "null", string(raw), "the darwin serializer must not be the no-op stub")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	assert.Contains(t, payload, "process", "payload must carry process context")
	assert.Contains(t, payload, "evt", "payload must carry the event descriptor")
	assert.Contains(t, payload, "date", "payload must carry a timestamp")

	evt, ok := payload["evt"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "exec", evt["name"], "event name must be the SECL event type")

	proc, ok := payload["process"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 901, proc["pid"])
	assert.EqualValues(t, 900, proc["ppid"])

	exe, ok := proc["executable"].(map[string]any)
	require.True(t, ok, "process must carry an executable")
	assert.Equal(t, "/bin/sh", exe["path"])
	assert.Equal(t, "sh", exe["name"])

	// The ancestor chain is what makes a signal legible in the UI.
	ancestors, ok := proc["ancestors"].([]any)
	require.True(t, ok, "payload must carry the ancestor lineage")
	assert.NotEmpty(t, ancestors)

	t.Logf("exec payload: %s", raw)
}

// TestSerializeCarriesInterpreterForScripts checks that an interpreted entry
// point reports both files: the script as the executable (SECL's convention) and
// the interpreter alongside it. Without both, a reader cannot tell that "npm"
// was really run by /bin/sh.
func TestSerializeCarriesInterpreterForScripts(t *testing.T) {
	tr := newTestTranslator(t)

	ev, err := tr.Translate(execMessageScript(t, 940, 1,
		"/bin/sh", "/private/tmp/fake-b/npm", []string{"/bin/sh", "/private/tmp/fake-b/npm"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	raw, err := serializers.MarshalEvent(ev, testScrubber(t))
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	proc, ok := payload["process"].(map[string]any)
	require.True(t, ok)

	exe, ok := proc["executable"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "npm", exe["name"], "the executable must be the script")

	interp, ok := proc["interpreter"].(map[string]any)
	require.True(t, ok, "an interpreted script must report its interpreter")
	assert.Equal(t, "sh", interp["name"])
	assert.Equal(t, "/bin/sh", interp["path"])

	t.Logf("script payload: %s", raw)
}

// TestSerializeOmitsInterpreterForBinaries guards the other direction: a real
// binary must not grow an empty interpreter section.
func TestSerializeOmitsInterpreterForBinaries(t *testing.T) {
	tr := newTestTranslator(t)

	ev, err := tr.Translate(execMessage(t, 941, 1, "/private/tmp/fake-c/npm", []string{"npm"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	raw, err := serializers.MarshalEvent(ev, testScrubber(t))
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "interpreter",
		"a real binary has no interpreter and must not report one")
}

// TestSerializeNeverEmitsEnvs is the last line of the privacy defence: whatever
// happens upstream, the wire payload must not contain environment variables.
func TestSerializeNeverEmitsEnvs(t *testing.T) {
	tr := newTestTranslator(t)

	ev, err := tr.Translate(execMessage(t, 910, 1, "/bin/sh", []string{"sh", "-c", "echo hi"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	raw, err := serializers.MarshalEvent(ev, testScrubber(t))
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "envs", "no envs key may reach the wire")
	assert.NotContains(t, string(raw), "envp", "no envp key may reach the wire")
}

// TestSerializeScrubsArgs proves the scrubber is actually applied to argv on the
// way out, not merely available.
//
// Note the limits of what this guarantees. The scrubber is keyword-based: it
// redacts a value when the adjacent key matches one of procutil's sensitive
// words (*password*, *access_token*, *api_key*, *secret*, *credentials*, ...)
// plus the *token* / *jwt* that pkg/security/utils adds. It does NOT recognise
// arbitrary opaque secrets. In particular
//
//	curl -H "Authorization: Bearer <token>"
//
// is not redacted, because no word in it matches. That pattern is common on a
// developer laptop, so the privacy story for a shipped product needs more than
// the default scrubber -- see the PoC write-up.
//
// There is a second, positional limitation: the scrubber's key group is
// ( +| -{1,2}), i.e. the key must be preceded by whitespace. argv is joined
// before matching, so the FIRST argument after argv0 sits at offset 0 and can
// never match. This is pre-existing behaviour in pkg/process/procutil shared with
// Linux CWS, not something specific to darwin. The argument order below is
// therefore deliberate.
func TestSerializeScrubsArgs(t *testing.T) {
	tr := newTestTranslator(t)

	ev, err := tr.Translate(execMessage(t, 920, 1, "/usr/bin/deploy",
		[]string{"deploy", "--endpoint", "https://example.com", "--api_key=abcdef123456", "--password", "hunter2"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	raw, err := serializers.MarshalEvent(ev, testScrubber(t))
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "abcdef123456",
		"an --api_key= value in argv must be scrubbed before serialization")
	assert.NotContains(t, string(raw), "hunter2",
		"a --password value in argv must be scrubbed before serialization")
	// The non-secret parts must survive, otherwise the event is useless.
	assert.Contains(t, string(raw), "https://example.com")

	t.Logf("scrubbed payload: %s", raw)
}

// TestScrubberMissesLeadingAndOpaqueSecrets documents, with executable evidence,
// the two gaps described above. It is here so the limitation is discoverable
// rather than assumed away: anyone reasoning about what argv is safe to ship from
// a developer laptop needs to know these cases are NOT redacted.
//
// If a future scrubber change starts catching them, this test fails and should be
// deleted -- that would be good news.
func TestScrubberMissesLeadingAndOpaqueSecrets(t *testing.T) {
	tr := newTestTranslator(t)

	ev, err := tr.Translate(execMessage(t, 921, 1, "/usr/bin/curl",
		[]string{"curl", "--api_key=leading123", "-H", "Authorization: Bearer opaque456"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	raw, err := serializers.MarshalEvent(ev, testScrubber(t))
	require.NoError(t, err)

	assert.Contains(t, string(raw), "leading123",
		"KNOWN GAP: the first argument after argv0 is never scrubbed (regex needs leading whitespace)")
	assert.Contains(t, string(raw), "opaque456",
		"KNOWN GAP: keyword-based scrubbing does not recognise an Authorization: Bearer header")
}

// TestSerializeExitCarriesCauseAndCode checks the exit section, where the raw
// wait(2) status word would otherwise leak into the UI as a nonsense code.
func TestSerializeExitCarriesCauseAndCode(t *testing.T) {
	tr := newTestTranslator(t)

	_, err := tr.Translate(execMessage(t, 930, 1, "/usr/libexec/xpcproxy", []string{"xpcproxy"}))
	require.NoError(t, err)

	// 19968 is the real status word captured from eslogger: exit code 78.
	ev, err := tr.Translate(exitMessage(t, 930, 19968))
	require.NoError(t, err)
	require.NotNil(t, ev)

	raw, err := serializers.MarshalEvent(ev, testScrubber(t))
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	exit, ok := payload["exit"].(map[string]any)
	require.True(t, ok, "an exit event must carry an exit section")
	assert.EqualValues(t, 78, exit["code"], "code must be WEXITSTATUS, not the raw status word")
	assert.Equal(t, "EXITED", exit["cause"])

	t.Logf("exit payload: %s", raw)
}
