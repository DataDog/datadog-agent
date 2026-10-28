// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/auth"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
)

// stubClient is the session a dial that succeeds returns.
type stubClient struct{}

func (stubClient) ListDir(context.Context, string) ([]Entry, error) { return nil, nil }
func (stubClient) ReadAt(context.Context, string, int64, int) (ReadResult, error) {
	return ReadResult{}, nil
}
func (stubClient) Close() error { return nil }

// logonServer answers the dials of the tests: each dial is a SESSION_SETUP,
// counted, and fails with what reply returns for the password it sent.
type logonServer struct {
	mu    sync.Mutex
	dials int
	gate  chan struct{} // when set, a dial waits for it to close before it answers
	reply func(cfg Config) error
}

func (s *logonServer) dial(ctx context.Context, cfg Config) (Client, error) {
	s.mu.Lock()
	s.dials++
	gate, reply := s.gate, s.reply
	s.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := reply(cfg); err != nil {
		return nil, err
	}
	return stubClient{}, nil
}

func (s *logonServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dials
}

func refuse(code uint32) func(Config) error {
	return func(Config) error { return fmt.Errorf("smb: connect to smb://h/s: %w", status(code)) }
}

func accountConfig(host, share, password string) Config {
	return Config{Host: host, Share: share, Username: "svc-logs", Password: password, Domain: "CORP"}
}

// reader returns a client of cfg that dials through the guard, and calls it.
func reader(t *testing.T, g *Guard, clk clock.Clock, srv *logonServer, cfg Config) func() error {
	t.Helper()
	c := NewReconnecting(cfg, WithDialer(srv.dial), WithClock(clk), WithGuard(g))
	t.Cleanup(func() { _ = c.Close() })
	return func() error {
		_, err := c.ListDir(context.Background(), "")
		return err
	}
}

// TestOneRefusedLogonStopsEverySourceOfTheAccount: the server refuses the
// password once, and the other sources, on the same host and on others, send no
// logon, however many there are and however they race.
func TestOneRefusedLogonStopsEverySourceOfTheAccount(t *testing.T) {
	for _, code := range []uint32{statusLogonFailure, statusWrongPassword, statusNoSuchUser} {
		t.Run(fmt.Sprintf("%#x", code), func(t *testing.T) {
			clk := clock.NewMock()
			g := NewGuard(clk)
			srv := &logonServer{gate: make(chan struct{}), reply: refuse(code)}
			var calls []func() error
			for _, host := range []string{"dc1.corp.example.com", "dc2.corp.example.com", "dc3.corp.example.com"} {
				for _, share := range []string{"a", "b", "c", "d"} {
					calls = append(calls, reader(t, g, clk, srv, accountConfig(host, share, "wrong-Passw0rd")))
				}
			}

			errs := make(chan error, len(calls))
			for _, call := range calls {
				go func() { errs <- call() }()
			}
			// Every dial is in flight or waiting for the first one's outcome.
			require.Eventually(t, func() bool { return srv.count() == 1 }, time.Second, time.Millisecond)
			time.Sleep(20 * time.Millisecond)
			assert.Equal(t, 1, srv.count(), "one logon in flight for the account: the others wait for its outcome")
			close(srv.gate)
			for range calls {
				err := <-errs
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrLogonStopped)
				assert.Equal(t, ErrAuth, Classify(err))
			}
			assert.Equal(t, 1, srv.count(), "exactly one SESSION_SETUP for %d sources on 3 hosts", len(calls))

			// Later attempts, minutes and hours after, send nothing either.
			for range 3 {
				clk.Add(31 * time.Second)
				for _, call := range calls {
					assert.ErrorIs(t, call(), ErrLogonStopped)
				}
			}
			clk.Add(48 * time.Hour)
			for _, call := range calls {
				assert.ErrorIs(t, call(), ErrLogonStopped)
			}
			assert.Equal(t, 1, srv.count())
		})
	}
}

