// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package listeners

const (
	dbmPostgresADIdentifier = "_dbm_postgres"
	dbmMySQLADIdentifier    = "_dbm_mysql"
	// For reasons passing understanding, the engine name is postgres for
	// instances but aurora-postgresql for clusters
	postgresqlEngine = "postgres"
	mysqlEngine      = "mysql"
)

const (
	dbmPostgresAuroraADIdentifier = "_dbm_postgres_aurora"
	dbmMySQLAuroaADIdentifier     = "_dbm_mysql_aurora"
	auroraPostgresqlEngine        = "aurora-postgresql"
	auroraMysqlEngine             = "aurora-mysql"
)

var engineToIntegrationType = map[string]string{
	postgresqlEngine:       "postgres",
	mysqlEngine:            "mysql",
	auroraPostgresqlEngine: "postgres",
	auroraMysqlEngine:      "mysql",
}

var engineToRdsADIdentifier = map[string]string{
	postgresqlEngine:       dbmPostgresADIdentifier,
	mysqlEngine:            dbmMySQLADIdentifier,
	auroraPostgresqlEngine: dbmPostgresADIdentifier,
	auroraMysqlEngine:      dbmMySQLADIdentifier,
}

var engineToAuroraADIdentifier = map[string]string{
	auroraPostgresqlEngine: dbmPostgresAuroraADIdentifier,
	auroraMysqlEngine:      dbmMySQLAuroaADIdentifier,
}

func findDeletedServices(currServices map[string]Service, discoveredServices map[string]struct{}) []string {
	deletedServices := make([]string, 0)
	for svc := range currServices {
		if _, exists := discoveredServices[svc]; !exists {
			deletedServices = append(deletedServices, svc)
		}
	}

	return deletedServices
}
