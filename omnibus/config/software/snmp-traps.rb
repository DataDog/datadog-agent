name "snmp-traps"
default_version "0.3.0"

source :url => "https://s3.amazonaws.com/dd-agent-omnibus/snmp_traps_db/dd_traps_db-#{version}.json.gz",
       :sha256 => "d2fc278b31e36b23a3f64f87a0c46b857bee4e31313b6eaef90af42d9ab3cf93",
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
