rule SCRIPT_Sample_Unique_9d673b14 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "9d673b14260dbdf23641187f0028ceb3f4eab874bf29da2eb6a4e8bcd0c9af43.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "43bace85-579e-421a-ba10-46eff3b6b629"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "f06429c0b0b005bcffa5b8a5b5a27e1e"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "# kmod telemetry sync" ascii wide
        $b = "# kmod telemetry sync" ascii wide
    condition:
        all of them
}
