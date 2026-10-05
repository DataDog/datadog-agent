rule SCRIPT_Sample_Unique_6daa4ed3 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "6daa4ed3529d880f511818d0f7a696598bd0ade806f23fce9324b667a2abad1e.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "28a5dcf8-b120-4fa5-8e37-82a2baac38f8"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "713919597e3d381c20acf8983694ec37"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "# kmod telemetry sync" ascii wide
        $b = "# kmod telemetry sync" ascii wide
    condition:
        all of them
}
