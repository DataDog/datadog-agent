rule LIN_Sample_Unique_7ee5f6d3 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "7ee5f6d3be8eb2b03d3b5202fbb258bfd737a25466e7c305dd3494da558aaddc.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "8767f85d-3b9c-4c6f-87ee-d1b05ece837f"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "eda9656bf2f03eacb5e8ea1ad3464c60"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "ExecStart=/usr/bin/chisell server --port 9443 --tls-key /etc/xray/xray.key --tls-cert /etc/xray/xray.crt --socks5" ascii wide
        $b = "# Membuat file service systemd untuk Chisel di port 9443 (HTTPS)" ascii wide
    condition:
        all of them
}