// TestStoppedUntilThePasswordChanges: a new password, which a secret refresh or a
// config change brings, gives a new fingerprint and dials again, once; the old
// password stays refused for the life of the process.
func TestStoppedUntilThePasswordChanges(t *testing.T) {
	clk := clock.NewMock()
	g := NewGuard(clk)
	srv := &logonServer{reply: func(cfg Config) error {
		if cfg.Password == "good-Passw0rd" {
			return nil
		}
		return refuse(statusLogonFailure)(cfg)
	}}
	old := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "old-Passw0rd"))
	assert.ErrorIs(t, old(), ErrLogonStopped)
	assert.Equal(t, 1, srv.count())

	wrong := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "typo-Passw0rd"))
	assert.ErrorIs(t, wrong(), ErrLogonStopped, "the new password is refused too")
	assert.Equal(t, 2, srv.count(), "one logon for the new password")
	again := reader(t, g, clk, srv, accountConfig("dc2.corp.example.com", "b", "typo-Passw0rd"))
	assert.ErrorIs(t, again(), ErrLogonStopped)
	assert.Equal(t, 2, srv.count(), "and no more")

	fixed := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "good-Passw0rd"))
	require.NoError(t, fixed())
	assert.Equal(t, 3, srv.count())
	clk.Add(time.Hour)
	assert.ErrorIs(t, old(), ErrLogonStopped, "the old password is not tried again")
	assert.Equal(t, 3, srv.count())
}

// TestAccountStatesStop: an account the server says is disabled, expired or
// needs a new password is not asked again either, until the password changes.
func TestAccountStatesStop(t *testing.T) {
	for name, code := range map[string]uint32{
		"disabled":             statusAccountDisabled,
		"expired":              statusAccountExpired,
		"password expired":     statusPasswordExpired,
		"password must change": statusPasswordMustChange,
	} {
		t.Run(name, func(t *testing.T) {
			clk := clock.NewMock()
			g := NewGuard(clk)
			srv := &logonServer{reply: refuse(code)}
			first := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "Passw0rd-1"))
			second := reader(t, g, clk, srv, accountConfig("dc2.corp.example.com", "b", "Passw0rd-1"))
			assert.ErrorIs(t, first(), ErrLogonStopped)
			clk.Add(48 * time.Hour)
			assert.ErrorIs(t, second(), ErrLogonStopped)
			assert.ErrorIs(t, first(), ErrLogonStopped)
			assert.Equal(t, 1, srv.count())
			why, fix, probes, ok := LogonStop(second())
			require.True(t, ok)
			assert.NotEmpty(t, why)
			assert.NotEmpty(t, fix)
			assert.False(t, probes, "nothing but a new password ends it")
		})
	}
}

// TestLockedOutAccountIsProbedHourly: a locked-out account is asked once an hour,
// give or take 10%, by one source, and the other sources wait for its answer.
func TestLockedOutAccountIsProbedHourly(t *testing.T) {
	clk := clock.NewMock()
	g := NewGuard(clk)
	g.jitter = func() float64 { return 0 }
	srv := &logonServer{reply: refuse(statusAccountLockedOut)}
	first := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "Passw0rd-1"))
	second := reader(t, g, clk, srv, accountConfig("dc2.corp.example.com", "b", "Passw0rd-1"))

	assert.ErrorIs(t, first(), ErrLogonStopped)
	assert.ErrorIs(t, second(), ErrLogonStopped)
	assert.Equal(t, 1, srv.count())
	_, _, probes, ok := LogonStop(second())
	require.True(t, ok)
	assert.True(t, probes, "the status says the Agent tries again")

	clk.Add(time.Hour - time.Minute)
	assert.ErrorIs(t, first(), ErrLogonStopped)
	assert.Equal(t, 1, srv.count(), "not before the hour")
	clk.Add(time.Minute)
	assert.ErrorIs(t, first(), ErrLogonStopped)
	assert.Equal(t, 2, srv.count(), "one probe at the hour")
	assert.ErrorIs(t, second(), ErrLogonStopped)
	assert.Equal(t, 2, srv.count(), "the other source uses the probe's verdict")

	clk.Add(time.Hour)
	assert.ErrorIs(t, second(), ErrLogonStopped)
	assert.Equal(t, 3, srv.count(), "and the next hour")

	// The lock expires: the next probe logs on, and the stop ends for everyone.
	srv.mu.Lock()
	srv.reply = func(Config) error { return nil }
	srv.mu.Unlock()
	clk.Add(time.Hour)
	require.NoError(t, first())
	require.NoError(t, second())
	assert.Equal(t, 5, srv.count(), "each source logs on with its own session")
}

