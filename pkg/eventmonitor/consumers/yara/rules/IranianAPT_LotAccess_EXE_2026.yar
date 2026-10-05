rule IranianAPT_LotAccess_EXE_2026 {
    meta:
        description     = "LotAccess trojanized AppEx VPN client - Operation Epic Fury (Iran/Israel 2026)"
        author          = "Paolo Costanzo - paolocostanzo.github.io"
        date            = "2026-03-15"
        reference       = "https://paolocostanzo.github.io/operation-epic-fury-cyber-war-iran/"
        hash_v3_sha256  = "6209a9524e97ee8ac5fb05668f2be9a18a455870bb8cf6022049ee8f458c12d6"
        hash_v1_sha256  = "7d43d7f6c743912b74273901494ed18451aa2824130d9d405da250a9fe3aad0d"
        pe_imphash      = "d89625bf08b621847b3ab97338a84dda"
        mitre_attack    = "T1071.001, T1497.003, T1053.005, T1036.007, T1112, T1059.007"
        confidence      = "HIGH"
        yarahub_uuid              = "30d13fd1-fbc1-446a-9e1d-853a2dd55d4b"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "58dad3a41691265128c751d133d5525f"

    strings:

        $mutex          = "tqvpn-gui-keep-one-instance" ascii wide


        $c2_ip_primary  = "216.45.58.148" ascii wide


        $c2_ip_backup   = "167.160.187.43" ascii wide


        $c2_host        = "api.ra-backup.com" ascii wide


        $cfg            = "cloudvpn.cfg" ascii wide


        $appex_api_1    = "/cgi-bin/d_device_action.py?ButtonDownSSLFile" ascii wide
        $appex_api_2    = "/cgi-bin/d_device_action.py?ButtonSSLClientDownLinked" ascii wide


        $appex_brand    = "AppEx Networks" ascii wide
        $tianqin        = "TianQin" ascii wide


        $sched_task     = "Microsoft-Windows-DiskDiagnosticDataCollector" ascii wide


        $reg_key        = "AppEx Networks\\LotAccess" ascii wide

    condition:
        uint16(0) == 0x5A4D
        and filesize < 5MB
        and (
            $mutex
            or ($c2_ip_primary and $cfg)
            or ($c2_ip_backup and $cfg)
            or ($c2_host and $cfg)
            or (($appex_api_1 or $appex_api_2) and ($c2_ip_primary or $c2_ip_backup or $c2_host))
            or ($reg_key and $sched_task and ($appex_brand or $tianqin))
        )
}
