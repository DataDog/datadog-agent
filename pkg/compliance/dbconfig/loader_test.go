// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package dbconfig

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/assert"
)

func TestDBConfLoader(t *testing.T) {
	{
		proc, err := process.NewProcess(int32(os.Getpid()))
		assert.NoError(t, err)
		resourceType, res, ok := LoadConfiguration(context.Background(), "/", proc)
		assert.False(t, ok)
		assert.Empty(t, resourceType)
		assert.Nil(t, res)
	}

	{
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		proc, stop := launchFakeProcess(ctx, t, "postgres")
		defer stop()
		resourceType, res, ok := LoadConfiguration(context.Background(), "/", proc)
		assert.True(t, ok)
		assert.NotNil(t, res)
		assert.Equal(t, postgresqlResourceType, resourceType)
		assert.NotNil(t, res)
		assert.NotNil(t, res.ConfigData)
		assert.NotEmpty(t, res.ProcessName)
		assert.NotEmpty(t, res.ProcessUser)
		assert.Empty(t, res.ConfigFilePath)
		assert.Equal(t, "<none>", res.ConfigFileUser)
		assert.Equal(t, "<none>", res.ConfigFileGroup)
		assert.Zero(t, res.ConfigFileMode)
	}

	{
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "postgres.conf")
		err := os.WriteFile(configPath, []byte(pgConfigCommon), 0600)
		assert.NoError(t, err)

		proc, stop := launchFakeProcess(ctx, t, "postgres", "--config-file", "postgres.conf")
		defer stop()

		resourceType, ok := GetProcResourceType(proc)
		assert.True(t, ok)
		assert.Equal(t, postgresqlResourceType, resourceType)

		resourceType, res, ok := LoadConfiguration(context.Background(), tempDir, proc)
		assert.True(t, ok)
		assert.NotNil(t, res)
		assert.Equal(t, postgresqlResourceType, resourceType)
		assert.NotNil(t, res.ConfigData)
		assert.NotEmpty(t, res.ProcessName)
		assert.NotEmpty(t, res.ProcessUser)
		assert.NotEmpty(t, res.ConfigFilePath)
		assert.NotEmpty(t, res.ConfigFileUser)
		assert.NotEmpty(t, res.ConfigFileGroup)
		assert.Equal(t, uint32(0600), res.ConfigFileMode)
	}
}

func launchFakeProcess(ctx context.Context, t *testing.T, procname string, args ...string) (*process.Process, func()) {
	binPath := filepath.Join(t.TempDir(), procname)
	if err := os.WriteFile(binPath, []byte("#!/bin/bash\nsleep 10"), 0700); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("could not start fake process %q: %v", procname, err)
	}

	proc, err := process.NewProcess(int32(cmd.Process.Pid))
	assert.NoError(t, err)

	return proc, func() {
		cmd.Process.Kill()
		cmd.Process.Wait()
	}
}

func TestPGConfParsingIncludeRel(t *testing.T) {
	hostroot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostroot, "/etc/postgresql"), 0700); err != nil {
		t.Fatal(err)
	}

	const configPathCommon = "/etc/postgresql/postgresql-common.conf"
	const configPath = "/etc/postgresql/postgresql.conf"
	const config = `
include 'postgresql-common.conf'

foo = 'bar'
foo = yes
dynamic_shared_memory_type = 'overridden'`
	if err := os.WriteFile(filepath.Join(hostroot, configPath), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostroot, configPathCommon), []byte(pgConfigCommon), 0600); err != nil {
		t.Fatal(err)
	}

	proc, stop := launchFakeProcess(context.Background(), t, "postgres", "--config", configPath)
	defer stop()
	conf, ok := LoadPostgreSQLConfig(context.Background(), hostroot, proc)
	assert.Equal(t, true, ok)
	configData := conf.ConfigData.(map[string]interface{})
	assert.Equal(t, "/etc/postgresql/postgresql.conf", conf.ConfigFilePath)
	assert.Equal(t, "on", configData["foo"])
	assert.Equal(t, "overridden", configData["dynamic_shared_memory_type"])
}

func TestPGConfParsingIncludeAbs(t *testing.T) {
	hostroot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostroot, "/etc/postgresql"), 0700); err != nil {
		t.Fatal(err)
	}

	const configPathCommon = "/etc/postgresql/postgresql-common.conf"
	const configPath = "/etc/postgresql/postgresql.conf"
	const config = `
include '/etc/postgresql/postgresql-common.conf'

foo = 'bar''\b'
dynamic_shared_memory_type = 'overridden'
`
	if err := os.WriteFile(filepath.Join(hostroot, configPath), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostroot, configPathCommon), []byte(pgConfigCommon), 0600); err != nil {
		t.Fatal(err)
	}

	proc, stop := launchFakeProcess(context.Background(), t, "postgres", "--config", configPath)
	defer stop()
	c, ok := LoadPostgreSQLConfig(context.Background(), hostroot, proc)
	assert.Equal(t, true, ok)
	configData := c.ConfigData.(map[string]interface{})
	assert.Equal(t, "bar'\b", configData["foo"])
	assert.Equal(t, "overridden", configData["dynamic_shared_memory_type"])
}

