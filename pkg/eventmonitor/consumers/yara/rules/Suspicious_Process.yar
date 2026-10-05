rule Suspicious_Process {
    meta:
        description = "Suspicious process creation"
        author = "Security Research Team"
        date = "2024-11-27"
        yarahub_uuid = "0fcee061-50c2-404d-9854-c67b78287c64"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d41d8cd98f00b204e9800998ecf8427e"
        license = "MIT"
        threat_type = "Suspicious Behavior"
        severity = "Medium"
        
    strings:
        $proc1 = "svchost.exe" nocase
        $proc2 = "rundll32.exe" nocase
        $proc3 = "powershell.exe" nocase
        $arg1 = "-enc" nocase
        $arg2 = "/c" nocase
        $net1 = "http://" nocase
        
    condition:
        (
            ($proc1 or $proc2 or $proc3) and
            ($arg1 or $arg2) and
            ($net1)
        )
}
