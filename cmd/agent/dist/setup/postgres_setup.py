"""
DBM Postgres setup — Detect → Plan → Apply using psycopg (v3).

Called by 'datadog-agent integration setup postgres' via the embedded Python binary.
Reads a JSON config from stdin, writes a JSON result to stdout.
Function bodies mirror integrations-core/postgres/tests/compose/resources/03_setup.sh.
NOTE: if the canonical SQL in integrations-core changes, this file must be updated manually.

Connections reuse the Postgres integration's connection layer rather than a
standalone copy: delegates to datadog_checks.postgres.connection_pool.configure_connection
(autocommit, SQL_ASCII text decoding, CommenterCursor) and the integration's
database-autodiscovery query. When the integration package isn't importable
(e.g. dev runs with --use-sys-python), falls back to a minimal psycopg config.
"""

import json
import sys

try:
    import psycopg
    from psycopg import sql

    _IMPORT_ERROR = None
except ImportError as exc:  # pragma: no cover - exercised only without psycopg3 installed
    psycopg = None
    sql = None
    _IMPORT_ERROR = exc

# Reuse the integration's connection layer when available so setup connects and
# decodes exactly like the running check.
try:
    from datadog_checks.postgres.connection_pool import configure_connection as _integration_configure_connection
    from datadog_checks.postgres.discovery import AUTODISCOVERY_QUERY

    _INTEGRATION_CONFIGURE = _integration_configure_connection
except ImportError:  # pragma: no cover - dev / --use-sys-python without the integration
    _INTEGRATION_CONFIGURE = None
    AUTODISCOVERY_QUERY = "select datname from pg_catalog.pg_database where datistemplate = false"

# (name, desired_value, requires_restart, required)
# required=False means the setting is optional per DBM docs — DBM works without it,
# but it improves query coverage and performance visibility.
#
# pg_stat_statements.max is intentionally excluded. It can only be set after
# pg_stat_statements is loaded (which requires a restart), and it is itself a
# postmaster-context GUC that requires a second restart. Requiring two restarts
# on a fresh install is poor UX; the PostgreSQL default of 5000 is sufficient
# for most deployments. See the note at the end of the RFC for details.
_SETTINGS = [
    ("shared_preload_libraries", "pg_stat_statements", True, True),
    ("track_activity_query_size", "4096", True, True),
    ("pg_stat_statements.track", "all", False, False),
    ("track_io_timing", "on", False, False),
    ("pg_stat_statements.track_utility", "on", False, False),
]

_SQL_FUNC_PG_STAT_ACTIVITY = """
CREATE OR REPLACE FUNCTION datadog.pg_stat_activity() RETURNS SETOF pg_stat_activity AS
  $$ SELECT * FROM pg_catalog.pg_stat_activity; $$
LANGUAGE sql SECURITY DEFINER"""

_SQL_FUNC_PG_STAT_STATEMENTS = """
CREATE OR REPLACE FUNCTION datadog.pg_stat_statements() RETURNS SETOF pg_stat_statements AS
  $$ SELECT * FROM pg_stat_statements; $$
LANGUAGE sql SECURITY DEFINER"""

_SQL_FUNC_EXPLAIN_STATEMENT = """
CREATE OR REPLACE FUNCTION datadog.explain_statement(
    l_query TEXT,
    OUT explain JSON
)
RETURNS SETOF JSON AS
$$
DECLARE
curs REFCURSOR;
plan JSON;

BEGIN
    OPEN curs FOR EXECUTE pg_catalog.concat('EXPLAIN (FORMAT JSON) ', l_query);
    FETCH curs INTO plan;
    CLOSE curs;
    RETURN QUERY SELECT plan;
END;
$$
LANGUAGE 'plpgsql'
RETURNS NULL ON NULL INPUT
SECURITY DEFINER"""


# ---------------------------------------------------------------------------
# Connections
# ---------------------------------------------------------------------------


