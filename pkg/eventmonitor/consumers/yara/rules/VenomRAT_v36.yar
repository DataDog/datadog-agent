rule VenomRAT_v36
{
    meta:
        id = "Cpu4cELWET4LHVKv8Kznwi"
        fingerprint = "c9dff9fdd0b06e593081f383d934dc57c00ab2390ac29f6065e2800b91732b64"
        version = "1.0"
        date = "2026-03-31"
        modified = "2026-03-31"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "VenomRAT v3.6 (dcRAT/qwqdanchun fork) -- AMSI/ETW bypass, plugin loader, process kill list"
        category = "MALWARE"
        malware = "VENOMRAT"
        mitre_att = "T1219"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "VenomRAT v3.6 dcRAT fork detected."
        yarahub_uuid = "4719b9d9-7ea5-4c2f-8dab-61620ea50f4a"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a1dfce8e37a7f1a4ef5c722049521352"

    strings:
        // VenomRAT-specific underscore command names (differ from AsyncRAT)
        $cmd_plugin = "plu_gin" ascii wide
        $cmd_save = "save_Plugin" ascii wide

        // dcRAT lineage -- TLS cert issuer baked into all builds
        $dcrat_salt = "DcRatByqwqdanchun" ascii wide

        // AMSI bypass target
        $amsi = "AmsiScanBuffer" ascii wide

        // ETW bypass target
        $etw = "EtwEventWrite" ascii wide

        // Anti-analysis class name unique to this builder
        $anti = "Anti_Analysis" ascii wide

        // Settings field names with underscores (VenomRAT-specific naming)
        $cfg_ports = "Por_ts" ascii wide
        $cfg_hosts = "Hos_ts" ascii wide

        // Obfuscated class name unique to this build
        $class_meth = "Mesth4ods" ascii wide

        // Process kill list targeting security tools
        $kill_ph = "ProcessHacker" ascii wide
        $kill_mpux = "MpUXSrv" ascii wide
        $kill_csp = "ConfigSecurityPolicy" ascii wide

    condition:
        uint16(0) == 0x5A4D
        and filesize > 30KB and filesize < 500KB
        and $dcrat_salt
        and ($cmd_plugin or $cmd_save)
        and ($amsi or $etw)
        and 2 of ($cfg_ports, $cfg_hosts, $class_meth, $anti)
        and 1 of ($kill_ph, $kill_mpux, $kill_csp)
}
