rule IranianAPT_LotAccess_Family {
    meta:
        description     = "AppEx Networks VPN client trojanized family -- any C2 (Operation Epic Fury variants)"
        author          = "Paolo Costanzo - paolocostanzo.github.io"
        date            = "2026-03-15"
        reference       = "https://paolocostanzo.github.io/operation-epic-fury-cyber-war-iran/"
        confidence      = "MEDIUM -- possible false positives on legitimate AppEx VPN installs"
        mitre_attack    = "T1071.001, T1497.003, T1053.005, T1036.007, T1112"
        yarahub_uuid              = "d96876ae-e64b-40d3-905f-6321a16d8277"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "58dad3a41691265128c751d133d5525f"

    strings:
        $mutex          = "tqvpn-gui-keep-one-instance" ascii wide
        $cfg            = "cloudvpn.cfg" ascii wide
        $appex_api      = "/cgi-bin/d_device_action.py" ascii wide
        $sched_task     = "Microsoft-Windows-DiskDiagnosticDataCollector" ascii wide
        $reg_key        = "AppEx Networks\\LotAccess" ascii wide

    condition:
        uint16(0) == 0x5A4D
        and filesize < 5MB
        and 3 of them
}
