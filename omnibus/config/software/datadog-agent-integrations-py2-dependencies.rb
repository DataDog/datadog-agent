name 'datadog-agent-integrations-py2-dependencies'

dependency 'pip2'

if arm_target?
  # same with libffi to build the cffi wheel
  dependency 'libffi'
  # same with libxml2 and libxslt to build the lxml wheel
  dependency 'libxml2'
  dependency 'libxslt'
end

if osx_target?
  dependency 'unixodbc'
  # SDS library
  dependency 'sds'
end

if linux_target?
  # add nfsiostat script
  dependency 'unixodbc'
  dependency 'nfsiostat'
  # add libkrb5 for all integrations supporting kerberos auth with `requests-kerberos`
  dependency 'libkrb5'
  # needed for glusterfs
  dependency 'gstatus'
  # SDS library
  dependency 'sds'
end

if linux_target?
  # We need to use cython<3.0.0 to build pyyaml for py2
  dependency 'pyyaml-py2'
  dependency 'kubernetes-py2'
end
