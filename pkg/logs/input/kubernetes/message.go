/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2019 Datadog, Inc.
 */

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)


type Message struct {
	DDMessage *message.Message
	Flag string
}

func NewKubernetesMessage(content []byte, status string, timestamp string, flag string) *Message {
	return &Message{
		DDMessage: message.NewPartialMessage(content, status, timestamp),
		Flag: flag,
	}
}
