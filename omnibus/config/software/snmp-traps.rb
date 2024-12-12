name "snmp-traps"
default_version "0.4.0"

source :url => "https://s3.amazonaws.com/dd-agent-omnibus/snmp_traps_db/dd_traps_db-#{version}.json.gz",
       :sha256 => "04fb9d43754c2656edf35f08fbad11ba8dc20d52654962933f3dd8f4d463b42c",
       :target_filename => "dd_traps_db.json.gz"


build do
  # The dir for confs
  if osx_target?
    traps_db_dir = "#{install_dir}/etc/conf.d/snmp.d/traps_db"
  else
    traps_db_dir = "#{install_dir}/etc/datadog-agent/conf.d/snmp.d/traps_db"
  end
  mkdir traps_db_dir
  copy "dd_traps_db.json.gz", "#{traps_db_dir}/dd_traps_db.json.gz"
end
