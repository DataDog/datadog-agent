rule ANDROID_Sample_Unique_70440c6c {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "70440c6c55be6ba26a7bfdcddb03f29fd656e8f47201d350af0a19cb69d36040.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "94841d63-35d0-492d-9a3f-cb669945673c"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "17af089a49369bda4f84f0e43c5cbfce"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "assets/water/textures/gui/sprites/hud/heart/icon.png.pngPK" ascii wide
        $b = "assets/minecraft/textures/font/unicode_page_04.png]z	\\Ri" ascii wide
    condition:
        all of them
}
