rule SCRIPT_Sample_Unique_fab631b0 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "fab631b04f4b27fb17c4afaa455b22b2146a4e2359b0b1039f2653f880b7c2f0.vbs"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "878cd507-8cd0-4bec-8494-eef678682e29"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "cbc2009e7ab0b1ffda098764b7d81c02"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Const stfaffdsasdsafffdassgfandfvggfdbiddene = \"glutfsffsffsadfnfffshageligses! vrdireduktidoners whereon.\"" ascii wide
        $b = "Call Ugfdfging(\"dpfnIIa7afeb91f5ddpfnIIbabcbba7e3ddpfnIIe9f280ddpfnIIabfa8ddpfnIIab68da3fddpfnIIef19ec384\")" ascii wide
    condition:
        all of them
}
