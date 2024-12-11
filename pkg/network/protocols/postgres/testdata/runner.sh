#!/usr/bin/env bash

# Log the commands and their output for better debugging.
set -x

# Create a directory to store the configuration files
mkdir -p /postgres-test

# Define the paths to the configuration files
PG_HBA_CONF="/postgres-test/pg_hba.conf"
POSTGRES_CONF="/postgres-test/postgres.conf"

# If the ENCRYPTION_MODE is set to on, then we force TLS traffic.
# If the ENCRYPTION_MODE is set to off, then we force non-TLS traffic.
# See more details in the documentation: https://www.postgresql.org/docs/15/auth-pg-hba-conf.html
if [ "$ENCRYPTION_MODE" == "on" ]; then
  cat > "$PG_HBA_CONF" <<EOL
local   all             all                                     trust
hostssl  all  all  0.0.0.0/0  md5
EOL
else
  cat > "$PG_HBA_CONF" <<EOL
local   all             all                                     trust
hostnossl  all  all  0.0.0.0/0  md5
EOL
fi

# Write the configuration to postgres.conf
# We modify the listen_addresses to listen on all interfaces (appears in the original configuration) and
# we set the hba_file to the path of the pg_hba.conf file.
cat > "$POSTGRES_CONF" <<EOL
hba_file = '$PG_HBA_CONF'
listen_addresses = '*'
EOL

# Copying the server key and certificate to the test directory, as we change the permissions of the files (required for postgres to run)
cp /v/server.* /postgres-test/
chown -R postgres:postgres /postgres-test
chmod 755 /postgres-test
chmod 600 /postgres-test/server.*
chmod 666 /postgres-test/postgres.conf

# Call the original entrypoint script
/usr/local/bin/docker-entrypoint.sh "$@"
