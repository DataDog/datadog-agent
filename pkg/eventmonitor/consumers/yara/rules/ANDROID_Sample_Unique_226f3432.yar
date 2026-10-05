rule ANDROID_Sample_Unique_226f3432 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "226f34325652ddba2e1afb897fe78fa4626687ce9c0ae1f9458b0100b53fdbe9.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "1270dfee-4ba7-4374-9a73-638795e6bdcf"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "9c036201694b6a095db111bf2d1088c3"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "assets/zenya/textures/gui/sprites/hud/heart/absorption_half.pngPK" ascii wide
        $b = "assets/zenya/textures/gui/category/star-plus-yellow.png.mcmetaPK" ascii wide
    condition:
        all of them
}
