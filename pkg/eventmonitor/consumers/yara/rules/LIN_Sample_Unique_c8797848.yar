rule LIN_Sample_Unique_c8797848 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "c879784871a623d448c76ee8049ba269b24a66ac1da9c9ac534137489bb8bf26.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "fc836c35-2bf4-45df-9d4f-eab82f8f2733"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "6167aae96f3a45ecbda8f72dd43fd5b4"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "cp /usr/lib/x86_64-linux-gnu/openvpn/plugins/openvpn-plugin-auth-pam.so /usr/lib/openvpn/openvpn-plugin-auth-pam.so" ascii wide
        $b = "# Copy config OpenVPN client ke home directory root agar mudah didownload ( TCP 1194 )" ascii wide
    condition:
        all of them
}