func TestPGConfParsingCustom(t *testing.T) {
	hostroot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostroot, "/etc/postgresql"), 0700); err != nil {
		t.Fatal(err)
	}

	const configPathCommon = "/etc/postgresql/postgresql-common.conf"
	const configPath = "/etc/postgresql/postgresql.conf"
	const config = pgConfigCustom
	if err := os.WriteFile(filepath.Join(hostroot, configPath), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostroot, configPathCommon), []byte(pgConfigCommon), 0600); err != nil {
		t.Fatal(err)
	}

	proc, stop := launchFakeProcess(context.Background(), t, "postgres", "--config", configPath)
	defer stop()
	c, ok := LoadPostgreSQLConfig(context.Background(), hostroot, proc)
	assert.True(t, ok)
	configData := c.ConfigData.(map[string]interface{})
	assert.Equal(t, `envdir "/run/etc/wal-g.d/env" wal-g wal-push "%p"`, configData["archive_command"])
	assert.Equal(t, `on`, configData["archive_mode"])
	assert.Equal(t, `1800s`, configData["archive_timeout"])
	assert.Equal(t, `on`, configData["autovacuum"])
	assert.Equal(t, `0.05`, configData["autovacuum_analyze_scale_factor"])
	assert.Equal(t, `5`, configData["autovacuum_max_workers"])
	assert.Equal(t, `15s`, configData["autovacuum_naptime"])
	assert.Equal(t, `2ms`, configData["autovacuum_vacuum_cost_delay"])
	assert.Equal(t, `1800`, configData["autovacuum_vacuum_cost_limit"])
	assert.Equal(t, `0.1`, configData["autovacuum_vacuum_scale_factor"])
	assert.Equal(t, `30min`, configData["checkpoint_timeout"])
	assert.Equal(t, `resources-canary-k8s-01`, configData["cluster_name"])
	assert.Equal(t, `500`, configData["default_statistics_target"])
	assert.Equal(t, `119GB`, configData["effective_cache_size"])
	assert.Equal(t, `200`, configData["effective_io_concurrency"])
	assert.Equal(t, `on`, configData["fsync"])
	assert.Equal(t, `on`, configData["hot_standby"])
	assert.Equal(t, `on`, configData["hot_standby_feedback"])
	assert.Equal(t, `15min`, configData["idle_in_transaction_session_timeout"])
	assert.Equal(t, `24h`, configData["idle_session_timeout"])
	assert.Equal(t, `*`, configData["listen_addresses"])
	assert.Equal(t, `5s`, configData["log_autovacuum_min_duration"])
	assert.Equal(t, `on`, configData["log_checkpoints"])
	assert.Equal(t, `on`, configData["log_connections"])
	assert.Equal(t, `../pg_log`, configData["log_directory"])
	assert.Equal(t, `off`, configData["log_disconnections"])
	assert.Equal(t, `0644`, configData["log_file_mode"])
	assert.Equal(t, `%m [%p] %q%a %u@%d %r `, configData["log_line_prefix"])
	assert.Equal(t, `on`, configData["log_lock_waits"])
	assert.Equal(t, `500ms`, configData["log_min_duration_sample"])
	assert.Equal(t, `1d`, configData["log_rotation_age"])
	assert.Equal(t, `512MB`, configData["log_rotation_size"])
	assert.Equal(t, `ddl`, configData["log_statement"])
	assert.Equal(t, `0.05`, configData["log_statement_sample_rate"])
	assert.Equal(t, `on`, configData["logging_collector"])
	assert.Equal(t, `2048MB`, configData["maintenance_work_mem"])
	assert.Equal(t, `4000`, configData["max_connections"])
	assert.Equal(t, `64`, configData["max_locks_per_transaction"])
	assert.Equal(t, `14`, configData["max_parallel_workers"])
	assert.Equal(t, `0`, configData["max_prepared_transactions"])
	assert.Equal(t, `999`, configData["max_replication_slots"])
	assert.Equal(t, `999`, configData["max_wal_senders"])
	assert.Equal(t, `25600MB`, configData["max_wal_size"])
	assert.Equal(t, `14`, configData["max_worker_processes"])
	assert.Equal(t, `scram-sha-256`, configData["password_encryption"])
	assert.Equal(t, `10000`, configData["pg_stat_statements.max"])
	assert.Equal(t, `all`, configData["pg_stat_statements.track"])
	assert.Equal(t, `off`, configData["pg_stat_statements.track_utility"])
	assert.Equal(t, `5432`, configData["port"])
	assert.Equal(t, `1.1`, configData["random_page_cost"])
	assert.Equal(t, `1.0`, configData["seq_page_cost"])
	assert.Equal(t, `6912MB`, configData["shared_buffers"])
	assert.Equal(t, `pg_stat_statements,uuid-ossp,hstore,pg_stat_kcache`, configData["shared_preload_libraries"])
	assert.Equal(t, `on`, configData["ssl"])
	assert.Equal(t, `/run/certs/ca-crt.pem`, configData["ssl_ca_file"])
	assert.Equal(t, `/run/certs/server.crt`, configData["ssl_cert_file"])
	assert.Equal(t, `HIGH:!RC4:!MD5:!3DES:!aNULL`, configData["ssl_ciphers"])
	assert.Equal(t, `/run/certs/server.key`, configData["ssl_key_file"])
	assert.Equal(t, `TLSv1.2`, configData["ssl_min_protocol_version"])
	assert.Equal(t, `30s`, configData["statement_timeout"])
	assert.Equal(t, `900`, configData["tcp_keepalives_idle"])
	assert.Equal(t, `100`, configData["tcp_keepalives_interval"])
	assert.Equal(t, `on`, configData["track_activities"])
	assert.Equal(t, `4096`, configData["track_activity_query_size"])
	assert.Equal(t, `off`, configData["track_commit_timestamp"])
	assert.Equal(t, `all`, configData["track_functions"])
	assert.Equal(t, `on`, configData["track_io_timing"])
	assert.Equal(t, `16MB`, configData["wal_buffers"])
	assert.Equal(t, `on`, configData["wal_compression"])
	assert.Equal(t, `128MB`, configData["wal_keep_size"])
	assert.Equal(t, `logical`, configData["wal_level"])
	assert.Equal(t, `on`, configData["wal_log_hints"])
	assert.Equal(t, `/home/postgres/pgdata/pgroot/data/pg_hba.conf`, configData["hba_file"])
	assert.Equal(t, `/home/postgres/pgdata/pgroot/data/pg_ident.conf`, configData["ident_file"])
}

func FuzzPGConfTokenizer(f *testing.F) {
	f.Add(pgConfigCommon)
	f.Fuzz(func(t *testing.T, a string) {
		tok := pgConfLexer{buf: []byte(a)}
		tok.next()
	})
}

func TestCassandraConfParsing(t *testing.T) {
	hostroot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostroot, "/etc/cassandra"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostroot, "/etc/cassandra/cassandra.yaml"), []byte(cassandraConfigSample), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostroot, "/etc/cassandra/logback.xml"), []byte(cassandraLogbackSample), 0600); err != nil {
		t.Fatal(err)
	}
	proc, stop := launchFakeProcess(context.Background(), t, "java")
	defer stop()
	c, ok := LoadCassandraConfig(context.Background(), hostroot, proc)
	assert.True(t, ok)
	configData := c.ConfigData.(*cassandraDBConfig)
	assert.Equal(t, uint32(0600), c.ConfigFileMode)
	assert.Equal(t, "/etc/cassandra/cassandra.yaml", c.ConfigFilePath)
	assert.NotEmpty(t, c.ConfigFileUser)
	assert.NotNil(t, configData)
	assert.Equal(t, "/etc/cassandra/logback.xml", configData.LogbackFilePath)
	assert.Equal(t, cassandraLogbackSample, configData.LogbackFileContent)
}

func TestMongoDBConfParsing(t *testing.T) {
	hostroot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostroot, "/etc"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostroot, "/etc/mongod.conf"), []byte(mongodConfigSample), 0600); err != nil {
		t.Fatal(err)
	}
	proc, stop := launchFakeProcess(context.Background(), t, "mongod")
	defer stop()
	c, ok := LoadMongoDBConfig(context.Background(), hostroot, proc)
	assert.True(t, ok)
	configData := c.ConfigData.(*mongoDBConfig)
	assert.Equal(t, uint32(0600), c.ConfigFileMode)
	assert.Equal(t, "/etc/mongod.conf", c.ConfigFilePath)
	assert.NotEmpty(t, c.ConfigFileUser)
	assert.NotNil(t, configData)
	assert.Equal(t, "file", *configData.SystemLog.Destination)
	assert.Equal(t, true, *configData.SystemLog.LogAppend)
	assert.Equal(t, "/var/log/mongodb/mongod.log", *configData.SystemLog.Path)
}

