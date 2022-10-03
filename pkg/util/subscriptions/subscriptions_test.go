// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriptions

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestSubscribe(t *testing.T) {
	rx1 := NewReceiver[string]()
	rx2 := NewReceiver[string]()
	tx := NewTransmitter[string]([]Receiver[string]{rx1, rx2})

	tx.Notify("hello!")
	require.Equal(t, "hello!", <-rx1.Chan())
	require.Equal(t, "hello!", <-rx2.Chan())

	require.Equal(t, 0, len(rx1.Chan()))
	require.Equal(t, 0, len(rx2.Chan()))
}

// ---- rx receives messages

type RxComponent interface {
	GetMessage() string
}

type receiver struct {
	rx Receiver[string]
}

func newRx() (RxComponent, Subscription[string]) {
	sub := NewSubscription[string]()
	return &receiver{rx: sub.Receiver}, sub
}

func (rx *receiver) GetMessage() string {
	return <-rx.rx.Chan()
}

// --- tx sends messages

type TxComponent interface {
	SendMessage(string)
}

type transmitter struct {
	tx Transmitter[string]
}

func newTx(pub Publisher[string]) TxComponent {
	return &transmitter{
		tx: pub.Transmitter(),
	}
}

func (tx *transmitter) SendMessage(message string) {
	tx.tx.Notify(message)
}

func TestFx(t *testing.T) {
	var rx RxComponent
	var tx TxComponent
	fxutil.Test(t,
		fx.Options(
			fx.Provide(newRx),
			fx.Provide(newTx),
			fx.Populate(&rx),
			fx.Populate(&tx),
		), func() {
			tx.SendMessage("hello")
			require.Equal(t, "hello", rx.GetMessage())
		})
}
