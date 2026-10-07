rule shubstealer_wallet_asar {
    meta:
        author      = "Lenny-3BO"
        description = "Detects SHubStealer stage-3 trojanized wallet ASAR bundles by injection C2 and build markers"
        family      = "osx.shubstealer"
        tlp         = "WHITE"
        date        = "2026-04-17"
        yarahub_reference_md5 = "bbdb36b91b730e0f4f7532cff6434120"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_uuid = "4b347e79-ee70-4f08-a101-f0ccc8b838d5"

    strings:
        $inj_endpoint  = "wallets-gate.io/api/injection" ascii
        $bip39         = "bip39_stealer" ascii
        $build_id      = "BUILD_ID" ascii
        $api_key_pat   = /apiKey[^"]{0,10}"[a-f0-9]{32}/ ascii
        $atom_template = "NSPrincipalClass" ascii wide
        $atom_value    = "AtomApplication" ascii wide
        $pewwee_gate   = "pewweepor092" ascii

    condition:
        filesize > 1MB and filesize < 60MB and
        (
            $inj_endpoint or
            ($bip39 and $build_id) or
            ($atom_template and $atom_value and ($api_key_pat or $build_id)) or
            ($pewwee_gate and ($bip39 or $inj_endpoint))
        )
}
