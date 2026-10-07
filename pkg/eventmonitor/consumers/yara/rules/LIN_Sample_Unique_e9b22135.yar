rule LIN_Sample_Unique_e9b22135 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "e9b221359e680cc0798333cab47d186db40f408b07b3e777921d370f43fab131.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "e86d2259-5e1d-4eb8-a67c-e89d8a218aa3"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "44eacf430da2cd8c7454a7e6df900de7"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "#  |=================================================================================|" ascii wide
        $b = " Autoscript AIO By FN Project                                                    |" ascii wide
    condition:
        all of them
}
