// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	mrand "math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
)

// lockoutProbe is how often the Agent tries a locked-out account again, give or
// take 10%.
const lockoutProbe = time.Hour

// unansweredWait is how long an account and password wait after a logon that was
// sent and never answered, give or take 10%.
const unansweredWait = 15 * time.Minute

// ErrLogonStopped is wrapped by the error of a dial the Guard did not send, or
// whose logon the server refused. It classifies as ErrAuth.
var ErrLogonStopped = errors.New("logons of the account are stopped")

// Guard keeps a wrong password from locking an account out. A domain locks an
// account after a few bad logons, whatever their source, and every source and
// server of the account retries on its own: a mistyped password sent by 20
// sources every 30 seconds locks it at once, for every service that uses it.
//
// The account is the lower-cased domain and user, or the user alone when the
// source sets no domain, never the host: with no domain the server's challenge
// chooses it, and a domain-joined server chooses the same one whatever its name.
// A DOMAIN\ prefix or an @suffix of the user name is not part of the user. The
// cost: a refusal by one server also stops the same user and password on every
// other server, until the password changes or the Agent restarts. The password
// is told apart by a salted SHA-256 fingerprint, kept in memory only. The Guard
// sends one logon at a time per account (the dials that arrive meanwhile wait
// for its outcome and use the verdict), and:
//   - after STATUS_LOGON_FAILURE, WRONG_PASSWORD or NO_SUCH_USER, a guest
//     session (Samba maps a wrong password to guest, and the domain controller
//     counted it), or an account the server says is disabled, expired or needs
//     a new password, it sends no logon with that account and password, on any
//     host or share, until the password changes (a refreshed secret or a changed
//     configuration has another fingerprint) or the Agent restarts;
//   - a locked-out account is tried once an hour, give or take 10%;
//   - a logon that was sent and never answered (the dial timed out or the
//     connection failed after the AUTHENTICATE) may still have been counted by a
//     slow domain controller: that account and password wait 15 minutes, give or
//     take 10%, then dial again. It is no stop. A dial whose context was
//     cancelled, because its source was replaced or removed, starts no wait;
//   - nothing else counts: a network error before the AUTHENTICATE, an
//     unreachable domain controller, a failed trust, a refused tree connect, an
//     unknown share or a server that does not encrypt says nothing about the
//     password, and keeps the backoff of the client that got it.
//
// The Guard of the process outlives the launcher, so a restart of the logs
// agent does not send the refused password again.
type Guard struct {
	clock  clock.Clock
	salt   [16]byte
	jitter func() float64 // in [-1, 1], scaled to +-10% of lockoutProbe; a seam for tests

	mu       sync.Mutex
	accounts map[string]*account
}

type account struct {
	busy  chan struct{}         // closed when the logon in flight ends; nil when none
	stops map[[32]byte]*stopped // by password fingerprint
	waits map[[32]byte]waiting  // by password fingerprint: logons that were sent and never answered
}

// waiting is a password whose last logon got no answer: no other logon goes
// before until, which the error of the dials in between says.
type waiting struct {
	until time.Time
	err   error
}

// stopped is a password the server refused.
type stopped struct {
	code    uint32
	probeAt time.Time // when a locked-out account is tried again; zero when only a new password ends it
}

var processGuard = NewGuard(clock.New())

// ProcessGuard returns the Guard of the process, which the SMB sources share.
func ProcessGuard() *Guard { return processGuard }

// NewGuard returns a Guard that reads time from clk.
func NewGuard(clk clock.Clock) *Guard {
	g := &Guard{clock: clk, accounts: make(map[string]*account), jitter: func() float64 { return 2*mrand.Float64() - 1 }}
	_, _ = rand.Read(g.salt[:])
	return g
}

// WithGuard makes the client send its logons through g.
func WithGuard(g *Guard) Option {
	return func(r *reconnecting) { r.guard = g }
}

