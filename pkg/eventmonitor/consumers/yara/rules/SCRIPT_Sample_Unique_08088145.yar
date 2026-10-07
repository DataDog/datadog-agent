rule SCRIPT_Sample_Unique_08088145 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "08088145cbe6cdf461020a955ea4a314735b4359f85577ef86865fd19f20f29c.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "9b914fd8-b14e-4833-a216-71bf296bbab6"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "77c2dfdfa9c181fcd63027b6b0f22f2b"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "53ed52aea5b36890057d77dd" ascii wide
        $b = "53ed52aea5b36890057d77dd" ascii wide
    condition:
        all of them
}
