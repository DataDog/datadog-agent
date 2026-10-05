rule ANDROID_Sample_Unique_5be608a5 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "5be608a563a02c94dbf3b1812b3aa0685a6f8228b5dc74c773cc3c912adef7ba.apk"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "a7044958-a248-4aa1-ae99-e71358d82aa1"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "63a449ad781b0cd273f9ecb6509319a9"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "M12,4.5C7,4.5 2.73,7.61 1,12c1.73,4.39 6,7.5 11,7.5s9.27,-3.11 11,-7.5c-1.73,-4.39 -6,-7.5 -11,-7.5zM12,17c-2.76,0 -5,-2.24 -5,-5s2.24,-5 5,-5 5,2.24 5,5 -2.24,5 -5,5zM12,9c-1.66,0 -3,1.34 -3,3s1.34,3 3,3 3,-1.34 3,-3 -1.34,-3 -3,-3z" ascii wide
        $b = "eeM23,7H9C7.9,7,7,7.9,7,9v14c0,1.1,0.9,2,2,2h14c1.1,0,2-0.9,2-2V9C25,7.9,24.1,7,23,7z M23,23H9V9h14V23z" ascii wide
    condition:
        all of them
}
