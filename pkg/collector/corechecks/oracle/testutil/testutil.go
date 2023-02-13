// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"fmt"
	"time"

	_ "github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	"github.com/ory/dockertest/docker"
	_ "github.com/sijms/go-ora/v2"
	"github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/testutil/dockerpool"
)

// CreateOraclePool creates an Oracle docker pool used for testing.
// TODO: Make configuratable, using hardcoded values for now...
func CreateOraclePool(dbName string, port int, attemptWait uint) (*sqlx.DB, dockerpool.TeardownFunc) {
	portStr := fmt.Sprintf("%d/tcp", port)

	pool, resource, teardown := dockerpool.CreatePool(&dockerpool.PoolConfig{
		//Repository: "gvenzl/oracle-xe",
		Repository: "container-registry.oracle.com/database/express",
		ImageTag:   "latest",
		DockerHostConfig: &docker.HostConfig{
			AutoRemove:    false,
			RestartPolicy: docker.RestartPolicy{Name: "no"},
		},
	}, dockerpool.OptionEnvs("ORACLE_PASSWORD=password"), dockerpool.OptionalExposedPorts(portStr))

	time.Sleep(1000 * time.Second)

	databaseUrl := fmt.Sprintf("%s/%s@localhost:%s/%s", "system", "password", resource.GetPort(portStr), dbName)

	// Hard kill the container after defined time
	if err := resource.Expire(180); err != nil {
		logrus.Fatalf("Failed to set resource expiration: %s", err)
	}

	var db *sqlx.DB

	// Exponential backoff-retry, our app in the container might not be ready to accept connections yet
	pool.MaxWait = time.Duration(attemptWait) * time.Second
	if err := pool.Retry(func() error {
		//oracleDB, err := sqlx.Open("godror", databaseUrl)
		oracleDB, err := sqlx.Open("oracle", databaseUrl)
		if err != nil {
			return err
		}
		db = oracleDB
		return oracleDB.Ping()
	}); err != nil {
		logrus.Fatalf("Failed to connect to docker after %d seconds | err=[%s]", attemptWait, err)
	}

	// TODO: Setup migrations to allow setup scripts

	return db, teardown
}
