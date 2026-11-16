// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cli

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

var errNoMatch = errors.New("no match in buffered output")

// fakeCLI is a synchronous imitation of a device CLI over a PTY: it echoes
// each line followed by \r\n, writes the line's output, and then writes the
// prompt. ReadUntil never waits: with no match it returns errNoMatch, as a
// real shell would once ctx ended.
type fakeCLI struct {
	prompt  string
	outputs map[string]string // line -> output; unknown lines are rejected
	noEcho  bool
	stall   string // a line whose output never ends with a prompt
	ask     string // a line that is answered with a question instead of a prompt

	buf         []byte
	sent        []string
	closedWrite bool
}

func newFakeCLI(banner, prompt string, outputs map[string]string) *fakeCLI {
	f := &fakeCLI{prompt: prompt, outputs: outputs}
	f.write(banner + prompt)
	return f
}

func (f *fakeCLI) write(s string) {
	f.buf = append(f.buf, strings.ReplaceAll(s, "\n", "\r\n")...)
}

func (f *fakeCLI) Send(_ context.Context, data string) error {
	line := strings.TrimSuffix(data, "\n")
	f.sent = append(f.sent, line)
	if !f.noEcho {
		f.write(line + "\n")
	}
	switch {
	case f.stall != "" && line == f.stall:
		f.write("partial output\n")
		return nil
	case f.ask != "" && line == f.ask:
		f.write("Destination filename [startup-config]? ")
		return nil
	case line == "":
	default:
		out, ok := f.outputs[line]
		if !ok {
			out = "ERROR: % Invalid input detected at '^' marker.\n"
		}
		f.write(out)
	}
	f.write(f.prompt)
	return nil
}

func (f *fakeCLI) ReadUntil(_ context.Context, match func([]byte) int) ([]byte, error) {
	i := match(f.buf)
	if i < 0 {
		out := f.buf
		f.buf = nil
		return out, errNoMatch
	}
	out := f.buf[:i]
	f.buf = f.buf[i:]
	return out, nil
}

func (f *fakeCLI) CloseWrite() error {
	f.closedWrite = true
	return nil
}

var testDialect = &Dialect{
	Name:   "test",
	Prompt: regexp.MustCompile(`^dev[#>] ?$`),
	Setup:  []string{"no paging"},
	Error:  regexp.MustCompile(`(?m)^ERROR: .*$`),
	Exit:   "logout",
}

func TestOpen_DiscardsBannerAndRunsSetup(t *testing.T) {
	f := newFakeCLI("Welcome to dev\nAuthorized use only\n", "dev# ", map[string]string{"no paging": ""})
	s, err := Open(context.Background(), f, testDialect)
	require.NoError(t, err)
	assert.Equal(t, []string{"no paging"}, f.sent)
	assert.Equal(t, "dev# ", s.Prompt())
	assert.Empty(t, f.buf)
}

func TestOpen_RejectedSetup(t *testing.T) {
	f := newFakeCLI("", "dev# ", map[string]string{})
	_, err := Open(context.Background(), f, testDialect)
	assert.EqualError(t, err, `running "no paging": ERROR: % Invalid input detected at '^' marker.`)
}

func TestOpen_NoPrompt(t *testing.T) {
	f := newFakeCLI("Welcome\n", "login: ", nil)
	_, err := Open(context.Background(), f, testDialect)
	assert.ErrorIs(t, err, errNoMatch)
	assert.ErrorContains(t, err, "waiting for test prompt")
}

func TestRun_StripsEchoAndPrompt(t *testing.T) {
	f := newFakeCLI("", "dev# ", map[string]string{
		"no paging":    "",
		"show version": "DevOS 1.2\nUptime 1 day\n",
	})
	s, err := Open(context.Background(), f, testDialect)
	require.NoError(t, err)

	reply, err := s.Run(context.Background(), "show version")
	require.NoError(t, err)
	assert.Equal(t, Reply{Output: "DevOS 1.2\nUptime 1 day\n", Prompt: "dev# "}, reply)
}

func TestRun_NoEcho(t *testing.T) {
	f := newFakeCLI("", "dev# ", map[string]string{"show version": "DevOS 1.2\n"})
	f.noEcho = true
	s, err := Open(context.Background(), f, &Dialect{Name: "test", Prompt: testDialect.Prompt})
	require.NoError(t, err)

	reply, err := s.Run(context.Background(), "show version")
	require.NoError(t, err)
	assert.Equal(t, "DevOS 1.2\n", reply.Output)
}

func TestStripEcho(t *testing.T) {
	for _, tc := range []struct {
		name, text, line, want string
	}{
		{"echo", "show version\nDevOS\n", "show version", "DevOS\n"},
		{"echo_trailing_space", "show version  \nDevOS\n", "show version", "DevOS\n"},
		{"redrawn_prompt", "dev# show version\nDevOS\n", "show version", "DevOS\n"},
		{"no_echo", "DevOS\nUptime\n", "show version", "DevOS\nUptime\n"},
		{"no_newline", "show version", "show version", "show version"},
		{"empty_line_echo", "\n[OK]\n", "", "[OK]\n"},
		{"empty_line_no_echo", "[OK]\n", "", "[OK]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stripEcho(tc.text, tc.line))
		})
	}
}

