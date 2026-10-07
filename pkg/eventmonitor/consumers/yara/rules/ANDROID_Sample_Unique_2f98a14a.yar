rule ANDROID_Sample_Unique_2f98a14a {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "2f98a14a0acb19957e7c55647f337b5a1d1c1ef2307423801e2874ba1550cf38.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "b5c4e52a-aea9-46b9-880a-dac7882bfe0f"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "4fce39f3e2ecb719ceede38e1f5ac317"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "assets/zyron-client/textures/sky/sky_art_black_dress.png.mcmetaPK" ascii wide
        $b = "assets/zyron-client/textures/sky/backgrounds/dramatic_day.pngTze@" ascii wide
    condition:
        all of them
}