// TestLockoutProbeJitter: the probe is an hour plus or minus 10%.
func TestLockoutProbeJitter(t *testing.T) {
	g := NewGuard(clock.NewMock())
	g.jitter = func() float64 { return -1 }
	assert.Equal(t, 54*time.Minute, g.jittered(lockoutProbe))
	g.jitter = func() float64 { return 1 }
	assert.Equal(t, 66*time.Minute, g.jittered(lockoutProbe))

	g = NewGuard(clock.NewMock())
	seen := make(map[time.Duration]bool)
	for range 1000 {
		d := g.jittered(lockoutProbe)
		require.GreaterOrEqual(t, d, 54*time.Minute)
		require.LessOrEqual(t, d, 66*time.Minute)
		seen[d] = true
	}
	assert.Greater(t, len(seen), 100, "the delay varies, so that the Agents of a fleet do not probe together")
}

// guestError is what a dial returns when the server grants only a guest session.
func guestError(cfg Config) error {
	return fmt.Errorf("smb: connect to smb://h/%s: %w: %w", cfg.Share, errGuestSession, errors.New("guest account doesn't support signing"))
}

// TestGuestSessionStopsTheAccountLikeARefusal: Samba maps a wrong password to
// guest ("map to guest = Bad Password"), but the domain controller counted the
// bad password all the same. So a guest session stops the logons of the account
// and password, on every source and server, until the password changes, and the
// status says what to fix.
func TestGuestSessionStopsTheAccountLikeARefusal(t *testing.T) {
	clk := clock.NewMock()
	g := NewGuard(clk)
	srv := &logonServer{reply: func(cfg Config) error {
		if cfg.Password == "Good-Passw0rd" {
			return nil
		}
		return guestError(cfg)
	}}
	guest := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "Passw0rd-1"))
	other := reader(t, g, clk, srv, accountConfig("dc2.corp.example.com", "b", "Passw0rd-1"))

	err := guest()
	require.ErrorIs(t, err, ErrLogonStopped)
	assert.ErrorIs(t, err, errGuestSession)
	assert.Equal(t, ErrAuth, Classify(err))
	why, fix, _, ok := LogonStop(err)
	require.True(t, ok)
	assert.Contains(t, why, "guest")
	assert.Contains(t, fix, "account mapping")
	assert.Contains(t, fix, "password")

	assert.ErrorIs(t, other(), ErrLogonStopped)
	for range 3 {
		clk.Add(31 * time.Second)
		assert.ErrorIs(t, guest(), ErrLogonStopped)
		assert.ErrorIs(t, other(), ErrLogonStopped)
	}
	clk.Add(48 * time.Hour)
	assert.ErrorIs(t, other(), ErrLogonStopped)
	assert.Equal(t, 1, srv.count(), "one logon of the account, however many sources: the guest session counted as a bad password")
	_, _, _, ok = LogonStop(other())
	require.True(t, ok, "the sources that did not dial say it too")

	fixed := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "Good-Passw0rd"))
	require.NoError(t, fixed())
	assert.Equal(t, 2, srv.count(), "a new password is tried")
}

