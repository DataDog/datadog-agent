rule SCRIPT_Sample_Unique_831f9e83 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "831f9e839ba36ecf62583d362bf180b7d7cf7f66b332739dc66daf0deb82afb9.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "6faba347-991a-49ae-b442-b7c48caf4bd3"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "8463c1eed6e76110ee88fc5091131f8f"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_f2cce323fa'" ascii wide
        $b = "return 'REDIS_OK_f2cce323fa'" ascii wide
    condition:
        all of them
}
