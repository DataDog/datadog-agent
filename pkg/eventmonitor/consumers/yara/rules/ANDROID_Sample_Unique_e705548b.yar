rule ANDROID_Sample_Unique_e705548b {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "e705548bb935333cc8c3094f27dd570a9e2c2c101a65321f7a3dfb37c3f9a1c6.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "13101d89-ab8c-4601-88df-10bfe3372d26"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "7b5f7422a373debc2032eb17027ece39"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:q:" ascii wide
        $b = "assets/ocr2/ocr_v1_for_cpu/ch_ppocr_mobile_v2.0_det_opt.nbPK" ascii wide
    condition:
        all of them
}
