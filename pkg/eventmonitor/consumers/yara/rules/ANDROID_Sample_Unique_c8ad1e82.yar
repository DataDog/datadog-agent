rule ANDROID_Sample_Unique_c8ad1e82 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "c8ad1e829c66eba78b31e75bb02c3f8c9f1838a6400cb77288e0059a3ac9ac04.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "85921663-0120-423e-90ed-467bd78169ac"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "7e4ea41025b7aac021c0671f3c2fa3c7"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "assets/water/textures/gui/sprites/hud/heart/icon.png.pngPK" ascii wide
        $b = "assets/water/textures/gui/sprites/hud/heart/icon.png.png" ascii wide
    condition:
        all of them
}