def _configure_connection(conn):
    """Configure a psycopg connection for setup.

    Delegates to the integration's configure_connection when available so setup
    connects exactly like the running check (autocommit, SQL_ASCII text decoding,
    CommenterCursor). Falls back to a minimal config when the integration package
    isn't importable (dev / --use-sys-python).

    ClientCursor is required — not just stylistic: PostgreSQL rejects server-side
    bound parameters in utility statements (CREATE USER ... WITH PASSWORD %s),
    which psycopg3's default server-binding Cursor would send as $1.
    CommenterCursor subclasses ClientCursor, so the fallback sets it directly."""
    if _INTEGRATION_CONFIGURE is not None:
        _INTEGRATION_CONFIGURE(conn)
    else:
        conn.autocommit = True
        conn.cursor_factory = psycopg.ClientCursor
    return conn


def _connect(uri, dbname=None):
    """Open a configured psycopg (v3) connection. `dbname`, when given, overrides
    the database in `uri` for per-database operations."""
    kwargs = {}
    if dbname:
        kwargs["dbname"] = dbname
    return _configure_connection(psycopg.connect(uri, **kwargs))


def _query(cur, composed):
    """Render a psycopg.sql composition to a plain string. CommenterCursor runs
    add_sql_comment(), which calls .strip() on the query, so it needs a str —
    not a sql.Composed. Identifiers stay safely quoted by as_string()."""
    return composed.as_string(cur)


# ---------------------------------------------------------------------------
# Detect
# ---------------------------------------------------------------------------


def _detect(cur, config):
    # Fail immediately if connected to a standby/read replica.
    cur.execute("SELECT pg_is_in_recovery()")
    if cur.fetchone()[0]:
        raise RuntimeError(
            "Connected to a read replica (pg_is_in_recovery() = true); connect to the primary instance to run setup"
        )

    flavor = _detect_flavor(cur)
    pg_version = _detect_version(cur)
    current_settings, pending_restart = _detect_settings(cur)
    user_exists = _detect_user_exists(cur, config["datadog_user"])
    databases = _detect_databases(cur, config)

    return {
        "flavor": flavor,
        "pg_version": pg_version,
        "user_exists": user_exists,
        "current_settings": current_settings,
        "pending_restart": pending_restart,
        "databases": databases,
    }


def _detect_flavor(cur):
    try:
        cur.execute("SELECT 1 FROM pg_roles WHERE rolname = 'cloudsqladmin'")
        if cur.fetchone():
            return "cloud_sql"
    except Exception:
        pass

    try:
        cur.execute("SELECT 1 FROM pg_roles WHERE rolname = 'azure_superuser'")
        if cur.fetchone():
            return "azure"
    except Exception:
        pass

    try:
        cur.execute("SELECT current_setting('rds.extensions', true)")
        result = cur.fetchone()
        if result and result[0]:
            cur.execute("SELECT version()")
            version_str = cur.fetchone()[0]
            return "aurora" if "Aurora" in version_str else "rds"
    except Exception:
        pass

    return "self_hosted"


def _detect_version(cur):
    cur.execute("SHOW server_version_num")
    return int(cur.fetchone()[0]) // 10000


def _detect_settings(cur):
    cur.execute("""
        SELECT name, setting, pending_restart
        FROM pg_settings
        WHERE name IN (
            'shared_preload_libraries', 'track_activity_query_size',
            'pg_stat_statements.max', 'pg_stat_statements.track',
            'track_io_timing', 'pg_stat_statements.track_utility'
        )
    """)
    current_settings = {}
    pending_restart = []
    for name, setting, is_pending in cur.fetchall():
        current_settings[name] = setting
        if is_pending:
            pending_restart.append(name)
    return current_settings, pending_restart


def _detect_user_exists(cur, username):
    cur.execute("SELECT 1 FROM pg_roles WHERE rolname = %s", (username,))
    return cur.fetchone() is not None


def _detect_databases(cur, config):
    if config.get("all_databases"):
        # Reuse the integration's autodiscovery query; sort for stable output.
        cur.execute(AUTODISCOVERY_QUERY)
        return sorted(row[0] for row in cur.fetchall())
    return config.get("databases", [])


# ---------------------------------------------------------------------------
# Plan
# ---------------------------------------------------------------------------


