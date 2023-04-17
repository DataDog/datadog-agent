Currently supported configurations
----------------------------------
- multi-tenant (no legacy architecture)
- single node instance (no RAC and Exadata)
- requires SYSDBA in CDB for setup (no RDS)
- tested with Oracle releases: 19c and 21c

Currently implemented features
------------------------------
- Activity sample profiling

Database
--------
Connect as sysdba to the monitored CDB database and execute the following sql script in the monitored database:

https://github.com/DataDog/datadog-agent/blob/main/pkg/collector/corechecks/oracle-dbm/sql/setup.sql

The script will ask for password and create the monitoring account. If the account already exists, the CREATE USER command will fail, but the other commands will execute successfully.

Agent
-----
Add the parameter `dbm: true` to the monitored database instance in the `conf.yaml` file in `oracle-dbm.d`.