const pgConfigCommon = `
# -----------------------------
# PostgreSQL configuration file
# -----------------------------
#
# This file consists of lines of the form:
#
#   name = value
#
# (The "=" is optional.)  Whitespace may be used.  Comments are introduced with
# "#" anywhere on a line.  The complete list of parameter names and allowed
# values can be found in the PostgreSQL documentation.
#
# The commented-out settings shown in this file represent the default values.
# Re-commenting a setting is NOT sufficient to revert it to the default value;
# you need to reload the server.
#
# This file is read on server startup and when the server receives a SIGHUP
# signal.  If you edit the file on a running system, you have to SIGHUP the
# server for the changes to take effect, run "pg_ctl reload", or execute
# "SELECT pg_reload_conf()".  Some parameters, which are marked below,
# require a server shutdown and restart to take effect.
#
# Any parameter can also be given as a command-line option to the server, e.g.,
# "postgres -c log_connections=on".  Some parameters can be changed at run time
# with the "SET" SQL command.
#
# Memory units:  B  = bytes            Time units:  us  = microseconds
#                kB = kilobytes                     ms  = milliseconds
#                MB = megabytes                     s   = seconds
#                GB = gigabytes                     min = minutes
#                TB = terabytes                     h   = hours
#                                                   d   = days

#------------------------------------------------------------------------------
# FILE LOCATIONS
#------------------------------------------------------------------------------

# The default values of these variables are driven from the -D command-line
# option or PGDATA environment variable, represented here as ConfigDir.

#data_directory = 'ConfigDir'		# use data in another directory
					# (change requires restart)
#hba_file = 'ConfigDir/pg_hba.conf'	# host-based authentication file
					# (change requires restart)
#ident_file = 'ConfigDir/pg_ident.conf'	# ident configuration file
					# (change requires restart)

# If external_pid_file is not explicitly set, no extra PID file is written.
#external_pid_file = ''			# write an extra PID file
					# (change requires restart)

#------------------------------------------------------------------------------
# CONNECTIONS AND AUTHENTICATION
#------------------------------------------------------------------------------

# - Connection Settings -

#listen_addresses = 'localhost'		# what IP address(es) to listen on;
					# comma-separated list of addresses;
					# defaults to 'localhost'; use '*' for all
					# (change requires restart)
#port = 5432				# (change requires restart)
#max_connections = 100			# (change requires restart)
#reserved_connections = 0		# (change requires restart)
#superuser_reserved_connections = 3	# (change requires restart)
#unix_socket_directories = '/tmp'	# comma-separated list of directories
					# (change requires restart)
#unix_socket_group = ''			# (change requires restart)
#unix_socket_permissions = 0777		# begin with 0 to use octal notation
					# (change requires restart)
#bonjour = off				# advertise server via Bonjour
					# (change requires restart)
#bonjour_name = ''			# defaults to the computer name
					# (change requires restart)

# - TCP settings -
# see "man tcp" for details

#tcp_keepalives_idle = 0		# TCP_KEEPIDLE, in seconds;
					# 0 selects the system default
#tcp_keepalives_interval = 0		# TCP_KEEPINTVL, in seconds;
					# 0 selects the system default
#tcp_keepalives_count = 0		# TCP_KEEPCNT;
					# 0 selects the system default
#tcp_user_timeout = 0			# TCP_USER_TIMEOUT, in milliseconds;
					# 0 selects the system default

#client_connection_check_interval = 0	# time between checks for client
					# disconnection while running queries;
					# 0 for never

# - Authentication -

authentication_timeout = 1min		# 1s-600s
password_encryption = scram-sha-256	# scram-sha-256 or md5
scram_iterations = 4096

# GSSAPI using Kerberos
krb_server_keyfile = 'FILE:${sysconfdir}/krb5.keytab'
krb_caseins_users = off
gss_accept_delegation = off

# - SSL -

#ssl = off
#ssl_ca_file = ''
#ssl_cert_file = 'server.crt'
#ssl_crl_file = ''
#ssl_crl_dir = ''
#ssl_key_file = 'server.key'
#ssl_ciphers = 'HIGH:MEDIUM:+3DES:!aNULL' # allowed SSL ciphers
#ssl_prefer_server_ciphers = on
#ssl_ecdh_curve = 'prime256v1'
#ssl_min_protocol_version = 'TLSv1.2'
#ssl_max_protocol_version = ''
#ssl_dh_params_file = ''
#ssl_passphrase_command = ''
#ssl_passphrase_command_supports_reload = off

#------------------------------------------------------------------------------
# RESOURCE USAGE (except WAL)
#------------------------------------------------------------------------------

# - Memory -

shared_buffers = 128MB			# min 128kB
					# (change requires restart)
huge_pages = try			# on, off, or try
					# (change requires restart)
huge_page_size = 0			# zero for system default
					# (change requires restart)
#temp_buffers = 8MB			# min 800kB
#max_prepared_transactions = 0		# zero disables the feature
					# (change requires restart)
# Caution: it is not advisable to set max_prepared_transactions nonzero unless
# you actively intend to use prepared transactions.
work_mem = 4MB				# min 64kB
hash_mem_multiplier = 2.0		# 1-1000.0 multiplier on hash table work_mem
maintenance_work_mem = 64MB		# min 1MB
autovacuum_work_mem = -1		# min 1MB, or -1 to use maintenance_work_mem
logical_decoding_work_mem = 64MB	# min 64kB
max_stack_depth = 2MB			# min 100kB
shared_memory_type = mmap		# the default is the first option
					# supported by the operating system:
					#   mmap
					#   sysv
					#   windows
					# (change requires restart)
dynamic_shared_memory_type = posix	# the default is usually the first option
					# supported by the operating system:
					#   posix
					#   sysv
					#   windows
					#   mmap
					# (change requires restart)
#min_dynamic_shared_memory = 0MB	# (change requires restart)
#vacuum_buffer_usage_limit = 256kB	# size of vacuum and analyze buffer access strategy ring;
					# 0 to disable vacuum buffer access strategy;
					# range 128kB to 16GB

# - Disk -

#temp_file_limit = -1			# limits per-process temp file space
					# in kilobytes, or -1 for no limit

# - Kernel Resources -

#max_files_per_process = 1000		# min 64
					# (change requires restart)

# - Cost-Based Vacuum Delay -

#vacuum_cost_delay = 0			# 0-100 milliseconds (0 disables)
#vacuum_cost_page_hit = 1		# 0-10000 credits
#vacuum_cost_page_miss = 2		# 0-10000 credits
#vacuum_cost_page_dirty = 20		# 0-10000 credits
#vacuum_cost_limit = 200		# 1-10000 credits

# - Background Writer -

#bgwriter_delay = 200ms			# 10-10000ms between rounds
#bgwriter_lru_maxpages = 100		# max buffers written/round, 0 disables
#bgwriter_lru_multiplier = 2.0		# 0-10.0 multiplier on buffers scanned/round
#bgwriter_flush_after = 0		# measured in pages, 0 disables

# - Asynchronous Behavior -

#backend_flush_after = 0		# measured in pages, 0 disables
#effective_io_concurrency = 1		# 1-1000; 0 disables prefetching
#maintenance_io_concurrency = 10	# 1-1000; 0 disables prefetching
#max_worker_processes = 8		# (change requires restart)
#max_parallel_workers_per_gather = 2	# taken from max_parallel_workers
#max_parallel_maintenance_workers = 2	# taken from max_parallel_workers
#max_parallel_workers = 8		# maximum number of max_worker_processes that
					# can be used in parallel operations
#parallel_leader_participation = on

#------------------------------------------------------------------------------
# WRITE-AHEAD LOG
#------------------------------------------------------------------------------

# - Settings -

#wal_level = replica			# minimal, replica, or logical
					# (change requires restart)
#fsync = on				# flush data to disk for crash safety
					# (turning this off can cause
					# unrecoverable data corruption)
#synchronous_commit = on		# synchronization level;
					# off, local, remote_write, remote_apply, or on
#wal_sync_method = fsync		# the default is the first option
					# supported by the operating system:
					#   open_datasync
					#   fdatasync (default on Linux and FreeBSD)
					#   fsync
					#   fsync_writethrough
					#   open_sync
#full_page_writes = on			# recover from partial page writes
#wal_log_hints = off			# also do full page writes of non-critical updates
					# (change requires restart)
#wal_compression = off			# enables compression of full-page writes;
					# off, pglz, lz4, zstd, or on
#wal_init_zero = on			# zero-fill new WAL files
#wal_recycle = on			# recycle WAL files
#wal_buffers = -1			# min 32kB, -1 sets based on shared_buffers
					# (change requires restart)
#wal_writer_delay = 200ms		# 1-10000 milliseconds
#wal_writer_flush_after = 1MB		# measured in pages, 0 disables
#wal_skip_threshold = 2MB

#commit_delay = 0			# range 0-100000, in microseconds
#commit_siblings = 5			# range 1-1000

# - Checkpoints -

#checkpoint_timeout = 5min		# range 30s-1d
#checkpoint_completion_target = 0.9	# checkpoint target duration, 0.0 - 1.0
#checkpoint_flush_after = 0		# measured in pages, 0 disables
#checkpoint_warning = 30s		# 0 disables
#max_wal_size = 1GB
#min_wal_size = 80MB

# - Prefetching during recovery -

#recovery_prefetch = try		# prefetch pages referenced in the WAL?
#wal_decode_buffer_size = 512kB		# lookahead window used for prefetching
					# (change requires restart)

# - Archiving -

#archive_mode = off		# enables archiving; off, on, or always
				# (change requires restart)
#archive_library = ''		# library to use to archive a WAL file
				# (empty string indicates archive_command should
				# be used)
#archive_command = ''		# command to use to archive a WAL file
				# placeholders: %p = path of file to archive
				#               %f = file name only
				# e.g. 'test ! -f /mnt/server/archivedir/%f && cp %p /mnt/server/archivedir/%f'
#archive_timeout = 0		# force a WAL file switch after this
				# number of seconds; 0 disables

# - Archive Recovery -

# These are only used in recovery mode.

#restore_command = ''		# command to use to restore an archived WAL file
				# placeholders: %p = path of file to restore
				#               %f = file name only
				# e.g. 'cp /mnt/server/archivedir/%f %p'
#archive_cleanup_command = ''	# command to execute at every restartpoint
#recovery_end_command = ''	# command to execute at completion of recovery

# - Recovery Target -

# Set these only when performing a targeted recovery.

#recovery_target = ''		# 'immediate' to end recovery as soon as a
                                # consistent state is reached
				# (change requires restart)
#recovery_target_name = ''	# the named restore point to which recovery will proceed
				# (change requires restart)
#recovery_target_time = ''	# the time stamp up to which recovery will proceed
				# (change requires restart)
#recovery_target_xid = ''	# the transaction ID up to which recovery will proceed
				# (change requires restart)
#recovery_target_lsn = ''	# the WAL LSN up to which recovery will proceed
				# (change requires restart)
#recovery_target_inclusive = on # Specifies whether to stop:
				# just after the specified recovery target (on)
				# just before the recovery target (off)
				# (change requires restart)
#recovery_target_timeline = 'latest'	# 'current', 'latest', or timeline ID
				# (change requires restart)
#recovery_target_action = 'pause'	# 'pause', 'promote', 'shutdown'
				# (change requires restart)

#------------------------------------------------------------------------------
# REPLICATION
#------------------------------------------------------------------------------

# - Sending Servers -

# Set these on the primary and on any standby that will send replication data.

#max_wal_senders = 10		# max number of walsender processes
				# (change requires restart)
#max_replication_slots = 10	# max number of replication slots
				# (change requires restart)
#wal_keep_size = 0		# in megabytes; 0 disables
#max_slot_wal_keep_size = -1	# in megabytes; -1 disables
#wal_sender_timeout = 60s	# in milliseconds; 0 disables
#track_commit_timestamp = off	# collect timestamp of transaction commit
				# (change requires restart)

# - Primary Server -

# These settings are ignored on a standby server.

#synchronous_standby_names = ''	# standby servers that provide sync rep
				# method to choose sync standbys, number of sync standbys,
				# and comma-separated list of application_name
				# from standby(s); '*' = all

# - Standby Servers -

# These settings are ignored on a primary server.

#primary_conninfo = ''			# connection string to sending server
#primary_slot_name = ''			# replication slot on sending server
#hot_standby = on			# "off" disallows queries during recovery
					# (change requires restart)
#max_standby_archive_delay = 30s	# max delay before canceling queries
					# when reading WAL from archive;
					# -1 allows indefinite delay
#max_standby_streaming_delay = 30s	# max delay before canceling queries
					# when reading streaming WAL;
					# -1 allows indefinite delay
#wal_receiver_create_temp_slot = off	# create temp slot if primary_slot_name
					# is not set
#wal_receiver_status_interval = 10s	# send replies at least this often
					# 0 disables
#hot_standby_feedback = off		# send info from standby to prevent
					# query conflicts
#wal_receiver_timeout = 60s		# time that receiver waits for
					# communication from primary
					# in milliseconds; 0 disables
#wal_retrieve_retry_interval = 5s	# time to wait before retrying to
					# retrieve WAL after a failed attempt
#recovery_min_apply_delay = 0		# minimum delay for applying changes during recovery

# - Subscribers -

# These settings are ignored on a publisher.

#max_logical_replication_workers = 4	# taken from max_worker_processes
					# (change requires restart)
#max_sync_workers_per_subscription = 2	# taken from max_logical_replication_workers
#max_parallel_apply_workers_per_subscription = 2	# taken from max_logical_replication_workers

#------------------------------------------------------------------------------
# QUERY TUNING
#------------------------------------------------------------------------------

# - Planner Method Configuration -

#enable_async_append = on
#enable_bitmapscan = on
#enable_gathermerge = on
#enable_hashagg = on
#enable_hashjoin = on
#enable_incremental_sort = on
#enable_indexscan = on
#enable_indexonlyscan = on
#enable_material = on
#enable_memoize = on
#enable_mergejoin = on
#enable_nestloop = on
#enable_parallel_append = on
#enable_parallel_hash = on
#enable_partition_pruning = on
#enable_partitionwise_join = off
#enable_partitionwise_aggregate = off
#enable_presorted_aggregate = on
#enable_seqscan = on
#enable_sort = on
#enable_tidscan = on

# - Planner Cost Constants -

#seq_page_cost = 1.0			# measured on an arbitrary scale
#random_page_cost = 4.0			# same scale as above
#cpu_tuple_cost = 0.01			# same scale as above
#cpu_index_tuple_cost = 0.005		# same scale as above
#cpu_operator_cost = 0.0025		# same scale as above
#parallel_setup_cost = 1000.0	# same scale as above
#parallel_tuple_cost = 0.1		# same scale as above
#min_parallel_table_scan_size = 8MB
#min_parallel_index_scan_size = 512kB
#effective_cache_size = 4GB

#jit_above_cost = 100000		# perform JIT compilation if available
					# and query more expensive than this;
					# -1 disables
#jit_inline_above_cost = 500000		# inline small functions if query is
					# more expensive than this; -1 disables
#jit_optimize_above_cost = 500000	# use expensive JIT optimizations if
					# query is more expensive than this;
					# -1 disables

# - Genetic Query Optimizer -

#geqo = on
#geqo_threshold = 12
#geqo_effort = 5			# range 1-10
#geqo_pool_size = 0			# selects default based on effort
#geqo_generations = 0			# selects default based on effort
#geqo_selection_bias = 2.0		# range 1.5-2.0
#geqo_seed = 0.0			# range 0.0-1.0

# - Other Planner Options -

#default_statistics_target = 100	# range 1-10000
#constraint_exclusion = partition	# on, off, or partition
#cursor_tuple_fraction = 0.1		# range 0.0-1.0
#from_collapse_limit = 8
#jit = on				# allow JIT compilation
#join_collapse_limit = 8		# 1 disables collapsing of explicit
					# JOIN clauses
#plan_cache_mode = auto			# auto, force_generic_plan or
					# force_custom_plan
#recursive_worktable_factor = 10.0	# range 0.001-1000000

#------------------------------------------------------------------------------
# REPORTING AND LOGGING
#------------------------------------------------------------------------------

# - Where to Log -

log_destination 'stderr'		# Valid values are combinations of
					# stderr, csvlog, jsonlog, syslog, and
					# eventlog, depending on platform.
					# csvlog and jsonlog require
					# logging_collector to be on.

# This is used when logging to stderr:
#logging_collector = off		# Enable capturing of stderr, jsonlog,
					# and csvlog into log files. Required
					# to be on for csvlogs and jsonlogs.
					# (change requires restart)

# These are only used if logging_collector is on:
log_directory = 'log'			# directory where log files are written,
					# can be absolute or relative to PGDATA
log_filename = 'postgresql-%Y-%m-%d_%H%M%S.log'	# log file name pattern,
					# can include strftime() escapes
log_file_mode = 0600			# creation mode for log files,
					# begin with 0 to use octal notation
log_rotation_age = 1d			# Automatic rotation of logfiles will
					# happen after that time.  0 disables.
log_rotation_size = 10MB		# Automatic rotation of logfiles will
					# happen after that much log output.
					# 0 disables.
log_truncate_on_rotation = off		# If on, an existing log file with the
					# same name as the new log file will be
					# truncated rather than appended to.
					# But such truncation only occurs on
					# time-driven rotation, not on restarts
					# or size-driven rotation.  Default is
					# off, meaning append to existing files
					# in all cases.

# These are relevant when logging to syslog:
#syslog_facility = 'LOCAL0'
#syslog_ident = 'postgres'
#syslog_sequence_numbers = on
#syslog_split_messages = on

# This is only relevant when logging to eventlog (Windows):
# (change requires restart)
#event_source = 'PostgreSQL'

# - When to Log -

log_min_messages = warning		# values in order of decreasing detail:
					#   debug5
					#   debug4
					#   debug3
					#   debug2
					#   debug1
					#   info
					#   notice
					#   warning
					#   error
					#   log
					#   fatal
					#   panic

log_min_error_statement = error	# values in order of decreasing detail:
					#   debug5
					#   debug4
					#   debug3
					#   debug2
					#   debug1
					#   info
					#   notice
					#   warning
					#   error
					#   log
					#   fatal
					#   panic (effectively off)

log_min_duration_statement = -1	# -1 is disabled, 0 logs all statements
					# and their durations, > 0 logs only
					# statements running at least this number
					# of milliseconds

log_min_duration_sample = -1		# -1 is disabled, 0 logs a sample of statements
					# and their durations, > 0 logs only a sample of
					# statements running at least this number
					# of milliseconds;
					# sample fraction is determined by log_statement_sample_rate

log_statement_sample_rate = 1.0	# fraction of logged statements exceeding
					# log_min_duration_sample to be logged;
					# 1.0 logs all such statements, 0.0 never logs

log_transaction_sample_rate = 0.0	# fraction of transactions whose statements
					# are logged regardless of their duration; 1.0 logs all
					# statements from all transactions, 0.0 never logs

log_startup_progress_interval = 10s	# Time between progress updates for
					# long-running startup operations.
					# 0 disables the feature, > 0 indicates
					# the interval in milliseconds.

# - What to Log -

#debug_print_parse = off
#debug_print_rewritten = off
#debug_print_plan = off
#debug_pretty_print = on
#log_autovacuum_min_duration = 10min	# log autovacuum activity;
					# -1 disables, 0 logs all actions and
					# their durations, > 0 logs only
					# actions running at least this number
					# of milliseconds.
#log_checkpoints = on
#log_connections = off
#log_disconnections = off
#log_duration = off
#log_error_verbosity = default		# terse, default, or verbose messages
#log_hostname = off
#log_line_prefix = '%m [%p] '		# special values:
					#   %a = application name
					#   %u = user name
					#   %d = database name
					#   %r = remote host and port
					#   %h = remote host
					#   %b = backend type
					#   %p = process ID
					#   %P = process ID of parallel group leader
					#   %t = timestamp without milliseconds
					#   %m = timestamp with milliseconds
					#   %n = timestamp with milliseconds (as a Unix epoch)
					#   %Q = query ID (0 if none or not computed)
					#   %i = command tag
					#   %e = SQL state
					#   %c = session ID
					#   %l = session line number
					#   %s = session start timestamp
					#   %v = virtual transaction ID
					#   %x = transaction ID (0 if none)
					#   %q = stop here in non-session
					#        processes
					#   %% = '%'
					# e.g. '<%u%%%d> '
#log_lock_waits = off			# log lock waits >= deadlock_timeout
#log_recovery_conflict_waits = off	# log standby recovery conflict waits
					# >= deadlock_timeout
#log_parameter_max_length = -1		# when logging statements, limit logged
					# bind-parameter values to N bytes;
					# -1 means print in full, 0 disables
#log_parameter_max_length_on_error = 0	# when logging an error, limit logged
					# bind-parameter values to N bytes;
					# -1 means print in full, 0 disables
#log_statement = 'none'			# none, ddl, mod, all
#log_replication_commands = off
#log_temp_files = -1			# log temporary files equal or larger
					# than the specified size in kilobytes;
					# -1 disables, 0 logs all temp files
#log_timezone = 'GMT'

# - Process Title -

#cluster_name = ''			# added to process titles if nonempty
					# (change requires restart)
#update_process_title = on

#------------------------------------------------------------------------------
# STATISTICS
#------------------------------------------------------------------------------

# - Cumulative Query and Index Statistics -

#track_activities = on
#track_activity_query_size = 1024	# (change requires restart)
#track_counts = on
#track_io_timing = off
#track_wal_io_timing = off
#track_functions = none			# none, pl, all
#stats_fetch_consistency = cache	# cache, none, snapshot

# - Monitoring -

#compute_query_id = auto
#log_statement_stats = off
#log_parser_stats = off
#log_planner_stats = off
#log_executor_stats = off

#------------------------------------------------------------------------------
# AUTOVACUUM
#------------------------------------------------------------------------------

#autovacuum = on			# Enable autovacuum subprocess?  'on'
					# requires track_counts to also be on.
#autovacuum_max_workers = 3		# max number of autovacuum subprocesses
					# (change requires restart)
#autovacuum_naptime = 1min		# time between autovacuum runs
#autovacuum_vacuum_threshold = 50	# min number of row updates before
					# vacuum
#autovacuum_vacuum_insert_threshold = 1000	# min number of row inserts
					# before vacuum; -1 disables insert
					# vacuums
#autovacuum_analyze_threshold = 50	# min number of row updates before
					# analyze
#autovacuum_vacuum_scale_factor = 0.2	# fraction of table size before vacuum
#autovacuum_vacuum_insert_scale_factor = 0.2	# fraction of inserts over table
					# size before insert vacuum
#autovacuum_analyze_scale_factor = 0.1	# fraction of table size before analyze
#autovacuum_freeze_max_age = 200000000	# maximum XID age before forced vacuum
					# (change requires restart)
#autovacuum_multixact_freeze_max_age = 400000000	# maximum multixact age
					# before forced vacuum
					# (change requires restart)
#autovacuum_vacuum_cost_delay = 2ms	# default vacuum cost delay for
					# autovacuum, in milliseconds;
					# -1 means use vacuum_cost_delay
#autovacuum_vacuum_cost_limit = -1	# default vacuum cost limit for
					# autovacuum, -1 means use
					# vacuum_cost_limit

#------------------------------------------------------------------------------
# CLIENT CONNECTION DEFAULTS
#------------------------------------------------------------------------------

# - Statement Behavior -

#client_min_messages = notice		# values in order of decreasing detail:
					#   debug5
					#   debug4
					#   debug3
					#   debug2
					#   debug1
					#   log
					#   notice
					#   warning
					#   error
#search_path = '"$user", public'	# schema names
#row_security = on
#default_table_access_method = 'heap'
#default_tablespace = ''		# a tablespace name, '' uses the default
#default_toast_compression = 'pglz'	# 'pglz' or 'lz4'
#temp_tablespaces = ''			# a list of tablespace names, '' uses
					# only default tablespace
#check_function_bodies = on
#default_transaction_isolation = 'read committed'
#default_transaction_read_only = off
#default_transaction_deferrable = off
#session_replication_role = 'origin'
#statement_timeout = 0			# in milliseconds, 0 is disabled
#lock_timeout = 0			# in milliseconds, 0 is disabled
#idle_in_transaction_session_timeout = 0	# in milliseconds, 0 is disabled
#idle_session_timeout = 0		# in milliseconds, 0 is disabled
#vacuum_freeze_table_age = 150000000
#vacuum_freeze_min_age = 50000000
#vacuum_failsafe_age = 1600000000
#vacuum_multixact_freeze_table_age = 150000000
#vacuum_multixact_freeze_min_age = 5000000
#vacuum_multixact_failsafe_age = 1600000000
#bytea_output = 'hex'			# hex, escape
#xmlbinary = 'base64'
#xmloption = 'content'
#gin_pending_list_limit = 4MB
#createrole_self_grant = ''		# set and/or inherit
#event_triggers = on

# - Locale and Formatting -

#datestyle = 'iso, mdy'
#intervalstyle = 'postgres'
#timezone = 'GMT'
#timezone_abbreviations = 'Default'     # Select the set of available time zone
					# abbreviations.  Currently, there are
					#   Default
					#   Australia (historical usage)
					#   India
					# You can create your own file in
					# share/timezonesets/.
#extra_float_digits = 1			# min -15, max 3; any value >0 actually
					# selects precise output mode
#client_encoding = sql_ascii		# actually, defaults to database
					# encoding

# These settings are initialized by initdb, but they can be changed.
#lc_messages = 'C'			# locale for system error message
					# strings
#lc_monetary = 'C'			# locale for monetary formatting
#lc_numeric = 'C'			# locale for number formatting
#lc_time = 'C'				# locale for time formatting

#icu_validation_level = warning		# report ICU locale validation
					# errors at the given level

# default configuration for text search
#default_text_search_config = 'pg_catalog.simple'

# - Shared Library Preloading -

#local_preload_libraries = ''
#session_preload_libraries = ''
#shared_preload_libraries = ''	# (change requires restart)
#jit_provider = 'llvmjit'		# JIT library to use

# - Other Defaults -

#dynamic_library_path = '$libdir'
#gin_fuzzy_search_limit = 0

#------------------------------------------------------------------------------
# LOCK MANAGEMENT
#------------------------------------------------------------------------------

#deadlock_timeout = 1s
#max_locks_per_transaction = 64		# min 10
					# (change requires restart)
#max_pred_locks_per_transaction = 64	# min 10
					# (change requires restart)
#max_pred_locks_per_relation = -2	# negative values mean
					# (max_pred_locks_per_transaction
					#  / -max_pred_locks_per_relation) - 1
#max_pred_locks_per_page = 2            # min 0

#------------------------------------------------------------------------------
# VERSION AND PLATFORM COMPATIBILITY
#------------------------------------------------------------------------------

# - Previous PostgreSQL Versions -

#array_nulls = on
#backslash_quote = safe_encoding	# on, off, or safe_encoding
#escape_string_warning = on
#lo_compat_privileges = off
#quote_all_identifiers = off
#standard_conforming_strings = on
#synchronize_seqscans = on

# - Other Platforms and Clients -

#transform_null_equals = off

#------------------------------------------------------------------------------
# ERROR HANDLING
#------------------------------------------------------------------------------

#exit_on_error = off			# terminate session on any error?
#restart_after_crash = on		# reinitialize after backend crash?
#data_sync_retry = off			# retry or panic on failure to fsync
					# data?
					# (change requires restart)
#recovery_init_sync_method = fsync	# fsync, syncfs (Linux 5.8+)

#------------------------------------------------------------------------------
# CONFIG FILE INCLUDES
#------------------------------------------------------------------------------

# These options allow settings to be loaded from files other than the
# default postgresql.conf.  Note that these are directives, not variable
# assignments, so they can usefully be given more than once.

#include_dir = '...'			# include files ending in '.conf' from
					# a directory, e.g., 'conf.d'
#include_if_exists = '...'		# include file only if it exists
#include = '...'			# include file

#------------------------------------------------------------------------------
# CUSTOMIZED OPTIONS
#------------------------------------------------------------------------------

# Add settings for extensions here
`

