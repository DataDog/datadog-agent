# Component Subscriptions

Subscriptions are a common form of registration, and have support in the `pkg/util/subscriptions` package.

In defining subscriptions, the component that transmits messages is the _collecting_ component, and the processes receiving components are the _providing_ components. These are matched using the message type, which must be unique across the codebase, and should not be a built-in type like `string`. Providing components provide a `subscriptions.Receiver[coll.Message]` which has a `Ch` channel from which to receive messages. Collecting components require a `subscriptions.Transmitter[coll.Message]` which has a `Notify` method to send messages.

=== ":octicons-file-code-16: announcer/component.go"
    ```go
    // ...
    // To subscribe to these announcements, provide a subscriptions.Subscription[announcer.Announcement].
    // ...
    package announcer
    ```

=== ":octicons-file-code-16: announcer/announcer.go"
    ```go
    func newAnnouncer(tx subscriptions.Transmitter[Anouncement]) Component {
        return &announcer{announcementTx: tx}  // (store the transmitter)
    }

    // ... later send messages with
    func (ann *announcer) announce(a announcement) {
        ann.annoucementTx.Notify(a)
    }
    ```

=== ":octicons-file-code-16: listener/listener.go"
    ```go
    func newListener() (Component, subscriptions.Receiver[announcer.Announcement]) {
        rx := subscriptions.Receiver[Event]() // create a receiver
        return &listener{announcementRx: rx}, rx  // capture the receiver _and_ return it
    }

    // ... later receive messages (usually in an actor's main loop)
    func (l *listener) run() {
        loop {
            select {
            case a := <- l.announcementRx.Ch:
                ...
            }
        }
    }
    ```

Any component receiving messages via a subscription will _automatically_ be instantiated by Fx if it is delcared in the app, regardless of whether its Component type is required by some other component. The workaround for this is to return a zero-valued Receiver when the component does not actually wish to receive messages (such as when the component is disabled by user configuration).

If a receiving component does not subscribe (for example, if it is not started), it can return the zero value, `subscriptions.Receiver[Event]{}`, from its constructor. If a component returns a non-nil subscriber, it _must_ consume messages from the receiver or risk blocking the transmitter.

See the `pkg/util/subscriptions` documentation for more details.
