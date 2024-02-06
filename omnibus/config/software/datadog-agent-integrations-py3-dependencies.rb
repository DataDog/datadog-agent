name 'datadog-agent-integrations-py3-dependencies'

dependency 'pip3'
dependency 'setuptools3'

dependency 'confluent-kafka-python'

if arm_target?
  # same with libffi to build the cffi wheel
  dependency 'libffi'
  # same with libxml2 and libxslt to build the lxml wheel
  dependency 'libxml2'
  dependency 'libxslt'
end

if linux_target?
  dependency 'unixodbc'
  dependency 'freetds'  # needed for SQL Server integration
  unless heroku_target?
    dependency 'msodbcsql18' # needed for SQL Server integration
  end
  # add nfsiostat script
  dependency 'nfsiostat'
  # add libkrb5 for all integrations supporting kerberos auth with `requests-kerberos`
  dependency 'libkrb5'
  # needed for glusterfs
  dependency 'gstatus'
end
