name 'datadog-agent-integrations-py3-dependencies'

dependency 'pip3'
dependency 'setuptools3'

dependency 'snowflake-connector-python-py3'
dependency 'confluent-kafka-python'

if arm?
  # same with libffi to build the cffi wheel
  dependency 'libffi'
  # same with libxml2 and libxslt to build the lxml wheel
  dependency 'libxml2'
  dependency 'libxslt'
end

if osx?
  dependency 'postgresql'
  dependency 'unixodbc'
end

if linux?
  # * Psycopg2 doesn't come with pre-built wheel on the arm architecture.
  #   to compile from source, it requires the `pg_config` executable present on the $PATH
  # * We also need it to build psycopg[c] Python dependency
  # * Note: because having unixodbc already built breaks postgresql build,
  #   we made unixodbc depend on postgresql to ensure proper build order.
  #   If we're ever removing/changing one of these dependencies, we need to
  #   take this into account.
  dependency 'postgresql'
  # add nfsiostat script
  dependency 'unixodbc'
  dependency 'freetds'  # needed for SQL Server integration
  dependency 'nfsiostat'
  # add libkrb5 for all integrations supporting kerberos auth with `requests-kerberos`
  dependency 'libkrb5'
  # needed for glusterfs
  dependency 'gstatus'
end

if redhat? && !arm?
  dependency 'pydantic-core-py3'
end

if linux?
  # We need to use cython<3.0.0 to build oracledb
  dependency 'oracledb-py3'
end
