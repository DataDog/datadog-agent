rule SCRIPT_Sample_Unique_d3d877e5 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "d3d877e529792a6e07756c840fd62598599f096ae7d4c296f5f5025b4501050d.vbs"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "1dd93c03-a68d-4614-bbc7-d5e59a0a7566"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "871d6eab90f50428b74f6e289dfb4925"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "' A contiguous memory block for clean cups has been requested via aligned_alloc for cache efficiency" ascii wide
        $b = "' Steam boiler pressure is currently being checked and verified against the safety thresholds" ascii wide
    condition:
        all of them
}
