rule SCRIPT_Sample_Unique_629a4247 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "629a4247fa8eeada99bc0325c0092e469cb14217ef071a5d11798038bd7f146e.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "333245a8-33cc-48ce-8bd6-41e203744ac2"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "98d16fb3df05787f66f224ab18e91058"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "    <AnonymousLogin Level=\"40/40\" Dispatch=\"account\">DISABLE</AnonymousLogin>" ascii wide
        $b = "        <Password Level=\"40/40\" Dispatch=\"account\">heintej</Password>" ascii wide
    condition:
        all of them
}
