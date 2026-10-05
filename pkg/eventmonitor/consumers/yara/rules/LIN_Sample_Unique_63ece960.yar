rule LIN_Sample_Unique_63ece960 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "63ece9606caf881a133458c95608bff9b81836ea98080fd32408e963e2984bb1.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "a6cc48f3-80be-42c0-8272-a24cafbe681b"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "9f967110dea294199e05ada0ad6de0da"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Green_font_prefix=\"\\033[32m\" && Red_font_prefix=\"\\033[31m\" && Green_background_prefix=\"\\033[42;37m\" && Red_background_prefix=\"\\033[41;37m\" && Font_color_suffix=\"\\033[0m\"" ascii wide
        $b = "#  |=================================================================================|" ascii wide
    condition:
        all of them
}