const pgConfigCustom = `
include 'postgresql-common.conf'

archive_command = 'envdir "/run/etc/wal-g.d/env" wal-g wal-push "%p"'
archive_mode = 'on'
archive_timeout = '1800s'
autovacuum = 'on'
autovacuum_analyze_scale_factor = '0.05'
autovacuum_max_workers = '5'
autovacuum_naptime = '15s'
autovacuum_vacuum_cost_delay = '2ms'
autovacuum_vacuum_cost_limit = '1800'
autovacuum_vacuum_scale_factor = '0.1'
checkpoint_timeout = '30min'
cluster_name = 'resources-canary-k8s-01'
default_statistics_target = '500'
effective_cache_size = '119GB'
effective_io_concurrency = '200'
fsync = 'on'
hot_standby = 'on'
hot_standby_feedback = 'on'
idle_in_transaction_session_timeout = '15min'
idle_session_timeout = '24h'
listen_addresses = '*'
log_autovacuum_min_duration = '5s'
log_checkpoints = 'on'
log_connections = 'on'
log_directory = '../pg_log'
log_disconnections = 'off'
log_file_mode = '0644'
log_line_prefix = '%m [%p] %q%a %u@%d %r '
log_lock_waits = 'on'
log_min_duration_sample = '500ms'
log_rotation_age = '1d'
log_rotation_size = '512MB'
log_statement = 'ddl'
log_statement_sample_rate = '0.05'
logging_collector = 'on'
maintenance_work_mem = '2048MB'
max_connections = '4000'
max_locks_per_transaction = '64'
max_parallel_workers = '14'
max_prepared_transactions = '0'
max_replication_slots = '999'
max_wal_senders = '999'
max_wal_size = '25600MB'
max_worker_processes = '14'
password_encryption = 'scram-sha-256'
pg_stat_statements.max = '10000'
pg_stat_statements.track = 'all'
pg_stat_statements.track_utility = 'off'
port = '5432'
random_page_cost = '1.1'
seq_page_cost = '1.0'
shared_buffers = '6912MB'
shared_preload_libraries = 'pg_stat_statements,uuid-ossp,hstore,pg_stat_kcache'
ssl = 'on'
ssl_ca_file = '/run/certs/ca-crt.pem'
ssl_cert_file = '/run/certs/server.crt'
ssl_ciphers = 'HIGH:!RC4:!MD5:!3DES:!aNULL'
ssl_key_file = '/run/certs/server.key'
ssl_min_protocol_version = 'TLSv1.2'
statement_timeout = '30s'
tcp_keepalives_idle = '900'
tcp_keepalives_interval = '100'
track_activities = 'on'
track_activity_query_size = '4096'
track_commit_timestamp = 'off'
track_functions = 'all'
track_io_timing = 'on'
wal_buffers = '16MB'
wal_compression = 'on'
wal_keep_size = '128MB'
wal_level = 'logical'
wal_log_hints = 'on'
hba_file = '/home/postgres/pgdata/pgroot/data/pg_hba.conf'
ident_file = '/home/postgres/pgdata/pgroot/data/pg_ident.conf'
`

