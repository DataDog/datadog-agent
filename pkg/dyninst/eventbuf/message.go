// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package eventbuf

import (
	"iter"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// Message is an opaque handle to a single event fragment. The buffer does not
// inspect the fragment contents; it only needs to keep them alive, read their
// byte length for budget accounting, produce a FragmentedEvent view when
// surfacing them to the caller, and release them when done.
type Message interface {
	// Event returns the raw event bytes for this fragment. The slice must be
	// stable until Release is called.
	Event() output.Event
	// Release returns the underlying storage to the pool / dispatcher. After
	// Release returns, the Message must not be used.
	Release()
}

// MessageList is a pooled linked-list node holding a Message. A list of nodes
// represents the fragments of a single logical event. Single-fragment events
// are one node with next == nil.
//
// *MessageList implements output.FragmentedEvent so the decoder can iterate
// across fragments without copying.
type MessageList struct {
	msg  Message
	next *MessageList
}

var messageListPool = sync.Pool{
	New: func() any { return new(MessageList) },
}

// NewMessageList returns a pooled list head holding msg.
func NewMessageList(msg Message) *MessageList {
	n := messageListPool.Get().(*MessageList)
	n.msg = msg
	n.next = nil
	return n
}

// Append adds a message to the end of the list.
func (n *MessageList) Append(msg Message) {
	tail := n
	for tail.next != nil {
		tail = tail.next
	}
	tail.next = NewMessageList(msg)
}

// Fragments implements output.FragmentedEvent: yields the event from each
// node in the list.
func (n *MessageList) Fragments() iter.Seq[output.Event] {
	return func(yield func(output.Event) bool) {
		for cur := n; cur != nil; cur = cur.next {
			if !yield(cur.msg.Event()) {
				return
			}
		}
	}
}

// Release releases all messages in the list and returns the nodes to the
// pool.
func (n *MessageList) Release() {
	for cur := n; cur != nil; {
		next := cur.next
		cur.msg.Release()
		cur.next = nil
		cur.msg = nil
		messageListPool.Put(cur)
		cur = next
	}
}

// TotalSize returns the sum of event byte lengths across all fragments.
func (n *MessageList) TotalSize() int {
	size := 0
	for cur := n; cur != nil; cur = cur.next {
		size += len(cur.msg.Event())
	}
	return size
}

// Head returns the event from the head node (first fragment).
func (n *MessageList) Head() output.Event {
	return n.msg.Event()
}
