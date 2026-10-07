rule ANDROID_Sample_Unique_f13e706c {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "f13e706c994d5daa41e16e921a07be037a92837ca7eb0f4755370e1712b92fff.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "96c30de5-126a-48d6-80e0-e96e0b653eb5"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "04ecc9d6602afcbc6d7052d745f918a8"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "assets/fakeclient/textures/font/media_camera.pngPK" ascii wide
        $b = "44f19907-2c1c-4147-a98c-9fc6b765cfca.binPK" ascii wide
    condition:
        all of them
}