const cassandraConfigSample = `
# Cassandra storage config YAML

# NOTE:
#   See http://wiki.apache.org/cassandra/StorageConfiguration for
#   full explanations of configuration directives
# /NOTE

# The name of the cluster. This is mainly used to prevent machines in
# one logical cluster from joining another.
cluster_name: contexts-2

# This defines the number of tokens randomly assigned to this node on the ring
# The more tokens, relative to other nodes, the larger the proportion of data
# that this node will store. You probably want all nodes to have the same number
# of tokens assuming they have equal hardware capability.
#
# If you leave this unspecified, Cassandra will use the default of 1 token for legacy compatibility,
# and will use the initial_token as described below.
#
# Specifying initial_token will override this setting on the node's initial start,
# on subsequent starts, this setting will apply even if initial token is set.
#
# If you already have a cluster with 1 token per node, and wish to migrate to
# multiple tokens per node, see http://wiki.apache.org/cassandra/Operations
num_tokens: 256

# When adding a vnode to an existing cluster or setting up nodes in a new datacenter,
# set to the target replication factor (RF) of keyspaces in the datacenter. Triggers
# algorithmic allocation for the RF and num_tokens for this node. The allocation algorithm
# attempts to choose tokens in a way that optimizes replicated load over the nodes in the
# datacenter for the specified RF. The load assigned to each node is close to proportional
# to the number of vnodes.
# As of cassandra-3.11.4 there is no reference to this key in open source version so it must be
# DSE-only. Don't try to use it. Use allocate_tokens_for_keyspace instead.
# allocate_tokens_for_local_replication_factor: ''

# Triggers automatic allocation of num_tokens tokens for this node. The allocation
# algorithm attempts to choose tokens in a way that optimizes replicated load over
# the nodes in the datacenter for the replication strategy used by the specified
# keyspace.
#
# The load assigned to each node will be close to proportional to its number of
# vnodes.
#
# Only supported with the Murmur3Partitioner.
# allocate_tokens_for_keyspace: KEYSPACE

# initial_token allows you to specify tokens manually.  While you can use # it with
# vnodes (num_tokens > 1, above) -- in which case you should provide a
# comma-separated list -- it's primarily used when adding nodes # to legacy clusters
# that do not have vnodes enabled.
# initial_token:

# See http://wiki.apache.org/cassandra/HintedHandoff
# May either be "true" or "false" to enable globally
hinted_handoff_enabled: true
# When hinted_handoff_enabled is true, a black list of data centers that will not
# perform hinted handoff
#hinted_handoff_disabled_datacenters:
#    - DC1
#    - DC2
# this defines the maximum amount of time a dead host will have hints
# generated.  After it has been dead this long, new hints for it will not be
# created until it has been seen alive and gone down again.
max_hint_window_in_ms: 10800000 # 3 hours

# Maximum throttle in KBs per second, per delivery thread.  This will be
# reduced proportionally to the number of nodes in the cluster.  (If there
# are two nodes in the cluster, each delivery thread will use the maximum
# rate; if there are three, each will throttle to half of the maximum,
# since we expect two nodes to be delivering hints simultaneously.)
hinted_handoff_throttle_in_kb: 1024

# Number of threads with which to deliver hints;
# Consider increasing this number when you have multi-dc deployments, since
# cross-dc handoff tends to be slower
max_hints_delivery_threads: 2

# Directory where Cassandra should store hints.
# If not set, the default directory is $CASSANDRA_HOME/data/hints.
# hints_directory: /var/lib/cassandra/hints

# How often hints should be flushed from the internal buffers to disk.
# Will *not* trigger fsync.
hints_flush_period_in_ms: 10000

# Maximum size for a single hints file, in megabytes.
max_hints_file_size_in_mb: 128

# Compression to apply to the hint files. If omitted, hints files
# will be written uncompressed. LZ4, Snappy, and Deflate compressors
# are supported.
#hints_compression:
#   - class_name: LZ4Compressor
#     parameters:
#         -

# Maximum throttle in KBs per second, total. This will be
# reduced proportionally to the number of nodes in the cluster.
batchlog_replay_throttle_in_kb: 1024

# Authentication backend, implementing IAuthenticator; used to identify users
# Out of the box, Cassandra provides org.apache.cassandra.auth.{AllowAllAuthenticator,
# PasswordAuthenticator}.
#
# - AllowAllAuthenticator performs no checks - set it to disable authentication.
# - PasswordAuthenticator relies on username/password pairs to authenticate
#   users. It keeps usernames and hashed passwords in system_auth.roles table.
#   Please increase system_auth keyspace replication factor if you use this authenticator.
#   If using PasswordAuthenticator, CassandraRoleManager must also be used (see below)
authenticator: AllowAllAuthenticator

# Authorization backend, implementing IAuthorizer; used to limit access/provide permissions
# Out of the box, Cassandra provides org.apache.cassandra.auth.{AllowAllAuthorizer,
# CassandraAuthorizer}.
#
# - AllowAllAuthorizer allows any action to any user - set it to disable authorization.
# - CassandraAuthorizer stores permissions in system_auth.role_permissions table. Please
#   increase system_auth keyspace replication factor if you use this authorizer.
authorizer: AllowAllAuthorizer


# Part of the Authentication & Authorization backend, implementing IRoleManager; used
# to maintain grants and memberships between roles.
# Out of the box, Cassandra provides org.apache.cassandra.auth.CassandraRoleManager,
# which stores role information in the system_auth keyspace. Most functions of the
# IRoleManager require an authenticated login, so unless the configured IAuthenticator
# actually implements authentication, most of this functionality will be unavailable.
#
# - CassandraRoleManager stores role data in the system_auth keyspace. Please
#   increase system_auth keyspace replication factor if you use this role manager.
role_manager: CassandraRoleManager

# Validity period for roles cache (fetching permissions can be an
# expensive operation depending on the authorizer). Granted roles are cached for
# authenticated sessions in AuthenticatedUser and after the period specified
# here, become eligible for (async) reload.
# Defaults to 2000, set to 0 to disable.
# Will be disabled automatically for AllowAllAuthenticator.
roles_validity_in_ms: 2000

# Refresh interval for roles cache (if enabled).
# After this interval, cache entries become eligible for refresh. Upon next
# access, an async reload is scheduled and the old value returned until it
# completes. If roles_validity_in_ms is non-zero, then this must be
# also.
# Defaults to the same value as roles_validity_in_ms.
# roles_update_interval_in_ms: 1000

# Validity period for permissions cache (fetching permissions can be an
# expensive operation depending on the authorizer, CassandraAuthorizer is
# one example). Defaults to 2000, set to 0 to disable.
# Will be disabled automatically for AllowAllAuthorizer.
permissions_validity_in_ms: 2000

# Refresh interval for permissions cache (if enabled).
# After this interval, cache entries become eligible for refresh. Upon next
# access, an async reload is scheduled and the old value returned until it
# completes. If permissions_validity_in_ms is non-zero, then this must be
# also.
# Defaults to the same value as permissions_validity_in_ms.
# permissions_update_interval_in_ms: 1000

# The partitioner is responsible for distributing groups of rows (by
# partition key) across nodes in the cluster.  You should leave this
# alone for new clusters.  The partitioner can NOT be changed without
# reloading all data, so when upgrading you should set this to the
# same partitioner you were already using.
#
# Besides Murmur3Partitioner, partitioners included for backwards
# compatibility include RandomPartitioner, ByteOrderedPartitioner, and
# OrderPreservingPartitioner.
#
partitioner: org.apache.cassandra.dht.Murmur3Partitioner

# Directories where Cassandra should store data on disk.  Cassandra
# will spread data evenly across them, subject to the granularity of
# the configured compaction strategy.
# If not set, the default directory is $CASSANDRA_HOME/data/data.
# data_file_directories:
#     - /var/lib/cassandra/data

# commit log.  when running on magnetic HDD, this should be a
# separate spindle than the data directories.
# If not set, the default directory is $CASSANDRA_HOME/data/commitlog.
# commitlog_directory: /var/lib/cassandra/commitlog

# policy for data disk failures:
# die: shut down gossip and client transports and kill the JVM for any fs errors or
#      single-sstable errors, so the node can be replaced.
# stop_paranoid: shut down gossip and client transports even for single-sstable errors,
#                kill the JVM for errors during startup.
# stop: shut down gossip and client transports, leaving the node effectively dead, but
#       can still be inspected via JMX, kill the JVM for errors during startup.
# best_effort: stop using the failed disk and respond to requests based on
#              remaining available sstables.  This means you WILL see obsolete
#              data at CL.ONE!
# ignore: ignore fatal errors and let requests fail, as in pre-1.2 Cassandra
disk_failure_policy: stop

# policy for commit disk failures:
# die: shut down gossip and Thrift and kill the JVM, so the node can be replaced.
# stop: shut down gossip and Thrift, leaving the node effectively dead, but
#       can still be inspected via JMX.
# stop_commit: shutdown the commit log, letting writes collect but
#              continuing to service reads, as in pre-2.0.5 Cassandra
# ignore: ignore fatal errors and let the batches fail
commit_failure_policy: stop

# Maximum size of the key cache in memory.
#
# Each key cache hit saves 1 seek and each row cache hit saves 2 seeks at the
# minimum, sometimes more. The key cache is fairly tiny for the amount of
# time it saves, so it's worthwhile to use it at large numbers.
# The row cache saves even more time, but must contain the entire row,
# so it is extremely space-intensive. It's best to only use the
# row cache if you have hot rows or static rows.
#
# NOTE: if you reduce the size, you may not get you hottest keys loaded on startup.
#
# Default value is empty to make it "auto" (min(5% of Heap (in MB), 100MB)). Set to 0 to disable key cache.
key_cache_size_in_mb: 0

# Duration in seconds after which Cassandra should
# save the key cache. Caches are saved to saved_caches_directory as
# specified in this configuration file.
#
# Saved caches greatly improve cold-start speeds, and is relatively cheap in
# terms of I/O for the key cache. Row cache saving is much more expensive and
# has limited use.
#
# Default is 14400 or 4 hours.
key_cache_save_period: 3600

# Number of keys from the key cache to save
# Disabled by default, meaning all keys are going to be saved
# key_cache_keys_to_save: 100

# Row cache implementation class name.
# Available implementations:
#   org.apache.cassandra.cache.OHCProvider                Fully off-heap row cache implementation (default).
#   org.apache.cassandra.cache.SerializingCacheProvider   This is the row cache implementation availabile
#                                                         in previous releases of Cassandra.
# row_cache_class_name: org.apache.cassandra.cache.OHCProvider

# Maximum size of the row cache in memory.
# Please note that OHC cache implementation requires some additional off-heap memory to manage
# the map structures and some in-flight memory during operations before/after cache entries can be
# accounted against the cache capacity. This overhead is usually small compared to the whole capacity.
# Do not specify more memory that the system can afford in the worst usual situation and leave some
# headroom for OS block level cache. Do never allow your system to swap.
#
# Default value is 0, to disable row caching.
row_cache_size_in_mb: 0

# Duration in seconds after which Cassandra should save the row cache.
# Caches are saved to saved_caches_directory as specified in this configuration file.
#
# Saved caches greatly improve cold-start speeds, and is relatively cheap in
# terms of I/O for the key cache. Row cache saving is much more expensive and
# has limited use.
#
# Default is 0 to disable saving the row cache.
row_cache_save_period: 0

# Number of keys from the row cache to save.
# Specify 0 (which is the default), meaning all keys are going to be saved
# row_cache_keys_to_save: 100

# Maximum size of the counter cache in memory.
#
# Counter cache helps to reduce counter locks' contention for hot counter cells.
# In case of RF = 1 a counter cache hit will cause Cassandra to skip the read before
# write entirely. With RF > 1 a counter cache hit will still help to reduce the duration
# of the lock hold, helping with hot counter cell updates, but will not allow skipping
# the read entirely. Only the local (clock, count) tuple of a counter cell is kept
# in memory, not the whole counter, so it's relatively cheap.
#
# NOTE: if you reduce the size, you may not get you hottest keys loaded on startup.
#
# Default value is empty to make it "auto" (min(2.5% of Heap (in MB), 50MB)). Set to 0 to disable counter cache.
# NOTE: if you perform counter deletes and rely on low gcgs, you should disable the counter cache.
counter_cache_size_in_mb:

# Duration in seconds after which Cassandra should
# save the counter cache (keys only). Caches are saved to saved_caches_directory as
# specified in this configuration file.
#
# Default is 7200 or 2 hours.
counter_cache_save_period: 7200

# Number of keys from the counter cache to save
# Disabled by default, meaning all keys are going to be saved
# counter_cache_keys_to_save: 100

# saved caches
# If not set, the default directory is $CASSANDRA_HOME/data/saved_caches.
# saved_caches_directory: /var/lib/cassandra/saved_caches

# commitlog_sync may be either "periodic" or "batch."
#
# When in batch mode, Cassandra won't ack writes until the commit log
# has been fsynced to disk.  It will wait
# commitlog_sync_batch_window_in_ms milliseconds between fsyncs.
# This window should be kept short because the writer threads will
# be unable to do extra work while waiting.  (You may need to increase
# concurrent_writes for the same reason.)
#
# commitlog_sync: batch
# commitlog_sync_batch_window_in_ms: 2
#
# the other option is "periodic" where writes may be acked immediately
# and the CommitLog is simply synced every commitlog_sync_period_in_ms
# milliseconds.
commitlog_sync: periodic
commitlog_sync_period_in_ms: 10000

# The size of the individual commitlog file segments.  A commitlog
# segment may be archived, deleted, or recycled once all the data
# in it (potentially from each columnfamily in the system) has been
# flushed to sstables.
#
# The default size is 32, which is almost always fine, but if you are
# archiving commitlog segments (see commitlog_archiving.properties),
# then you probably want a finer granularity of archiving; 8 or 16 MB
# is reasonable.
# Max mutation size is also configurable via max_mutation_size_in_kb setting in
# cassandra.yaml. The default is half the size commitlog_segment_size_in_mb * 1024.
# This should be positive and less than 2048.
#
# NOTE: If max_mutation_size_in_kb is set explicitly then commitlog_segment_size_in_mb must
# be set to at least twice the size of max_mutation_size_in_kb / 1024
#
commitlog_segment_size_in_mb: 32

# Compression to apply to the commit log. If omitted, the commit log
# will be written uncompressed.  LZ4, Snappy, and Deflate compressors
# are supported.
`