// TestOtherFailuresNeverStop: whatever is not a refused logon or an account state
// keeps the existing backoff and never stops anything: network errors, a domain
// controller that cannot be reached, a failed trust, a refused tree connect, an
// unknown share, a server that does not encrypt.
func TestOtherFailuresNeverStop(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		kind ErrorKind
	}{
		"network error":          {&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, ErrTransient},
		"EOF":                    {io.ErrUnexpectedEOF, ErrTransient},
		"no logon servers":       {status(0xC000005E), ErrOther},
		"trust failure":          {status(0xC000018D), ErrOther},
		"trusted domain":         {status(0xC000018C), ErrOther},
		"access denied":          {status(statusAccessDenied), ErrAuth},
		"account restriction":    {status(statusAccountRestriction), ErrAuth},
		"unknown share":          {status(statusBadNetworkName), ErrAuth},
		"not encrypted":          {ErrNotEncrypted, ErrAuth},
		"tree connect refusal":   {notALogon(status(statusLogonFailure)), ErrAuth},
		"tree connect, locked":   {notALogon(status(statusAccountLockedOut)), ErrAuth},
		"mount access denied":    {notALogon(status(statusAccessDenied)), ErrAuth},
		"logon type not granted": {status(statusLogonTypeNotGranted), ErrAuth},
	} {
		t.Run(name, func(t *testing.T) {
			clk := clock.NewMock()
			g := NewGuard(clk)
			srv := &logonServer{reply: func(Config) error { return fmt.Errorf("smb: connect to smb://h/s: %w", tc.err) }}
			first := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "Passw0rd-1"))
			other := reader(t, g, clk, srv, accountConfig("dc2.corp.example.com", "b", "Passw0rd-1"))

			err := first()
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrLogonStopped)
			assert.Equal(t, tc.kind, Classify(err))
			assert.Error(t, other())
			assert.Equal(t, 2, srv.count(), "a failure of one source does not keep the other from dialing")
			for i := range 4 {
				clk.Add(31 * time.Second) // past the longest backoff of a client
				assert.Error(t, first())
				assert.NotErrorIs(t, first(), ErrLogonStopped)
				assert.Equal(t, 3+2*i, srv.count(), "the client dials again after its backoff, as before")
				assert.Error(t, other())
			}
		})
	}
}

// TestAccountsAreKeyedByDomainAndUser: the key is the lower-cased domain and
// user, or the user alone when there is no domain: never the host, so one
// account is one key whatever the number of servers it is used on, and however
// each of them is spelled. A DOMAIN\ prefix or an @suffix of the user name is
// not part of the user.
func TestAccountsAreKeyedByDomainAndUser(t *testing.T) {
	clk := clock.NewMock()
	g := NewGuard(clk)
	srv := &logonServer{reply: refuse(statusLogonFailure)}
	call := func(cfg Config) error { return reader(t, g, clk, srv, cfg)() }

	assert.ErrorIs(t, call(Config{Host: "dc1", Share: "a", Username: "Svc-Logs", Password: "Passw0rd-1", Domain: "CORP"}), ErrLogonStopped)
	assert.Equal(t, 1, srv.count())

	for _, same := range []Config{
		{Host: "dc9", Share: "a", Username: "svc-logs", Password: "Passw0rd-1", Domain: "corp"},
		{Host: "dc9", Share: "z", Username: "SVC-LOGS", Password: "Passw0rd-1", Domain: "Corp"},
		{Host: "dc9", Share: "z", Username: `CORP\svc-logs`, Password: "Passw0rd-1", Domain: "CORP"},
		{Host: "dc9", Share: "z", Username: "svc-logs@corp.example.com", Password: "Passw0rd-1", Domain: "CORP"},
	} {
		assert.ErrorIs(t, call(same), ErrLogonStopped, "%+v", same)
	}
	assert.Equal(t, 1, srv.count(), "the case of the domain and the user does not matter, nor a prefix or a suffix")

	for _, other := range []Config{
		{Host: "dc1", Share: "a", Username: "other-user", Password: "Passw0rd-1", Domain: "CORP"},
		{Host: "dc1", Share: "a", Username: "svc-logs", Password: "Passw0rd-1", Domain: "OTHER"},
		{Host: "dc1", Share: "a", Username: "svc-logs", Password: "Passw0rd-1"},
	} {
		before := srv.count()
		assert.ErrorIs(t, call(other), ErrLogonStopped)
		assert.Equal(t, before+1, srv.count(), "%+v is another account: one logon", other)
	}

	// With no domain the user alone is the key, not the host: the server's
	// challenge chooses the domain, and on a domain-joined server it is the
	// same account whatever the server is called.
	noDomain := func(host, user string) Config {
		return Config{Host: host, Share: "a", Username: user, Password: "Passw0rd-2"}
	}
	before := srv.count()
	for _, cfg := range []Config{
		noDomain("fs1.example.com", "svc-logs"),
		noDomain("FS1.example.com", "SVC-LOGS"),
		noDomain("fs2.example.com", "svc-logs"),
		noDomain("10.0.0.7", `CORP\svc-logs`),
		noDomain("fs3", "svc-logs@corp.example.com"),
	} {
		assert.ErrorIs(t, call(cfg), ErrLogonStopped, "%+v", cfg)
	}
	assert.Equal(t, before+1, srv.count(), "five spellings of two servers, one logon")
}