func TestRun_EchoAfterRedrawnPrompt(t *testing.T) {
	sh := &scriptedShell{chunks: []string{"dev# ", "dev# show version\r\nDevOS 1.2\r\ndev# "}}
	s, err := Open(context.Background(), sh, &Dialect{Name: "test", Prompt: testDialect.Prompt})
	require.NoError(t, err)

	reply, err := s.Run(context.Background(), "show version")
	require.NoError(t, err)
	assert.Equal(t, "DevOS 1.2\n", reply.Output)
}

func TestRun_StopsAtQuestion(t *testing.T) {
	f := newFakeCLI("", "dev# ", map[string]string{"no paging": ""})
	f.ask = "copy running-config startup-config"
	s, err := Open(context.Background(), f, testDialect)
	require.NoError(t, err)

	question := regexp.MustCompile(`(\?|\[confirm\]) ?$`)
	reply, err := s.Run(context.Background(), f.ask, question)
	require.NoError(t, err)
	assert.Equal(t, Reply{Output: "Destination filename [startup-config]? "}, reply)
	assert.Equal(t, "dev# ", s.Prompt(), "the last prompt is kept")

	reply, err = s.Run(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, Reply{Output: "", Prompt: "dev# "}, reply)
}

func TestRun_StallReturnsPartialOutput(t *testing.T) {
	f := newFakeCLI("", "dev# ", map[string]string{"no paging": ""})
	f.stall = "show tech"
	s, err := Open(context.Background(), f, testDialect)
	require.NoError(t, err)

	reply, err := s.Run(context.Background(), "show tech")
	assert.ErrorIs(t, err, errNoMatch)
	assert.Equal(t, Reply{Output: "partial output\n"}, reply)
}

func TestRun_UnrecognizedPrompt(t *testing.T) {
	sh := newStreamShell()
	sh.push("dev# ")
	s, err := Open(context.Background(), sh, &Dialect{Name: "test", Prompt: testDialect.Prompt})
	require.NoError(t, err)

	// The device answers with a prompt the dialect does not recognize and
	// then goes quiet, so the read lasts until ctx ends.
	sh.onSend = func(line string) { sh.push(line + "\r\nDevOS 1.2\r\nweird-prompt$ ") }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	reply, err := s.Run(ctx, "show version")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, Reply{Output: "DevOS 1.2\nweird-prompt$ "}, reply)
}

func TestRun_ContextEnds(t *testing.T) {
	sh := newStreamShell()
	sh.push("dev# ")
	s, err := Open(context.Background(), sh, &Dialect{Name: "test", Prompt: testDialect.Prompt})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = s.Run(ctx, "show version")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestEnd_SendsExitAndClosesInput(t *testing.T) {
	f := newFakeCLI("", "dev# ", map[string]string{"no paging": "", "logout": ""})
	s, err := Open(context.Background(), f, testDialect)
	require.NoError(t, err)

	s.End(context.Background(), time.Millisecond)
	assert.Equal(t, []string{"no paging", "logout"}, f.sent)
	assert.True(t, f.closedWrite)
}

func TestCheckError(t *testing.T) {
	assert.NoError(t, (&Dialect{}).CheckError("ERROR: anything"))
	assert.NoError(t, testDialect.CheckError("all good\n"))
	assert.EqualError(t, testDialect.CheckError("x\nERROR: bad thing\ny\n"), "ERROR: bad thing")
}

func TestForProfile(t *testing.T) {
	d, err := ForProfile(profile.ProfileCiscoASA)
	require.NoError(t, err)
	assert.Same(t, ASA, d)

	d, err = ForProfile(profile.ProfileCiscoIOS)
	require.NoError(t, err)
	assert.Same(t, IOS, d)

	_, err = ForProfile(profile.ProfileEOS)
	assert.EqualError(t, err, `no interactive shell defined for profile "eos"`)
}

// scriptedShell returns one chunk of output per read, regardless of what is
// sent.
type scriptedShell struct {
	chunks []string
}

func (s *scriptedShell) Send(context.Context, string) error { return nil }

func (s *scriptedShell) ReadUntil(_ context.Context, match func([]byte) int) ([]byte, error) {
	if len(s.chunks) == 0 {
		return nil, errNoMatch
	}
	out := []byte(s.chunks[0])
	s.chunks = s.chunks[1:]
	if match(out) < 0 {
		return out, errNoMatch
	}
	return out, nil
}

// streamShell is an asynchronous shell: ReadUntil blocks for more output
// until ctx ends, like remote's sshShell.
type streamShell struct {
	mu     sync.Mutex
	buf    []byte
	notify chan struct{}
	onSend func(line string)
}

func newStreamShell() *streamShell {
	return &streamShell{notify: make(chan struct{}, 1)}
}

func (s *streamShell) push(out string) {
	s.mu.Lock()
	s.buf = append(s.buf, out...)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *streamShell) buffered() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf
}

func (s *streamShell) Send(_ context.Context, data string) error {
	if s.onSend != nil {
		s.onSend(strings.TrimSuffix(data, "\n"))
	}
	return nil
}

func (s *streamShell) ReadUntil(ctx context.Context, match func([]byte) int) ([]byte, error) {
	for {
		s.mu.Lock()
		if i := match(s.buf); i >= 0 {
			out := s.buf[:i:i]
			s.buf = s.buf[i:]
			s.mu.Unlock()
			return out, nil
		}
		pending := append([]byte(nil), s.buf...)
		s.mu.Unlock()
		select {
		case <-s.notify:
		case <-ctx.Done():
			return pending, ctx.Err()
		}
	}
}
