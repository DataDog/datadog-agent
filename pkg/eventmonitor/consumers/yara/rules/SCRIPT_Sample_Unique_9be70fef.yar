rule SCRIPT_Sample_Unique_9be70fef {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "9be70fef03d307ec98bc9432de05df1defde780926451845b50e791bd13d74ef.vbs"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "520d3456-f931-4071-a507-b855eed02715"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "4ddf364a6c5262d34c6dc693b439c181"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Const stfaffdsasdsafffdassgfandfvggfdbiddene = \"glutfsffsffsadfnfffshageligses! vrdireduktidoners whereon.\"" ascii wide
        $b = "Call Ugfdfging(\"rcIbige0a9cSifrcIbigea6e2SifrcIbiga98efSifrcIbig2f2a0a19SifrcIbig3f7eba7SifrcIbigcfac9dSi\")" ascii wide
    condition:
        all of them
}
