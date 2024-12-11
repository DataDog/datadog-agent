// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mongo provides a simple wrapper around 3rd party mongo client.
package mongo

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

const (
	defaultConnectionTimeout = time.Second * 10
)

// Options is a struct to hold the options for the mongo client
type Options struct {
	ClientDialer     *net.Dialer
	ServerAddress    string
	Username         string
	Password         string
	ConnectionTimout time.Duration
}

// Client is a wrapper around the mongo client
type Client struct {
	C *mongo.Client
}

// NewClient creates a new mongo client
func NewClient(opts Options) (*Client, error) {
	clientOptions := options.Client().ApplyURI(fmt.Sprintf("mongodb://%s", opts.ServerAddress))
	if opts.Username == "" {
		opts.Username = User
	}
	if opts.Password == "" {
		opts.Password = Pass
	}
	creds := options.Credential{
		Username:   opts.Username,
		Password:   opts.Password,
		AuthSource: "admin",
	}
	clientOptions.SetAuth(creds)
	clientOptions.SetDirect(true)

	if opts.ConnectionTimout == 0 {
		opts.ConnectionTimout = defaultConnectionTimeout
	}

	if opts.ClientDialer != nil {
		clientOptions.SetDialer(opts.ClientDialer)
	}

	timedCtx, cancel := context.WithTimeout(context.Background(), opts.ConnectionTimout)
	defer cancel()
	client, err := mongo.Connect(timedCtx, clientOptions)
	if err != nil {
		return nil, err
	}

	timedCtx, cancel = context.WithTimeout(context.Background(), opts.ConnectionTimout)
	defer cancel()
	if err := client.Ping(timedCtx, nil); err != nil {
		return nil, err
	}

	return &Client{
		C: client,
	}, nil
}

var (
	defaultDBs = common.StringSet{
		"admin":  {},
		"config": {},
		"local":  {},
	}
)

// DeleteDatabases deletes all databases except the default ones
func (c *Client) DeleteDatabases() error {
	dbs, err := c.C.ListDatabases(context.Background(), bson.M{})
	if err != nil {
		return err
	}

	for _, db := range dbs.Databases {
		if _, isDefault := defaultDBs[db.Name]; isDefault {
			continue
		}
		if err := c.C.Database(db.Name).Drop(context.Background()); err != nil {
			return err
		}
	}
	return nil
}

// Stop stops the mongo client
func (c *Client) Stop() error {
	return c.C.Disconnect(context.Background())
}