def _plan(state, config):
    ops = []
    ops.extend(_plan_user_ops(state, config))
    ops.extend(_plan_grant_ops(state, config))
    setting_ops, optional_restart_pending = _plan_setting_ops(state, config)
    ops.extend(setting_ops)
    for db in state["databases"]:
        ops.extend(_plan_per_db_ops(config, db))
    return ops, optional_restart_pending


def _plan_user_ops(state, config):
    user = config["datadog_user"]
    if not state["user_exists"]:
        if not config.get("datadog_password"):
            raise RuntimeError(f"--datadog-password is required when creating user {user!r} for the first time")
        return [
            {
                "kind": "SQL",
                "description": f"create user {user!r}",
                "op_type": "create_user",
                "args": [user, config["datadog_password"]],
                "redact": True,
            }
        ]
    if config.get("datadog_password") and config.get("update_password"):
        return [
            {
                "kind": "SQL",
                "description": f"sync password for user {user!r}",
                "op_type": "alter_user_password",
                "args": [user, config["datadog_password"]],
                "redact": True,
            }
        ]
    return [{"kind": "SKIP", "description": f"user {user!r} — already exists", "status": "skipped"}]


def _plan_grant_ops(state, config):
    user = config["datadog_user"]
    ops = []
    if state["pg_version"] >= 10:
        ops.append(
            {
                "kind": "SQL",
                "description": f"GRANT pg_monitor TO {user!r}",
                "op_type": "grant_pg_monitor",
                "args": [user],
            }
        )
    else:
        ops.append(
            {
                "kind": "SQL",
                "description": f"GRANT pg_stat_* tables to {user!r} (PG 9.6)",
                "op_type": "grant_pg96",
                "args": [user],
            }
        )
    if state["pg_version"] >= 15 and state["flavor"] in ("rds", "aurora"):
        ops.append(
            {
                "kind": "SQL",
                "description": f"ALTER ROLE {user!r} INHERIT (RDS/Aurora PG 15+)",
                "op_type": "alter_role_inherit",
                "args": [user],
            }
        )
    return ops


def _plan_setting_ops(state, config):
    # pg_stat_statements GUCs are only registered after the library is loaded.
    # pg_stat_statements explicitly blocks mid-session LOAD, so these GUCs
    # cannot be set until after the first restart that loads the library.
    spl_active = state["current_settings"].get("shared_preload_libraries", "")
    pg_stat_loaded = "pg_stat_statements" in [lib.strip() for lib in spl_active.split(",")]

    apply_optional_restart = config.get("apply_optional_restart", False)

    ops = []
    optional_restart_pending = []

    for name, desired, requires_restart, required in _SETTINGS:
        current = state["current_settings"].get(name, "")
        flavor = state["flavor"]

        # Skip pg_stat_statements.* GUCs until the library is loaded.
        if flavor == "self_hosted" and name.startswith("pg_stat_statements.") and not pg_stat_loaded:
            ops.append(
                {
                    "kind": "SKIP",
                    "description": f"{name} — applied on next run after pg_stat_statements loads",
                    "setting_name": name,
                    "status": "skipped",
                }
            )
            continue

        # Optional restart-required settings: skip unless explicitly requested.
        if not required and requires_restart and flavor == "self_hosted":
            if not apply_optional_restart:
                # Check if it already has the desired value before deferring.
                if current == desired or (name == "shared_preload_libraries" and desired in current.split(",")):
                    pass  # already set — fall through to normal planning (will show SKIP)
                else:
                    optional_restart_pending.append(
                        {
                            "name": name,
                            "desired": desired,
                            "current": current,
                            "description": f"optional — set {name}={desired} (requires one more restart)",
                        }
                    )
                    ops.append(
                        {
                            "kind": "SKIP",
                            "description": f"{name} — optional, skipped (add --yes to apply, requires restart)",
                            "setting_name": name,
                            "status": "skipped",
                        }
                    )
                    continue

        if flavor == "self_hosted":
            ops.extend(_plan_self_hosted_setting(state, name, desired, current, requires_restart))
        elif flavor in ("rds", "aurora"):
            ops.extend(_plan_aws_setting(state, name, desired, current))
        elif flavor == "cloud_sql":
            ops.extend(_plan_cloud_sql_setting(name, desired, current))
        elif flavor == "azure":
            ops.extend(_plan_azure_setting(name, desired, current))

    return ops, optional_restart_pending


