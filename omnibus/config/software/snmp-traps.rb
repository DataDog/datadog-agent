name "snmp-traps"
default_version "0.2.1"

source :url => "https://s3.amazonaws.com/dd-agent-omnibus/snmp_traps_db/dd_traps_db-#{version}.json.gz",
       :sha256 => "e79274eddd119b59f55ee57106c7cced91facba24db132d4b37194e16defb7f1",
       :target_filename => "dd_traps_db.json.gz"


build do
  # The dir for confs
  if osx?
    traps_db_dir = "#{install_dir}/etc/conf.d/snmp.d/traps_db"
  else
    traps_db_dir = "#{install_dir}/etc/datadog-agent/conf.d/snmp.d/traps_db"
  end
  mkdir traps_db_dir
  copy "dd_traps_db.json.gz", "#{traps_db_dir}/dd_traps_db.json.gz"
end