// TestSuccessfulLogonsGoOn: while a logon is in flight, the
// dials of the account wait for its outcome; once it succeeded, they go on.
func TestSuccessfulLogonsGoOn(t *testing.T) {
	clk := clock.NewMock()
	g := NewGuard(clk)
	srv := &logonServer{gate: make(chan struct{}), reply: func(Config) error { return nil }}
	var calls []func() error
	for _, share := range []string{"a", "b", "c", "d", "e"} {
		calls = append(calls, reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", share, "Passw0rd-1")))
	}
	errs := make(chan error, len(calls))
	for _, call := range calls {
		go func() { errs <- call() }()
	}
	require.Eventually(t, func() bool { return srv.count() == 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 1, srv.count(), "one logon in flight")
	close(srv.gate)
	for range calls {
		require.NoError(t, <-errs)
	}
	assert.Equal(t, len(calls), srv.count(), "each source logs on once the first logon succeeded")
}

// TestWaitingDialStopsWithItsContext: a dial that waits for another logon of the
// account gives up when its context ends.
func TestWaitingDialStopsWithItsContext(t *testing.T) {
	clk := clock.NewMock()
	g := NewGuard(clk)
	srv := &logonServer{gate: make(chan struct{}), reply: func(Config) error { return nil }}
	t.Cleanup(func() { close(srv.gate) })
	first := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "Passw0rd-1"))
	go func() { _ = first() }()
	require.Eventually(t, func() bool { return srv.count() == 1 }, time.Second, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	c := NewReconnecting(accountConfig("dc2.corp.example.com", "b", "Passw0rd-1"), WithDialer(srv.dial), WithClock(clk), WithGuard(g))
	defer c.Close()
	_, err := c.ListDir(ctx, "")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, srv.count())
}

