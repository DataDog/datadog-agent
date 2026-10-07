rule LIN_Sample_Unique_586960df {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "586960df8bf559ffbba600f11917a99baed4a875cb7faa5eabc060bcde67277b.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "6f2348b1-8dba-4782-9db3-a10b689c1686"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "09db349880b7db1013060d5c54c88097"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "#!/bin/bash" ascii wide
        $b = "history -c" ascii wide
    condition:
        all of them
}
