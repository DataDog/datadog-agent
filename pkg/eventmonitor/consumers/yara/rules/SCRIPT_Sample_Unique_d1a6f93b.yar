rule SCRIPT_Sample_Unique_d1a6f93b {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "d1a6f93ba2ca7025ff4c173da66ee4a59665e966667567a71c47829820cb64a7.bat"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "c09903bf-acd2-42d7-be7b-3535c7852715"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "2ed69cb385a1337d5ea227cb6e5d9abc"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "if not exist \"%Higendes%\" (" ascii wide
        $b = "if not exist \"%Higendes%\" (" ascii wide
    condition:
        all of them
}
