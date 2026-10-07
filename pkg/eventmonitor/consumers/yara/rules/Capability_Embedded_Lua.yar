rule Capability_Embedded_Lua : Embedded_Lua
{
    meta:
        author                       = "Obscurity Labs LLC"
        description                  = "Detects embedded Lua engines by looking for multiple Lua API symbols or env-var hooks"
        date                         = "2025-06-07"
        version                      = "1.0.0"
        yarahub_author_twitter       = "@obscuritylabs"
        yarahub_reference_md5        = "73e7e49b0803fc996e313e5284e103a6"
        yarahub_uuid                 = "90626ac0-e544-4d5b-b8c3-e70a7feb2b74"
        yarahub_license              = "CC BY-NC 4.0"
        yarahub_rule_matching_tlp    = "TLP:WHITE"
        yarahub_rule_sharing_tlp     = "TLP:WHITE"
        mitre_attack_tactic          = "TA0002"
        mitre_attack_technique       = "T1059.011"

    strings:
        // standard Lua environment-variable hooks
        $s_init      = "LUA_INIT"          ascii nocase
        $s_path      = "LUA_PATH"          ascii nocase

        // core Lua C-API functions / library loaders
        $a_new       = "luaL_newstate"     ascii nocase
        $a_openlibs  = "luaL_openlibs"     ascii nocase
        $a_loadbuf   = "luaL_loadbuffer"   ascii nocase
        $a_pcall     = "lua_pcall"         ascii nocase

        // any of the standard module openers, e.g. luaopen_base, luaopen_table, etc.
        $a_openmod   = /luaopen_[A-Za-z0-9_]+/ ascii nocase

    condition:
        // either an env-var hook...
        any of ($s_init, $s_path)

        // ... or a module-open pattern...
        or any of ($a_openmod)

        // ... or at least two core API functions (indicating they actually embed & use Lua)
        or (2 of ($a_new, $a_openlibs, $a_loadbuf, $a_pcall))
} 