def _plan_self_hosted_setting(state, name, desired, current, requires_restart):
    if name == "shared_preload_libraries":
        return _plan_spl(state, desired, current)
    if current == desired:
        return [
            {
                "kind": "SKIP",
                "description": f"{name} = {desired} — already set",
                "setting_name": name,
                "status": "skipped",
            }
        ]
    if name in state["pending_restart"]:
        return [
            {
                "kind": "SKIP",
                "description": f"{name} — restart already pending, skipping ALTER SYSTEM",
                "setting_name": name,
                "status": "skipped",
            }
        ]
    kind = "ALTER_SYSTEM" if requires_restart else "ALTER_SYSTEM_RELOAD"
    return [
        {
            "kind": kind,
            "description": f"ALTER SYSTEM SET {name} = '{desired}'",
            "sql": f"ALTER SYSTEM SET {name} = '{desired}'",
            "setting_name": name,
            "requires_restart": requires_restart,
        }
    ]


def _plan_spl(state, desired, current):
    libs = [lib.strip() for lib in current.split(",") if lib.strip()]
    if desired in libs:
        return [
            {
                "kind": "SKIP",
                "description": f"shared_preload_libraries already contains '{desired}'",
                "setting_name": "shared_preload_libraries",
                "status": "skipped",
            }
        ]
    if "shared_preload_libraries" in state["pending_restart"]:
        return [
            {
                "kind": "SKIP",
                "description": "shared_preload_libraries — restart already pending; check postgresql.auto.conf before restarting",
                "setting_name": "shared_preload_libraries",
                "status": "skipped",
            }
        ]
    new_value = f"{current},{desired}" if current else desired
    return [
        {
            "kind": "ALTER_SYSTEM",
            "description": f"ALTER SYSTEM SET shared_preload_libraries = '{new_value}'",
            "sql": f"ALTER SYSTEM SET shared_preload_libraries = '{new_value}'",
            "setting_name": "shared_preload_libraries",
            "requires_restart": True,
        }
    ]


def _plan_aws_setting(state, name, desired, current):
    if name == "shared_preload_libraries" and state["flavor"] == "aurora" and desired in current.split(","):
        return [
            {
                "kind": "SKIP",
                "description": f"shared_preload_libraries already contains '{desired}' on Aurora — skipped",
                "setting_name": name,
                "status": "skipped",
            }
        ]
    instruction = (
        f"Set {name} = '{desired}'\n    → RDS Console → Parameter Groups → your group → save → reboot instance"
    )
    return [
        {
            "kind": "MANUAL_STEP",
            "description": f"[AWS Parameter Group] {name} = '{desired}'",
            "setting_name": name,
            "manual_instruction": instruction,
            "status": "manual",
        }
    ]


def _plan_cloud_sql_setting(name, desired, current):
    if name == "shared_preload_libraries":
        return [
            {
                "kind": "SKIP",
                "description": "shared_preload_libraries — pre-loaded on Cloud SQL",
                "setting_name": name,
                "status": "skipped",
            }
        ]
    if current == desired:
        return [
            {
                "kind": "SKIP",
                "description": f"{name} = {desired} — already set",
                "setting_name": name,
                "status": "skipped",
            }
        ]
    instruction = (
        f"Set {name} = '{desired}'\n"
        f"    → Cloud SQL Console → your instance → Edit → Database flags → save → restart instance"
    )
    return [
        {
            "kind": "MANUAL_STEP",
            "description": f"[Cloud SQL Database Flag] {name} = '{desired}'",
            "setting_name": name,
            "manual_instruction": instruction,
            "status": "manual",
        }
    ]


