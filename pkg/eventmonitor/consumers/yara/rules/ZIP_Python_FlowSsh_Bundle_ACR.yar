rule ZIP_Python_FlowSsh_Bundle_ACR {
    meta:
        description = "ZIP bundle: embedded Python profiler/sampling suite + FlowSshNet SSH automation (ACRStealer-tagged)"
        author      = "Marjoriefort"
        date        = "2026-09-13"
        reference   = "InTheWild.0438 / ACRStealer 33c8aee0"
        confidence  = "medium"
        yarahub_uuid            = "6d53ace8-15d7-488f-95c8-648a72d5a396"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "259d4c1efa85f363a8d7c786758062ff"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $zip = { 50 4B 03 04 }
        $fs1 = "FlowSshNet_Exec.ps1"
        $fs2 = "FlowSshNet_Sftp.ps1"
        $spy = "sampling/binary_collector.pyc"
    condition:
        $zip and $fs1 and $fs2 and $spy
}
