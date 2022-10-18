// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kafka

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

type Client struct {
	ServerAddress string
	dialer        *kafka.Dialer
}

func NewClient(localAddress, serverAddress string) *Client {
	return &Client{
		ServerAddress: serverAddress,
		dialer: &kafka.Dialer{
			LocalAddr: &net.TCPAddr{
				IP: net.ParseIP(localAddress),
			},
			Timeout: 10 * time.Second,
		},
	}
}

func (c Client) CreateTopic(topicName string) error {
	conn, err := c.dialer.Dial("tcp", c.ServerAddress)
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	var controllerConn *kafka.Conn
	controllerConn, err = c.dialer.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             topicName,
			NumPartitions:     1,
			ReplicationFactor: 1,
		},
	}

	return controllerConn.CreateTopics(topicConfigs...)
}

func (c Client) Produce(topicName string, messages ...[]byte) error {
	conn, err := c.dialer.DialLeader(context.Background(), "tcp", c.ServerAddress, topicName, 0)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	messageArr := make([]kafka.Message, len(messages))

	for i, msg := range messages {
		messageArr[i] = kafka.Message{Value: msg, Topic: topicName}
	}

	_, err = conn.WriteMessages(messageArr...)
	return err
}

func (c Client) Fetch(topicName string) ([][]byte, error) {
	conn, err := c.dialer.DialLeader(context.Background(), "tcp", c.ServerAddress, topicName, 0)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	messages := make([][]byte, 0)
	for {
		msg, err := conn.ReadMessage(100)
		if err != nil {
			break
		}
		messages = append(messages, msg.Value)
	}

	return messages, nil
}