def _plan_azure_setting(name, desired, current):
    if name == "shared_preload_libraries" and desired in current.split(","):
        return [
            {
                "kind": "SKIP",
                "description": f"shared_preload_libraries already contains '{desired}' on Azure — skipped",
                "setting_name": name,
                "status": "skipped",
            }
        ]
    if current == desired:
        return [
            {
                "kind": "SKIP",
                "description": f"{name} = {desired} — already set",
                "setting_name": name,
                "status": "skipped",
            }
        ]
    instruction = (
        f"Set {name} = '{desired}'\n    → Azure Portal → your server → Server parameters → save → restart if required"
    )
    return [
        {
            "kind": "MANUAL_STEP",
            "description": f"[Azure Server Parameters] {name} = '{desired}'",
            "setting_name": name,
            "manual_instruction": instruction,
            "status": "manual",
        }
    ]


def _plan_per_db_ops(config, db):
    user = config["datadog_user"]
    return [
        {
            "kind": "SQL",
            "description": "CREATE EXTENSION IF NOT EXISTS pg_stat_statements",
            "op_type": "create_extension",
            "database": db,
        },
        {
            "kind": "SQL",
            "description": "CREATE SCHEMA IF NOT EXISTS datadog",
            "op_type": "create_schema",
            "database": db,
        },
        {
            "kind": "SQL",
            "description": f"GRANT USAGE ON SCHEMA datadog TO {user!r}",
            "op_type": "grant_schema_usage",
            "args": [user],
            "database": db,
        },
        {
            "kind": "SQL",
            "description": "CREATE OR REPLACE FUNCTION datadog.pg_stat_activity()",
            "op_type": "func_pg_stat_activity",
            "database": db,
        },
        {
            "kind": "SQL",
            "description": "CREATE OR REPLACE FUNCTION datadog.pg_stat_statements()",
            "op_type": "func_pg_stat_statements",
            "database": db,
        },
        {
            "kind": "SQL",
            "description": "CREATE OR REPLACE FUNCTION datadog.explain_statement()",
            "op_type": "func_explain_statement",
            "database": db,
        },
    ]


# ---------------------------------------------------------------------------
# Apply
# ---------------------------------------------------------------------------


def _execute_op(cur, op):
    """Execute a single operation using psycopg (v3) with safe identifier handling."""
    op_type = op.get("op_type")
    args = op.get("args", [])

    if op_type == "create_user":
        cur.execute(
            _query(cur, sql.SQL("CREATE USER {} WITH PASSWORD %s").format(sql.Identifier(args[0]))),
            (args[1],),
        )
    elif op_type == "alter_user_password":
        cur.execute(
            _query(cur, sql.SQL("ALTER USER {} WITH PASSWORD %s").format(sql.Identifier(args[0]))),
            (args[1],),
        )
    elif op_type == "grant_pg_monitor":
        cur.execute(_query(cur, sql.SQL("GRANT pg_monitor TO {}").format(sql.Identifier(args[0]))))
    elif op_type == "grant_pg96":
        cur.execute(
            _query(
                cur,
                sql.SQL(
                    "GRANT SELECT ON pg_stat_database TO {};"
                    "GRANT SELECT ON pg_stat_database_conflicts TO {};"
                    "GRANT SELECT ON pg_stat_bgwriter TO {}"
                ).format(
                    sql.Identifier(args[0]),
                    sql.Identifier(args[0]),
                    sql.Identifier(args[0]),
                ),
            )
        )
    elif op_type == "alter_role_inherit":
        cur.execute(_query(cur, sql.SQL("ALTER ROLE {} INHERIT").format(sql.Identifier(args[0]))))
    elif op_type == "create_extension":
        cur.execute("CREATE EXTENSION IF NOT EXISTS pg_stat_statements")
    elif op_type == "create_schema":
        cur.execute("CREATE SCHEMA IF NOT EXISTS datadog")
    elif op_type == "grant_schema_usage":
        cur.execute(_query(cur, sql.SQL("GRANT USAGE ON SCHEMA datadog TO {}").format(sql.Identifier(args[0]))))
    elif op_type == "func_pg_stat_activity":
        cur.execute(_SQL_FUNC_PG_STAT_ACTIVITY)
    elif op_type == "func_pg_stat_statements":
        cur.execute(_SQL_FUNC_PG_STAT_STATEMENTS)
    elif op_type == "func_explain_statement":
        cur.execute(_SQL_FUNC_EXPLAIN_STATEMENT)
    elif "sql" in op:
        cur.execute(op["sql"])
    else:
        raise RuntimeError(f"Unknown op_type: {op_type!r}")


