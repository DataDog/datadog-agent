// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicetype provides functionality to detect the service type for a given process.
package servicetype

// ServiceType represents a service type.
type ServiceType string

const (
	// Unknown is used when the service type could not be detected.
	Unknown ServiceType = "unknown"
	// WebService represents web services.
	WebService ServiceType = "web_service"
	// DB represents database services.
	DB ServiceType = "db"
	// Queue represents queue services.
	Queue ServiceType = "queue"
	// Storage represents storage services.
	Storage ServiceType = "storage"
	// FrontEnd represents frontend services.
	FrontEnd ServiceType = "front_end"
	// ThirdParty is used for third party services.
	ThirdParty ServiceType = "third_party"
)

var (
	/*
		Don't ever put in 8000, 8080, 8443, 8888, 9000
		or other ports that get used for many different
		web services and are commonly used inside companies.
		Let those fall through as WebService.
	*/
	portMap = map[int]ServiceType{
		// Postgres, MySQL, Oracle
		5432: DB,
		3306: DB,
		2483: DB,
		2484: DB,
		1521: DB,

		// Redit, Mongo
		9443:  DB,
		27017: DB,
		27018: DB,
		27019: DB,
		27020: DB,

		// Elastic
		9200: Storage,
		// Solr
		8983: Storage,
		// Sphinx
		9312: Storage,
		9306: Storage,

		// RabbitMQ
		15672: Queue,
		15671: Queue,
		61613: Queue,
		61614: Queue,
		1883:  Queue,
		8883:  Queue,
		15674: Queue,
		15675: Queue,

		// Kafka
		9092: Queue,

		// ActiveMQ
		61616: Queue,
		5672:  Queue,
		61617: Queue,

		// NATS
		4222: Queue,

		// IBM MQ
		1414: Queue,

		// Web
		80:  FrontEnd,
		443: FrontEnd,
	}

	// for now, this is unpopulated, but
	// as we find common service names that are listening on a
	// commonly used port, we can add them here
	nameMap = map[string]ServiceType{}
)

// Detect returns the ServiceType from the provided process information.
func Detect(name string, ports []int) ServiceType {
	// start with ports
	for _, v := range ports {
		if st, ok := portMap[v]; ok {
			return st
		}

	}
	// next check name
	if st, ok := nameMap[name]; ok {
		return st
	}

	// anything else is a webservice
	return WebService
}