const cassandraLogbackSample = `<configuration scan="true">
  <jmxConfigurator />

  <appender name="STDOUT" class="ch.qos.logback.core.ConsoleAppender">
    <encoder>
      <pattern>%-5level [%thread] %date{ISO8601} %F:%L - %msg%n</pattern>
    </encoder>
  </appender>

  <root level="INFO">
    <appender-ref ref="STDOUT" />
  </root>

  <!-- <logger name="com.thinkaurelius.thrift" level="ERROR"/>  -->
</configuration>`

const mongodConfigSample = `
# mongod.conf

# for documentation of all options, see:
#   http://docs.mongodb.org/manual/reference/configuration-options/

# where to write logging data.
systemLog:
  destination: file
  logAppend: true
  path: /var/log/mongodb/mongod.log

# Where and how to store data.
storage:
  dbPath: /var/lib/mongo

# how the process runs
processManagement:
  timeZoneInfo: /usr/share/zoneinfo

# network interfaces
net:
  port: 27017
  bindIp: 127.0.0.1  # Enter 0.0.0.0,:: to bind to all IPv4 and IPv6 addresses or, alternatively, use the net.bindIpAll setting.

#security:

#operationProfiling:

#replication:

#sharding:

## Enterprise-Only Options

#auditLog:
`
