rule MAC_Sample_Unique_841f0ccf {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "841f0ccf4e6e782476a0844cc861f7f44d7f321bc232444ef5f306fdba8944eb.macho"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "2e6936f8-0419-4d35-a2ea-37bd7f88252b"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "3d000465564399a53dfd52700c028fc7"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "setup-6aa79328f6164e3659e037ecd971062e6f874279" ascii wide
        $b = "/usr/lib/libSystem.B.dylib" ascii wide
    condition:
        all of them
}
