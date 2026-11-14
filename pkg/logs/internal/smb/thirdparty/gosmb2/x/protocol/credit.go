package protocol

import (
	"context"
	"errors"
	"math"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// The tree connection handles this before any part of a compound is sent.
var errCompoundCredits = errors.New("compound requires sequential requests")

type account struct {
	m                sync.Mutex
	notify           chan struct{}
	closed           bool   // set once the account is aborted; no further loans are possible
	closeErr         error  // error returned to pending loans after abort
	maxCreditBalance uint16 // configured maximum credit balance (e.g., 128)
	availableCredits uint16 // credits currently available in sequence window
	inFlightCredits  uint16 // credits currently in flight
	maxCredits       uint16 // maximum observed credits granted by the server
	nextMessageId    uint64
	creditTimeout    time.Duration
}

// saturatingAddUint16 adds two uint16 values, clamping the result at math.MaxUint16
// to prevent wraparound of the credit sequence window.
func saturatingAddUint16(a, b uint16) uint16 {
	sum := uint32(a) + uint32(b)
	if sum > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(sum)
}

func openAccount(maxCreditBalance uint16) *account {
	return &account{
		notify:           make(chan struct{}),
		maxCreditBalance: maxCreditBalance,
		availableCredits: 1, // MS-SMB2 3.3.1.2 / 3.2.4.1.6: initial credit is 1
		maxCredits:       1,
		nextMessageId:    0,
		creditTimeout:    clientCreditTimeout,
	}
}

// notifyWaitersLocked wakes every loan currently waiting for credits. The
// caller must hold a.m. The channel is closed to broadcast the wakeup: a
// waiter that still cannot proceed (for example, because it needs more
// credits than were replenished) must not consume the only wakeup and leave
// the remaining waiters blocked. The closed channel is replaced immediately
// under the same lock so that later waiters block on a fresh channel. No
// notification is lost because close happens before the replacement and the
// waiters capture the channel while holding a.m.
//
// The wait/notify scheme itself is client-side and implementation-defined per
// [MS-SMB2] 3.2.4.1.2, but it must preserve the requirement of [MS-SMB2]
// 3.2.4.1.3 that a request only proceeds once it can reserve its CreditCharge
// from the available credits.
func (a *account) notifyWaitersLocked() {
	close(a.notify)
	a.notify = make(chan struct{})
}

// abort closes the account so that any pending or subsequent loan fails
// immediately with err instead of blocking forever on credit availability.
func (a *account) abort(err error) {
	a.m.Lock()
	if a.closed {
		a.m.Unlock()
		return
	}
	if err == nil {
		err = &TransportError{Err: net.ErrClosed}
	}
	a.closeErr = err
	a.closed = true
	a.notifyWaitersLocked()
	a.m.Unlock()
}

// [MS-SMB2] 3.1.5.2 calculates CreditCharge from the payload size. Keep the
// calculation wide until it is known to fit the uint16 wire field, so the
// consecutive MessageIds required by [MS-SMB2] 3.2.4.1.5 are not undercounted.
func calcCreditCharge(payloadSize uint64) (uint16, error) {
	if payloadSize == 0 {
		return 1, nil
	}

	charge := (payloadSize-1)/uint64(maxSingleCreditPayloadSize) + 1
	if charge > math.MaxUint16 {
		return 0, errors.New("protocol: credit charge exceeds uint16")
	}
	return uint16(charge), nil
}

func (a *account) maxCreditCap() uint16 {
	a.m.Lock()
	defer a.m.Unlock()

	cap := a.maxCredits
	if a.maxCreditBalance > 0 && cap > a.maxCreditBalance {
		cap = a.maxCreditBalance
	}
	if cap < 1 {
		cap = 1
	}
	return cap
}

// reserve waits for credit counts and sets charge/request fields without
// consuming MessageIds. Callers must wait before acquiring send ownership.
func (a *account) reserve(ctx context.Context, reqs ...wire.Packet) (charges []uint16, totalCreditCharge uint16, err error) {
	// Charges are accumulated in uint32 to detect overflow of the uint16 wire field.
	if len(reqs) == 0 {
		return nil, 0, nil
	}

	charges = make([]uint16, len(reqs))
	var total uint32
	for i, req := range reqs {
		var cc uint16
		switch r := req.(type) {
		case *DirectReadRequest:
			cc, err = calcCreditCharge(uint64(r.Length))
		case *wire.ReadRequest:
			cc, err = calcCreditCharge(uint64(r.Length))
		case *wire.WriteRequest:
			cc, err = calcCreditCharge(uint64(len(r.Data)))
		case *wire.IoctlRequest:
			var inputSize uint64
			if r.Input != nil {
				size := r.Input.Size()
				if size < 0 {
					return nil, 0, errors.New("protocol: negative IOCTL input size")
				}
				inputSize = uint64(size)
			}
			// [MS-SMB2] 3.3.5.15 validates credits using the larger of the
			// request and Response buffer sums. Widen before adding.
			requestSize := inputSize + uint64(r.OutputCount)
			responseSize := uint64(r.MaxInputResponse) + uint64(r.MaxOutputResponse)
			cc, err = calcCreditCharge(max(requestSize, responseSize))
		case *wire.QueryDirectoryRequest:
			cc, err = calcCreditCharge(uint64(r.OutputBufferLength))
		case *wire.QueryInfoRequest:
			var inputSize uint64
			if r.Input != nil {
				size := r.Input.Size()
				if size < 0 {
					return nil, 0, errors.New("protocol: negative QUERY_INFO input size")
				}
				inputSize = uint64(size)
			}
			// [MS-SMB2] 3.3.5.20 requires the server to validate CreditCharge
			// against max(InputBufferLength, OutputBufferLength). That
			// contradicts 3.2.4.1.5, which tells the client to send 1 for every
			// command other than READ/WRITE/IOCTL/QUERY_DIRECTORY. Samba 4.19
			// enforces the server-side rule and rejects a >64 KiB QUERY_INFO
			// sent with CreditCharge 1, so the server-side document is adopted
			// deliberately. Do not revert this to a fixed charge of 1.
			cc, err = calcCreditCharge(max(inputSize, uint64(r.OutputBufferLength)))
		case *wire.SetInfoRequest:
			var inputSize uint64
			if r.Input != nil {
				size := r.Input.Size()
				if size < 0 {
					return nil, 0, errors.New("protocol: negative SET_INFO input size")
				}
				inputSize = uint64(size)
			}
			// [MS-SMB2] 3.3.5.21 requires the server to validate CreditCharge
			// against BufferLength. Like QUERY_INFO above, this contradicts
			// 3.2.4.1.5, and Samba 4.19 enforces the server-side rule, so the
			// server-side document is adopted deliberately. Do not revert this
			// to a fixed charge of 1.
			cc, err = calcCreditCharge(inputSize)
		default:
			cc = req.CreditCharge()
		}
		if err != nil {
			return nil, 0, err
		}
		charges[i] = cc
		total += uint32(cc)
		if total > math.MaxUint16 {
			return nil, 0, errors.New("protocol: compound credit charge exceeds uint16")
		}
	}

	a.m.Lock()
	maxPossible := max(a.maxCreditBalance, 1)
	if total > math.MaxUint16 || total > uint32(maxPossible) {
		a.m.Unlock()
		if len(reqs) > 1 && total <= math.MaxUint16 {
			return nil, 0, errCompoundCredits
		}
		return nil, 0, errors.New("protocol: requested credit charge exceeds maximum credit balance")
	}
	totalCreditCharge = uint16(total)
	a.m.Unlock()

	var timeout <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-timeout:
			return nil, 0, context.DeadlineExceeded
		default:
		}

		a.m.Lock()

		if a.closed {
			err := a.closeErr
			a.m.Unlock()
			return nil, 0, err
		}

		if a.availableCredits >= totalCreditCharge {
			a.availableCredits -= totalCreditCharge
			a.inFlightCredits += totalCreditCharge

			var creditRequest uint16
			// MS-SMB2 3.2.4.1.2:
			// Request credits sufficient to maintain total outstanding limit at maxCreditBalance.
			balance := int32(a.availableCredits) + int32(a.inFlightCredits) - int32(totalCreditCharge)
			needed := int32(a.maxCreditBalance) - balance
			if needed > 0 {
				creditRequest = uint16(needed)
			}

			a.m.Unlock()

			for i, req := range reqs {
				switch req.(type) {
				case *DirectReadRequest, *wire.ReadRequest, *wire.WriteRequest, *wire.IoctlRequest, *wire.QueryDirectoryRequest, *wire.QueryInfoRequest, *wire.SetInfoRequest:
					req.SetCreditCharge(charges[i])
				}
			}

			// [MS-SMB2] 3.2.4.1.2 and 3.2.4.1.4 require each compound
			// request to replenish its charge when maintaining the balance.
			// Preserve the total request when adjusting the balance: assign
			// up to each charge, then put any extra on the first request.
			// The assignments sum to creditRequest, so each fits in uint16.
			remaining := uint32(creditRequest)
			assigned := make([]uint32, len(reqs))
			for i, charge := range charges {
				assigned[i] = min(uint32(charge), remaining)
				remaining -= assigned[i]
			}
			assigned[0] += remaining
			for i, req := range reqs {
				req.SetCreditRequest(uint16(assigned[i]))
			}

			return charges, totalCreditCharge, nil
		}
		// With no requests in flight, make progress using the credits already
		// granted rather than depending on a future unrelated operation. A
		// compound can be sent separately; an indivisible multi-credit request
		// exceeding this idle window hits our local limit ([MS-SMB2] 3.2.4.1.3).
		if a.inFlightCredits == 0 {
			available := a.availableCredits
			a.m.Unlock()
			if available == 0 {
				return nil, 0, errors.New("protocol: no credits available")
			}
			if len(reqs) > 1 {
				return nil, 0, errCompoundCredits
			}
			return nil, 0, errors.New("protocol: requested credit charge exceeds idle credit window")
		}
		// Capture the current notification channel under the lock so that a
		// replenishment racing with this wait cannot be missed. The channel is
		// closed (not sent to) by notifyWaitersLocked, so a waiter that was
		// woken but still lacks credits simply waits on the replacement channel.
		notify := a.notify
		a.m.Unlock()

		// Bound the entire credit wait, including retries after insufficient
		// grants. Keep this timer local so it cannot cancel a sent request or
		// affect other requests sharing the account.
		if timeout == nil {
			to := a.creditTimeout
			if to <= 0 {
				to = clientCreditTimeout
			}
			timer := time.NewTimer(to)
			defer timer.Stop()
			timeout = timer.C
		}

		select {
		case <-notify:
			// Replenished, retry loan
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-timeout:
			return nil, 0, context.DeadlineExceeded
		}
	}
}

