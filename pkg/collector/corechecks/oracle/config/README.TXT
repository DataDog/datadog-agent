Database
--------

Connect as sysdba to the monitored database and execute the following sql script in the monitored database:

https://github.com/DataDog/datadog-agent/blob/nenad.noveljic/dbm-oracle-beta-7.43.x/pkg/collector/corechecks/oracle/sql/setup.sql

The script will ask for password and create the monitoring account. If the account already exists, the CREATE USER command will fail, but the other commands will execute successfully.

Agent
-----

On the server where the agent is running, install the agent version dbm-oracle-beta-0.17-1.

Create new configuration directory for oracle-dbm:

CONF_D=/etc/datadog-agent/conf.d
mkdir $CONF_D/oracle-dbm.d

For every configuration file in oracle.d create a symbolic link in oracle-dbm.d:

cd $CONF_D/oracle-dbm.d
ln -s ../oracle.d/conf.yaml 

Add the parameter `dbm: true` to the monitored database instance in the conf.yaml file.