// TestNoCredentialsInStoppedErrors: neither the password nor its fingerprint is in
// an error or a status text, whatever the library put in its own.
func TestNoCredentialsInStoppedErrors(t *testing.T) {
	const secret = "Sup3r-S3cret-K3y=="
	clk := clock.NewMock()
	g := NewGuard(clk)
	srv := &logonServer{reply: func(cfg Config) error {
		// The Dial of the client redacts what the library says; the guard must
		// not put the password back.
		leaky := fmt.Errorf("session setup for %s with %s: %w", cfg.Username, cfg.Password, status(statusLogonFailure))
		return checked(fmt.Errorf("smb: connect to smb://h/s: %w", redactErr(leaky, cfg.Password)))
	}}
	call := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", secret))
	other := reader(t, g, clk, srv, accountConfig("dc2.corp.example.com", "b", secret))
	for _, err := range []error{call(), call(), other()} {
		require.ErrorIs(t, err, ErrLogonStopped)
		assert.NotContains(t, err.Error(), secret)
		assert.NotContains(t, fmt.Sprintf("%+v %#v %v", err, err, errors.Unwrap(err)), secret)
		why, _, _, ok := LogonStop(err)
		require.True(t, ok)
		assert.NotContains(t, why, secret)
		assert.NotContains(t, why, "dc1", "the reason names no server or share: the status names the source's own")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	assert.NotContains(t, fmt.Sprintf("%+v %#v", g.accounts, g.accounts), secret, "the guard keeps a salted hash, never the password")
}

// timedOut is the error of a dial whose AUTHENTICATE was sent and whose answer
// never came.
func timedOut(Config) error {
	return fmt.Errorf("smb: connect to smb://h/s: %w", unanswered(context.DeadlineExceeded))
}

// TestUnansweredLogonWaitsFifteenMinutes: a logon that was sent and never
// answered may have been counted as a bad password by a domain controller that
// answers slowly, so the account and password wait 15 minutes (give or take 10%)
// before the next logon, whatever the number of sources and servers. It is no
// stop: the wait ends by itself, and a new password goes at once.
func TestUnansweredLogonWaitsFifteenMinutes(t *testing.T) {
	clk := clock.NewMock()
	g := NewGuard(clk)
	g.jitter = func() float64 { return 0 }
	srv := &logonServer{reply: timedOut}
	first := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "Passw0rd-1"))
	other := reader(t, g, clk, srv, accountConfig("dc2.corp.example.com", "b", "Passw0rd-1"))

	err := first()
	require.Error(t, err)
	assert.Equal(t, ErrTransient, Classify(err))
	assert.Equal(t, 1, srv.count())

	for range 13 {
		clk.Add(time.Minute)
		for _, call := range []func() error{first, other} {
			err := call()
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrLogonStopped, "a wait is no stop")
			assert.Equal(t, ErrTransient, Classify(err))
			assert.Contains(t, err.Error(), "15 minutes")
		}
	}
	assert.Equal(t, 1, srv.count(), "no logon within the 15 minutes, on any source or server")

	fresh := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "Passw0rd-2"))
	assert.Error(t, fresh())
	assert.Equal(t, 2, srv.count(), "another password is not waiting")

	clk.Add(2*time.Minute + 31*time.Second) // past the 15 minutes, and the 30 seconds of the client's backoff
	assert.Error(t, first())
	assert.Equal(t, 3, srv.count(), "one logon after the wait")
	assert.Error(t, other())
	assert.Equal(t, 3, srv.count(), "and the others wait again")

	srv.mu.Lock()
	srv.reply = func(Config) error { return nil }
	srv.mu.Unlock()
	clk.Add(16 * time.Minute)
	require.NoError(t, first(), "the wait ends by itself")
	require.NoError(t, other(), "an answered logon ends it for everyone")
}

// TestCancelledDialRecordsNoWait: a dial whose context was cancelled (the source
// was replaced or removed, the logs agent restarts) says nothing about the
// server, so it starts no 15 minute wait for the account. A dial that timed out
// does.
func TestCancelledDialRecordsNoWait(t *testing.T) {
	cfg := accountConfig("dc1.corp.example.com", "a", "Passw0rd-1")
	other := accountConfig("dc2.corp.example.com", "b", "Passw0rd-1")

	for name, cancelled := range map[string]bool{"cancelled": true, "timed out": false} {
		t.Run(name, func(t *testing.T) {
			g := NewGuard(clock.NewMock())
			var dials atomic.Int32
			dial := func(ctx context.Context, _ Config) (Client, error) {
				if dials.Add(1) > 1 {
					return stubClient{}, nil
				}
				<-ctx.Done() // the AUTHENTICATE went out, and no answer comes
				return nil, fmt.Errorf("smb: connect to smb://h/s: %w", unanswered(ctx.Err()))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if cancelled {
				cancel()
			}
			_, err := g.Dial(ctx, cfg, dial)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "15 minutes", "the error of the dial itself, untouched")

			_, err = g.Dial(context.Background(), other, dial)
			if !cancelled {
				require.Error(t, err, "the account waits")
				assert.Contains(t, err.Error(), "15 minutes")
				assert.EqualValues(t, 1, dials.Load())
				return
			}
			require.NoError(t, err, "the next dial of the account goes at once")
			assert.EqualValues(t, 2, dials.Load())
		})
	}
}

// TestUnansweredWaitJitter: the wait is 15 minutes plus or minus 10%.
func TestUnansweredWaitJitter(t *testing.T) {
	g := NewGuard(clock.NewMock())
	g.jitter = func() float64 { return -1 }
	assert.Equal(t, 13*time.Minute+30*time.Second, g.jittered(unansweredWait))
	g.jitter = func() float64 { return 1 }
	assert.Equal(t, 16*time.Minute+30*time.Second, g.jittered(unansweredWait))
}

