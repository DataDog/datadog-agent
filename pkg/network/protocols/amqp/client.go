// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package amqp

import (
	"fmt"
	"github.com/streadway/amqp"
	"sync"
)

type Options struct {
	ServerAddress string
	Username      string
	Password      string
}

type Client struct {
	PublishConn    *amqp.Connection
	PublishChannel *amqp.Channel
	ConsumeConn    *amqp.Connection
	ConsumeChannel *amqp.Channel
}

func NewClient(opts Options) (*Client, error) {
	user := opts.Username
	if user == "" {
		user = User
	}

	pass := opts.Password
	if pass == "" {
		pass = Pass
	}

	publishConn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s/", user, pass, opts.ServerAddress))
	if err != nil {
		return nil, err
	}
	publishCh, err := publishConn.Channel()
	if err != nil {
		return nil, err
	}
	consumeConn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s/", user, pass, opts.ServerAddress))
	if err != nil {
		return nil, err
	}
	consumeCh, err := consumeConn.Channel()
	if err != nil {
		return nil, err
	}
	return &Client{
		PublishConn:    publishConn,
		PublishChannel: publishCh,
		ConsumeConn:    consumeConn,
		ConsumeChannel: consumeCh,
	}, nil
}

func (c *Client) Terminate() {
	c.PublishChannel.Close()
	c.ConsumeChannel.Close()
	c.PublishConn.Close()
	c.ConsumeConn.Close()
}

func (c *Client) DeclareQueue(name string, ch *amqp.Channel) error {
	_, err := ch.QueueDeclare(
		name,  // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	return err
}

func (c *Client) Publish(queue, body string) error {
	return c.PublishChannel.Publish(
		"",    // exchange
		queue, // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(body),
		})
}

func (c *Client) Consume(queue string, numberOfMessages int) ([]string, error) {
	msgs, err := c.ConsumeChannel.Consume(
		queue,
		"",    // consumer
		true,  // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)

	if err != nil {
		return nil, err
	}

	res := make([]string, 0, numberOfMessages)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range msgs {
			res = append(res, string(msg.Body))
			if len(res) == numberOfMessages {
				return
			}
		}
	}()

	wg.Wait()

	return res, nil
}
