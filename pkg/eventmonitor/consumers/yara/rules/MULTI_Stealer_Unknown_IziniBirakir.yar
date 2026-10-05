rule MULTI_Stealer_Unknown_IziniBirakir {
    meta:
        description = "JAR/ZIP obfusque : package racine turc 'izini_birakir_arkasinda_geceler' + classes I/l randomisees (stealer a confirmer)"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "Veille 2026-09-17 / 2 soumissions (35 Mo chacune)"
        confidence  = "medium"
        note        = "FAMILLE NAISSANTE : 1 soumission hier, 2 aujourd'hui. Verifier la charge en sandbox."
        yarahub_uuid              = "16593828-be9c-4d2a-b5b7-d209b66d3351"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "8114450fe3f5a7121b971779de12179e"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $zip = { 50 4B 03 04 }
        $pkg = "izini_birakir_arkasinda_geceler/" ascii wide
    condition:
        $zip and $pkg
}
