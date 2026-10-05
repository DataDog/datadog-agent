rule LIN_Sample_Unique_45adf733 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "45adf73332172a1a1fcd0a182968fdf578bbc340512d6b7fa7bd18cf0c9a3e9f.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "d24dc699-6601-4b39-82ff-4b96fe18bc59"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "a9941f77e9a26acb1e258c004e2e847b"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "apt install msmtp-mta ca-certificates bsd-mailx -y" ascii wide
        $b = "chown -R www-data:www-data /etc/msmtprc" ascii wide
    condition:
        all of them
}
