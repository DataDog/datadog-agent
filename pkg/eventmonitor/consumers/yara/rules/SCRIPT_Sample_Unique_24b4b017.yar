rule SCRIPT_Sample_Unique_24b4b017 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "24b4b0172716d9bbacae35f12c121c1da6dac0dc1208de30dde037c6d3e849ae.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "075ef117-6d66-4e74-b384-7bc7cf38211b"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "5e64ee852833a04b57055dea0cb75178"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_23e2e4f2ab'" ascii wide
        $b = "return 'REDIS_OK_23e2e4f2ab'" ascii wide
    condition:
        all of them
}