// charge replenishes credits granted by server Response.
func (a *account) charge(granted uint16, consumed ...uint16) {
	var c uint16
	if len(consumed) > 0 {
		c = consumed[0]
	}
	if granted == 0 && c == 0 {
		return
	}

	a.m.Lock()
	if a.inFlightCredits >= c {
		a.inFlightCredits -= c
	} else {
		a.inFlightCredits = 0
	}
	// Saturate instead of wrapping around the uint16 wire field.
	a.availableCredits = saturatingAddUint16(a.availableCredits, granted)
	if a.availableCredits > a.maxCredits {
		a.maxCredits = a.availableCredits
	}
	a.notifyWaitersLocked()
	a.m.Unlock()
}

// unloan restores credits if sending a packet fails before network transmission.
func (a *account) unloan(creditCharge uint16) {
	if creditCharge == 0 {
		return
	}

	a.m.Lock()
	// Saturate instead of wrapping around the uint16 wire field.
	a.availableCredits = saturatingAddUint16(a.availableCredits, creditCharge)
	if a.inFlightCredits >= creditCharge {
		a.inFlightCredits -= creditCharge
	} else {
		a.inFlightCredits = 0
	}
	if a.availableCredits > a.maxCredits {
		a.maxCredits = a.availableCredits
	}
	a.notifyWaitersLocked()
	a.m.Unlock()
}

// assignIDs consumes the reserved counts' identifiers immediately before
// encoding. Ordinary requests call this only while holding conn.m, so no later
// request can publish an ID before this request succeeds or rolls back.
func (a *account) assignIDs(charges []uint16, reqs ...wire.Packet) []uint64 {
	a.m.Lock()
	defer a.m.Unlock()
	ids := make([]uint64, len(reqs))
	for i, req := range reqs {
		ids[i] = a.nextMessageId
		req.SetMessageId(ids[i])
		a.nextMessageId += uint64(charges[i])
	}
	return ids
}

// rollbackIDs restores only the current unpublished suffix. conn.m must remain
// held from assignIDs through rollback; count reservations alone consume no IDs.
func (a *account) rollbackIDs(charge uint16) {
	a.m.Lock()
	a.nextMessageId -= uint64(charge)
	a.m.Unlock()
}