def _apply(ops, uri, state, optional_restart_pending=None):
    base_conn = _connect(uri)

    db_conns = {}
    failed = False

    try:
        for op in ops:
            if op.get("status") in ("skipped", "manual"):
                continue
            if op["kind"] == "MANUAL_STEP":
                op["status"] = "manual"
                continue
            if op["kind"] == "SKIP":
                op["status"] = "skipped"
                continue
            if failed:
                op["status"] = "pending"
                continue

            db = op.get("database")
            try:
                if db:
                    if db not in db_conns:
                        db_conns[db] = _connect(uri, dbname=db)
                    conn = db_conns[db]
                else:
                    conn = base_conn

                cur = conn.cursor()
                _execute_op(cur, op)

                if op["kind"] == "ALTER_SYSTEM_RELOAD":
                    base_conn.cursor().execute("SELECT pg_reload_conf()")

                op["status"] = "completed"
            except Exception as exc:
                op["status"] = "failed"
                op["error"] = str(exc)
                failed = True
    finally:
        for c in db_conns.values():
            try:
                c.close()
            except Exception:
                pass
        base_conn.close()

    return _build_result(ops, state, failed, optional_restart_pending)


def _build_result(ops, state, failed, optional_restart_pending=None):
    manual_steps = any(op["kind"] == "MANUAL_STEP" for op in ops)
    restart_needed = any(op.get("requires_restart") and op.get("status") == "completed" for op in ops) or bool(
        state.get("pending_restart")
    )
    if failed:
        outcome = "failure"
    elif manual_steps or restart_needed:
        outcome = "success_with_manual_steps"
    else:
        outcome = "success"
    return {
        "operations": ops,
        "flavor": state["flavor"],
        "pg_version": state["pg_version"],
        "restart_needed": restart_needed,
        "manual_steps": manual_steps,
        "outcome": outcome,
        "optional_restart_pending": optional_restart_pending or [],
    }


def _dry_run_result(ops, state, optional_restart_pending=None):
    for op in ops:
        if op.get("status") == "skipped" or op["kind"] == "SKIP":
            op["status"] = "skipped"
        elif op["kind"] == "MANUAL_STEP":
            op["status"] = "manual"
        else:
            op["status"] = "pending"
    manual_steps = any(op["kind"] == "MANUAL_STEP" for op in ops)
    return {
        "operations": ops,
        "flavor": state["flavor"],
        "pg_version": state["pg_version"],
        "restart_needed": False,
        "manual_steps": manual_steps,
        "outcome": "dry_run",
        "optional_restart_pending": optional_restart_pending or [],
    }


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------


def main():
    raw = sys.stdin.read()
    args = json.loads(raw)
    uri = args["connection_uri"]
    config = args["config"]

    if _IMPORT_ERROR is not None:
        print(
            json.dumps(
                {
                    "success": False,
                    "error": "psycopg (v3) is not available in this Python environment; "
                    "ensure the Datadog Postgres integration is installed ({})".format(_IMPORT_ERROR),
                }
            )
        )
        sys.exit(1)

    try:
        conn = _connect(uri)
        cur = conn.cursor()

        state = _detect(cur, config)
        conn.close()

        if not state["user_exists"] and not config.get("datadog_password"):
            raise RuntimeError(
                f"--datadog-password is required when creating user {config['datadog_user']!r} for the first time"
            )

        ops, optional_restart_pending = _plan(state, config)

        if config.get("dry_run"):
            result = _dry_run_result(ops, state, optional_restart_pending)
        else:
            result = _apply(ops, uri, state, optional_restart_pending)

        print(json.dumps({"success": True, "result": result}))
    except Exception as exc:
        print(json.dumps({"success": False, "error": str(exc)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
