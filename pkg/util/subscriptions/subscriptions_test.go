// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// ---- msg is a message

type msg struct {
	s string
}

// ---- params defines parameters for the test

type params struct {
	// shouldReceive determines whether the receiver should register to receive
	// messages
	shouldReceive bool
}

// ---- rx receives messages

type RxComponent interface {
	GetMessage() string
}

type rx struct {
	receiver Receiver[msg]
}

func newRx(params params) (RxComponent, Receiver[msg]) {
	if !params.shouldReceive {
		return &rx{}, Receiver[msg]{}
	}

	receiver := NewReceiver[msg]()
	return &rx{receiver}, receiver
}

func (rx *rx) GetMessage() string {
	if rx.receiver.Ch == nil {
		panic("this component is not receiving")
	}
	return (<-rx.receiver.Ch).s
}

// ---- tx sends messages

type TxComponent interface {
	SendMessage(string)
}

type tx struct {
	transmitter Transmitter[msg]
}

func newTx(transmitter Transmitter[msg]) TxComponent {
	return &tx{transmitter}
}

func (tx *tx) SendMessage(message string) {
	tx.transmitter.Notify(msg{s: message})
}

// ---- tests

func TestReceiving(t *testing.T) {
	var rx RxComponent
	var tx TxComponent
	fxutil.Test(t,
		fx.Options(
			fx.Supply(params{shouldReceive: true}),
			fx.Provide(newRx),
			fx.Provide(newTx),
			fx.Populate(&rx),
			fx.Populate(&tx),
		), func() {
			tx.SendMessage("hello")
			require.Equal(t, "hello", rx.GetMessage())
		})
}

func TestNotReceiving(t *testing.T) {
	var rx RxComponent
	var tx TxComponent
	fxutil.Test(t,
		fx.Options(
			fx.Supply(params{shouldReceive: false}),
			fx.Provide(newRx),
			fx.Provide(newTx),
			fx.Populate(&rx),
			fx.Populate(&tx),
		), func() {
			// send three messages to ensure any buffered channels fill
			// up and block (there shouldn't be any channels!)
			tx.SendMessage("hello")
			tx.SendMessage("cruel")
			tx.SendMessage("world")
		})
}