// Dial calls dial for cfg unless the server refused the account's password, and
// holds the account's logon slot while it runs. A refused logon returns an error
// that wraps ErrLogonStopped and the server's answer.
func (g *Guard) Dial(ctx context.Context, cfg Config, dial DialFunc) (Client, error) {
	fp := sha256.Sum256(append(g.salt[:], cfg.Password...))
	a, err := g.begin(ctx, accountKey(cfg), fp)
	if err != nil {
		return nil, err
	}
	c, err := dial(ctx, cfg)
	return c, g.end(ctx, a, fp, err)
}

// accountKey is the lower-cased domain and user of cfg, the user alone with no
// domain, without the DOMAIN\ prefix or the @suffix of the user name.
func accountKey(cfg Config) string {
	user := cfg.Username
	if i := strings.LastIndexByte(user, '\\'); i >= 0 {
		user = user[i+1:]
	}
	user, _, _ = strings.Cut(user, "@")
	return strings.ToLower(cfg.Domain + "\x00" + user)
}

// begin waits for the logon in flight of the account, then takes its slot, or
// returns the stop that applies to the password.
func (g *Guard) begin(ctx context.Context, key string, fp [32]byte) (*account, error) {
	for {
		g.mu.Lock()
		a := g.accounts[key]
		if a == nil {
			a = &account{stops: make(map[[32]byte]*stopped), waits: make(map[[32]byte]waiting)}
			g.accounts[key] = a
		}
		if busy := a.busy; busy != nil {
			g.mu.Unlock()
			select {
			case <-busy:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if st := a.stops[fp]; st != nil && (st.probeAt.IsZero() || g.clock.Now().Before(st.probeAt)) {
			g.mu.Unlock()
			return nil, checked(&stoppedError{code: st.code})
		}
		if w, ok := a.waits[fp]; ok && g.clock.Now().Before(w.until) {
			g.mu.Unlock()
			return nil, w.err
		}
		a.busy = make(chan struct{})
		g.mu.Unlock()
		return a, nil
	}
}

// end releases the logon slot and records what the server answered. A dial that
// failed because ctx was cancelled (the source was replaced or removed, the
// logs agent stops) says nothing about the server and records nothing.
func (g *Guard) end(ctx context.Context, a *account, fp [32]byte, err error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	close(a.busy)
	a.busy = nil
	delete(a.waits, fp) // the logon was answered, or it starts another wait below
	if err == nil {
		delete(a.stops, fp) // a locked-out account that unlocked
		return nil
	}
	code, ok := logonStatus(err)
	if errors.Is(err, errGuestSession) {
		code, ok = codeGuestOnly, true
	}
	switch {
	case errors.Is(err, errUnanswered) && errors.Is(ctx.Err(), context.Canceled):
		return err
	case errors.Is(err, errUnanswered):
		wait := g.jittered(unansweredWait)
		a.waits[fp] = waiting{until: g.clock.Now().Add(wait), err: &unansweredWaitError{err}}
		return err
	case ok && code == statusAccountLockedOut:
		a.stops[fp] = &stopped{code: code, probeAt: g.clock.Now().Add(g.jittered(lockoutProbe))}
	case ok && refusals[code]:
		a.stops[fp] = &stopped{code: code}
	default:
		return err // says nothing about the password
	}
	return checked(&stoppedError{code: code, cause: err})
}

// jittered returns d give or take 10%, so that the Agents of a fleet do not try
// an account together.
func (g *Guard) jittered(d time.Duration) time.Duration {
	return d + time.Duration(g.jitter()*float64(d)/10)
}

// codeGuestOnly stands for a guest session in the codes of a stop. It is no
// NTSTATUS: the library reports a guest session without one.
const codeGuestOnly = 0xFFFFFFFF

// refusals are the statuses of a session setup that stop the logons of the
// account and password until the password changes.
var refusals = map[uint32]bool{
	codeGuestOnly:            true,
	statusLogonFailure:       true,
	statusWrongPassword:      true,
	statusNoSuchUser:         true,
	statusAccountDisabled:    true,
	statusAccountExpired:     true,
	statusPasswordExpired:    true,
	statusPasswordMustChange: true,
}

// errNotALogon marks the errors of Dial that come after the server accepted the
// credentials, such as the tree connect: they say nothing about the password.
// errUnanswered marks the errors of a dial whose AUTHENTICATE was sent and that
// failed with no answer from the server.
var (
	errNotALogon  = errors.New("not a logon")
	errUnanswered = errors.New("logon unanswered")
)

// markedError is an error that matches mark with errors.Is.
type markedError struct {
	error
	mark error
}

func (e *markedError) Unwrap() error        { return e.error }
func (e *markedError) Is(target error) bool { return target == e.mark }

// notALogon marks err as an error of the mount of the share, after the logon.
func notALogon(err error) error { return &markedError{err, errNotALogon} }

// unanswered marks err as the failure of a dial whose logon got no answer.
func unanswered(err error) error { return &markedError{err, errUnanswered} }

// unansweredWaitError is the error of the dials the Guard did not send because
// the last logon of the account and password got no answer. It classifies like
// that failure, as a Transient error.
type unansweredWaitError struct{ err error }

func (e *unansweredWaitError) Error() string {
	return "the server did not answer the last logon of this account, which it may still have counted as a bad password: the Agent sends no other logon with it for 15 minutes, so that a slow domain controller cannot lock it out; the failure was: " + e.err.Error()
}

func (e *unansweredWaitError) Unwrap() error { return e.err }

// logonStatus returns the status code the server answered the logon with.
func logonStatus(err error) (uint32, bool) {
	if errors.Is(err, errNotALogon) {
		return 0, false
	}
	return statusCode(err)
}

// stoppedError is the error of a dial the Guard did not send because the server
// refused the account's password (cause is nil), or of the dial the server
// refused (cause is its answer).
type stoppedError struct {
	code  uint32
	cause error
}

func (e *stoppedError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	why, _, _ := stopReason(e.code)
	return "the Agent sends no more logons for this account, which the server refused earlier: " + why
}

func (e *stoppedError) Unwrap() error        { return e.cause }
func (e *stoppedError) Is(target error) bool { return target == ErrLogonStopped }

// stopReason says why a status code stops the account's logons, what to do about
// it, and whether the Agent tries the account again by itself.
func stopReason(code uint32) (why, fix string, probes bool) {
	const newSecret = "refresh the secret or restart the Agent"
	switch code {
	case codeGuestOnly:
		return "the server granted only guest access to the account, which the Agent refuses: a guest session cannot sign messages", "Fix the account mapping on the server (for example Samba's map to guest) or the password, then " + newSecret, false
	case statusAccountLockedOut:
		return "the account is locked out", "It tries the account again once an hour, and at once with a new password: if the password is wrong, fix it, then " + newSecret, true
	case statusAccountDisabled:
		return "the account is disabled", "Enable the account, then restart the Agent: the password is the same, so a refreshed secret does not make it try again", false
	case statusAccountExpired:
		return "the account has expired", "Extend the account, then restart the Agent: the password is the same, so a refreshed secret does not make it try again", false
	case statusPasswordExpired, statusPasswordMustChange:
		return "the password of the account has expired and must be changed", "Change the password, then refresh the secret with the new one", false
	default:
		return "the server rejected the credentials: the user name or the password is wrong", "Fix the password, then " + newSecret, false
	}
}

// LogonStop reports whether err comes from the Guard stopping the logons of an
// account, with the reason, which names no server and no share, what to do
// about it, and whether the Agent tries the account again by itself, as it does
// for a locked-out one.
func LogonStop(err error) (why, fix string, probes, ok bool) {
	var stop *stoppedError
	if !errors.As(err, &stop) {
		return "", "", false, false
	}
	why, fix, probes = stopReason(stop.code)
	return why, fix, probes, true
}
