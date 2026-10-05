rule SCRIPT_Sample_Unique_7652f213 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "7652f213a448eacbe2a75594ecd99aa497284f27bb603ba38fdf5dad30746c81.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "ec957d61-8948-433a-be71-f2c987acf951"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "72242dfddd5d7bb2586420407823214d"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "                Please forward this error screen to 130.12.180.80's <a href=\"mailto:root@130-12-180-148.cprapid.com?subject=Error message [404] (none) for 45.55.193.40 requesting 130.12.180.80 port 80 on Monday, 14-Sep-2026 21:15:23 UTC\"> WebMaster</a>." ascii wide
        $b = "                            <img src=\"/img-sys/server_misconfigured.png\" class=\"info-image\" />" ascii wide
    condition:
        all of them
}