// TestAnAnsweredFailureIsNotUnanswered: only a dial that failed without a status
// after its AUTHENTICATE was sent waits. A status, a refusal before the
// AUTHENTICATE, and an answer the library cannot parse do not.
func TestAnAnsweredFailureIsNotUnanswered(t *testing.T) {
	cfg := Config{Host: "h", Share: "s", Username: "u", Password: "Passw0rd-1"}
	timeout := fmt.Errorf("session setup: %w", context.DeadlineExceeded)
	reset := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}

	for name, tc := range map[string]struct {
		err        error
		sent       bool
		unanswered bool
	}{
		"timeout after the AUTHENTICATE":    {timeout, true, true},
		"reset after the AUTHENTICATE":      {reset, true, true},
		"timeout before the AUTHENTICATE":   {timeout, false, false},
		"status after the AUTHENTICATE":     {status(statusAccessDenied), true, false},
		"refusal after the AUTHENTICATE":    {status(statusLogonFailure), true, false},
		"unparsable answer":                 {&protocol.InvalidResponseError{Message: "bad response"}, true, false},
		"guest session after AUTHENTICATE":  {&protocol.InvalidResponseError{Message: "guest account doesn't support signing"}, true, false},
		"not a transport failure, not sent": {errors.New("other"), false, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := dialError(tc.err, cfg, cfg.target(), tc.sent)
			assert.Equal(t, tc.unanswered, errors.Is(err, errUnanswered), "%v", err)
			if !errors.Is(err, errGuestSession) {
				assert.Equal(t, Classify(tc.err), Classify(err), "the kind does not change")
			}
		})
	}
}

// TestAuthenticateSentRecordsTheAuthenticate: the dialer's credentials say when
// the AUTHENTICATE message was made, which is when the server may count a bad
// password.
func TestAuthenticateSentRecordsTheAuthenticate(t *testing.T) {
	var sent atomic.Bool
	creds := recordAuthenticate(stubCredentials{&stubInitiator{}}, &sent)
	init, err := creds.NewInitiator(context.Background(), "host")
	require.NoError(t, err)
	_, err = init.InitSecContext()
	require.NoError(t, err)
	assert.False(t, sent.Load(), "the NEGOTIATE message carries no password")
	_, err = init.AcceptSecContext([]byte("challenge"))
	require.NoError(t, err)
	assert.True(t, sent.Load())

	sent.Store(false)
	failing, err := recordAuthenticate(stubCredentials{&stubInitiator{err: errors.New("bad challenge")}}, &sent).NewInitiator(context.Background(), "host")
	require.NoError(t, err)
	_, err = failing.AcceptSecContext([]byte("garbage"))
	require.Error(t, err)
	assert.False(t, sent.Load(), "nothing was sent when the challenge could not be answered")
}

type stubCredentials struct{ init auth.Initiator }

func (c stubCredentials) NewInitiator(context.Context, string) (auth.Initiator, error) {
	return c.init, nil
}

type stubInitiator struct {
	auth.Initiator
	err error
}

func (i *stubInitiator) InitSecContext() ([]byte, error) { return []byte("negotiate"), nil }
func (i *stubInitiator) AcceptSecContext([]byte) ([]byte, error) {
	return []byte("authenticate"), i.err
}

// TestShortPasswordKeepsTheStopStatus: a password that is a substring of the
// Agent's own text, such as "e", must not make the redaction replace the guard's
// error, which the status reads.
func TestShortPasswordKeepsTheStopStatus(t *testing.T) {
	clk := clock.NewMock()
	g := NewGuard(clk)
	srv := &logonServer{reply: refuse(statusLogonFailure)}
	first := reader(t, g, clk, srv, accountConfig("dc1.corp.example.com", "a", "e"))
	other := reader(t, g, clk, srv, accountConfig("dc2.corp.example.com", "b", "e"))
	for _, err := range []error{first(), other()} {
		require.ErrorIs(t, err, ErrLogonStopped)
		why, fix, _, ok := LogonStop(err)
		require.True(t, ok, "the status still says that the logons stopped: %v", err)
		assert.NotEmpty(t, why)
		assert.NotEmpty(t, fix)
	}
}
