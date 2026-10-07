rule shubstealer_stage2_applescript {
    meta:
        author      = "Lenny-3BO"
        description = "Detects SHubStealer stage-2 AppleScript payload by C2 endpoints and capability markers"
        family      = "osx.shubstealer"
        tlp         = "WHITE"
        date        = "2026-04-17"
        hash        = "60d53557746e266b1758d855a4d95df99c4a02a67b7fd2eb10300e5bfdab8d89"
        yarahub_reference_md5 = "0222f80483f64a80bd1898307495c9a8"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_uuid = "a9003727-8c9c-4e0e-b3a2-3190585a7449"

    strings:
        $c2_domain     = "pewweepor092" ascii
        $dscl_auth     = "dscl . authonly" ascii
        $nsprincipal   = "NSPrincipalClass" ascii
        $shub_prefix   = /shub_[a-z]{3,12}/ ascii
        $ditto_cmd     = "ditto" ascii
        $debug_event   = "api/debug/event" ascii
        $gate_chunk    = "/gate/chunk" ascii

    condition:
        filesize < 5MB and
        $c2_domain and
        (
            ($dscl_auth and $nsprincipal) or
            ($gate_chunk and $debug_event) or
            ($shub_prefix and $ditto_cmd and $gate_chunk)
        )
}
