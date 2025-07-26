// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package verification

var AdminFunctions = []string{
	// 9.24. System Administration Functions, https://www.postgresql.org/docs/9.1/functions-admin.html

	// Table 9-55. Configuration Settings Functions
	"current_setting",
	"set_config",
	// Table 9-56. Server Signalling Functions
	"pg_cancel_backend",
	"pg_reload_conf",
	"pg_rotate_logfile",
	"pg_terminate_backend",

	// Table 9-57. Backup Control Functions
	"pg_create_restore_point",
	"pg_current_xlog_insert_location",
	"pg_current_xlog_location",
	"pg_start_backup",
	"pg_stop_backup",
	"pg_switch_xlog",
	"pg_xlogfile_name",
	"pg_xlogfile_name_offset",

	// Table 9-58. Recovery Information Functions
	"pg_is_in_recovery",
	"pg_last_xlog_receive_location",
	"pg_last_xlog_replay_location",
	"pg_last_xact_replay_timestamp",

	// Table 9-59. Recovery Control Functions
	"pg_is_xlog_replay_paused",
	"pg_xlog_replay_pause",
	"pg_xlog_replay_resume",

	// Table 9-60. Database Object Size Functions
	"pg_column_size",
	"pg_database_size",
	"pg_indexes_size",
	"pg_relation_size",
	"pg_size_pretty",
	"pg_table_size",
	"pg_tablespace_size",
	"pg_total_relation_size",

	// Table 9-61. Database Object Location Functions
	"pg_relation_filenode",
	"pg_relation_filepath",

	// Table 9-62. Generic File Access Functions
	"pg_ls_dir",
	"pg_read_file",
	"pg_read_binary_file",
	"pg_stat_file",

	// Table 9-63. Advisory Lock Functions
	"pg_advisory_lock",
	"pg_advisory_lock_shared",
	"pg_advisory_unlock",
	"pg_advisory_unlock_all",
	"pg_advisory_unlock_shared",
	"pg_advisory_xact_lock",
	"pg_advisory_xact_lock_shared",
	"pg_try_advisory_lock",
	"pg_try_advisory_lock_shared",
	"pg_try_advisory_xact_lock",
	"pg_try_advisory_xact_lock_shared",
}

var InfoFunctions = []string{
	// 9.23. System Information Functions, https://www.postgresql.org/docs/9.1/functions-info.html

	// Table 9-48. Session Information Functions
	// except "current_user", "session_user", "user", "version"
	"current_catalog",
	"current_database",
	"current_query",
	"current_schema",
	"current_schemas",
	"inet_client_addr",
	"inet_client_port",
	"inet_server_addr",
	"inet_server_port",
	"pg_backend_pid",
	"pg_conf_load_time",
	"pg_is_other_temp_schema",
	"pg_listening_channels",
	"pg_my_temp_schema",
	"pg_postmaster_start_time",

	// Table 9-49. Access Privilege Inquiry Functions
	"has_any_column_privilege",
	"has_database_privilege",
	"has_foreign_data_wrapper_privilege",
	"has_function_privilege",
	"has_language_privilege",
	"has_schema_privilege",
	"has_sequence_privilege",
	"has_server_privilege",
	"has_table_privilege",
	"has_tablespace_privilege",
	"pg_has_role",

	// Table 9-50. Schema Visibility Inquiry Functions
	"pg_collation_is_visible",
	"pg_conversion_is_visible",
	"pg_function_is_visible",
	"pg_opclass_is_visible",
	"pg_operator_is_visible",
	"pg_table_is_visible",
	"pg_ts_config_is_visible",
	"pg_ts_dict_is_visible",
	"pg_ts_parser_is_visible",
	"pg_ts_template_is_visible",
	"pg_type_is_visible",

	// Table 9-51. System Catalog Information Functions
	"format_type",
	"pg_describe_object",
	"pg_get_constraintdef",
	"pg_get_expr",
	"pg_get_functiondef",
	"pg_get_function_arguments",
	"pg_get_function_identity_arguments",
	"pg_get_function_result",
	"pg_get_indexdef",
	"pg_get_keywords",
	"pg_get_ruledef",
	"pg_get_serial_sequence",
	"pg_get_triggerdef",
	"pg_get_userbyid",
	"pg_get_viewdef",
	"pg_options_to_table",
	"pg_tablespace_databases",
	"pg_typeof",

	// Table 9-52. Comment Information Functions
	"col_description",
	"obj_description",
	"shobj_description",

	// Table 9-53. Transaction IDs and Snapshots
	"txid_current",
	"txid_current_snapshot",
	"txid_snapshot_xip",
	"txid_snapshot_xmax",
	"txid_snapshot_xmin",
	"txid_visible_in_snapshot",

	// Table 9-54. Snapshot Components
	"xmin",
	"xmax",
	"xip_list",
}

var Tables = []string{
	"pg_stat_activity",
}
