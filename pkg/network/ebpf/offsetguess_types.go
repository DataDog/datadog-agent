//+build ignore

package ebpf

/*
#include "./c/prebuilt/offset-guess.h"
*/
import "C"

type Proc C.proc_t

const ProcCommMaxLen = C.TASK_COMM_LEN - 1

type TracerStatus C.tracer_status_t

type TracerState uint8

const (
	StateUninitialized TracerState = C.TRACER_STATE_UNINITIALIZED
	StateChecking      TracerState = C.TRACER_STATE_CHECKING // status set by userspace, waiting for eBPF
	StateChecked       TracerState = C.TRACER_STATE_CHECKED  // status set by eBPF, waiting for userspace
	StateReady         TracerState = C.TRACER_STATE_READY    // fully initialized, all offset known
)

type GuessWhat uint64

const (
	GuessSAddr     GuessWhat = C.GUESS_SADDR
	GuessDAddr     GuessWhat = C.GUESS_DADDR
	GuessFamily    GuessWhat = C.GUESS_FAMILY
	GuessSPort     GuessWhat = C.GUESS_SPORT
	GuessDPort     GuessWhat = C.GUESS_DPORT
	GuessNetNS     GuessWhat = C.GUESS_NETNS
	GuessRTT       GuessWhat = C.GUESS_RTT
	GuessDAddrIPv6 GuessWhat = C.GUESS_DADDR_IPV6
	// Following values are associated with an UDP connection, used for guessing offsets
	// in the flowi4 data structure
	GuessSAddrFl4 GuessWhat = C.GUESS_SADDR_FL4
	GuessDAddrFl4 GuessWhat = C.GUESS_DADDR_FL4
	GuessSPortFl4 GuessWhat = C.GUESS_SPORT_FL4
	GuessDPortFl4 GuessWhat = C.GUESS_DPORT_FL4
	// Following values are associated with an UDPv6 connection, used for guessing offsets
	// in the flowi6 data structure
	GuessSAddrFl6 GuessWhat = C.GUESS_SADDR_FL6
	GuessDAddrFl6 GuessWhat = C.GUESS_DADDR_FL6
	GuessSPortFl6 GuessWhat = C.GUESS_SPORT_FL6
	GuessDPortFl6 GuessWhat = C.GUESS_DPORT_FL6
	GuessSocketSK GuessWhat = C.GUESS_SOCKET_SK

	GuessNotApplicable GuessWhat = 99999
)
