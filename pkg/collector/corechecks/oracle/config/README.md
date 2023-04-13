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

https://github.com/DataDog/datadog-agent/blob/nenad.noveljic/dbm-oracle-beta-7.43.x/pkg/collector/corechecks/oracle/sql/setup.sql

The script will ask for password and create the monitoring account. If the account already exists, the CREATE USER command will fail, but the other commands will execute successfully.

Agent
-----
On the server where the agent is running, install the agent version dbm-oracle-beta-0.18-1.

Create the symbolic link for the oracle-dbm:

CONF_D=/etc/datadog-agent/conf.d
cd $CONF_D
ln -s oracle.d oracle-dbm.d

Add the parameter `dbm: true` to the monitored database instance in the conf.yaml file.
