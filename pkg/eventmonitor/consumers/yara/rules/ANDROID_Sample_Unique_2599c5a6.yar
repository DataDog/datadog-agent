rule ANDROID_Sample_Unique_2599c5a6 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "2599c5a6191bc208c4f0f1fcafba81b0f8d4099de6dc8d64ee7aec0b5289833e.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "a96830c5-1413-436a-a799-804b6d55c7fe"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "2c0747da107021dd2995640eef8afd37"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "bw8hu85v/dfahycjbg/xdssct/integrity.binPK" ascii wide
        $b = "bw8hu85v/dfahycjbg/xdssct/vault.binPK" ascii wide
    condition:
        all of them
}
