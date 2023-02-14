// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package amqp

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/streadway/amqp"
)

type Options struct {
	ServerAddress string
	Username      string
	Password      string
	Dialer        *net.Dialer
}

type Client struct {
	opts           Options
	PublishConn    *amqp.Connection
	PublishChannel *amqp.Channel
	ConsumeConn    *amqp.Connection
	ConsumeChannel *amqp.Channel
}

func NewClient(opts Options) (*Client, error) {
	if opts.Username == "" {
		opts.Username = User
	}

	if opts.Password == "" {
		opts.Password = Pass
	}

	dialOptions := amqp.Config{}
	if opts.Dialer != nil {
		dialOptions.Dial = opts.Dialer.Dial
	}

	publishConn, err := amqp.DialConfig(fmt.Sprintf("amqp://%s:%s@%s/", opts.Username, opts.Password, opts.ServerAddress), dialOptions)
	if err != nil {
		return nil, err
	}
	publishCh, err := publishConn.Channel()
	if err != nil {
		return nil, err
	}
	consumeConn, err := amqp.DialConfig(fmt.Sprintf("amqp://%s:%s@%s/", opts.Username, opts.Password, opts.ServerAddress), dialOptions)
	if err != nil {
		return nil, err
	}
	consumeCh, err := consumeConn.Channel()
	if err != nil {
		return nil, err
	}
	return &Client{
		opts:           opts,
		PublishConn:    publishConn,
		PublishChannel: publishCh,
		ConsumeConn:    consumeConn,
		ConsumeChannel: consumeCh,
	}, nil
}

type Queue struct {
	Name string
}

func (c *Client) DeleteQueues() error {
	host, _, _ := net.SplitHostPort(c.opts.ServerAddress)
	manager := fmt.Sprintf("http://%s:15672/api/queues/", host)
	client := &http.Client{}
	req, err := http.NewRequest("GET", manager, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.opts.Username, c.opts.Password)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	queues := make([]Queue, 0)
	if err := json.NewDecoder(resp.Body).Decode(&queues); err != nil {
		return err
	}

	for _, queue := range queues {
		_, _ = c.PublishChannel.QueueDelete(queue.Name, false, false, false)
	}

	return nil
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
