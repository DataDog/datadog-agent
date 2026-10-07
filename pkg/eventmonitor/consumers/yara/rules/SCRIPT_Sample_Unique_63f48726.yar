rule SCRIPT_Sample_Unique_63f48726 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "63f487269964719de0d25060edb7170960e0e44ebe36cc42f7087e0e7c5a4642.vbs"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "790302a3-8a06-4b15-84ba-fd4b0a828362"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "576af51a2a167d8edf83149c69cd3f42"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Const stfaffdsasdsafffdassgfandfvggfdbiddene = \"glutfsffsffsadfnfffshageligses! vrdireduktidoners whereon.\"" ascii wide
        $b = "Call Ugfdfging(\"Ainf7ac97b38fgnaAinfafb78gnaAinf2ffa08b9agnaAinff8f48a86gnaAinffbb08baebgnaAinf6b7899gnaA\")" ascii wide
    condition:
        all of them
}
