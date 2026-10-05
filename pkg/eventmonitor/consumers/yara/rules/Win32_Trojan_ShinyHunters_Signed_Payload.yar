rule Win32_Trojan_ShinyHunters_Signed_Payload {
    meta:
        description = "Detects Windows executables signed with certificates issued to Tobias Weihmann Software Development OU via Sectigo, observed in ShinyHunters / UNC6240 campaigns."
        author = "Serhii Kocherhan"
        date = "2026-09-26"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "a4a84699-ed73-491a-a7dc-1da1389a6e17"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        reference = "https://cloud.google.com/blog/topics/threat-intelligence/shinyhunters-renewed-mass-exploitation-campaign-targeting-oracle-peoplesoft"
        yarahub_reference_md5 = "ba0ec19a36d0e08979ae38a2c8977a9c"

    strings:
        // Certificate Issuer and Subject Artifacts
        $s_publisher = "Tobias Weihmann Software Development OU" ascii wide nocase
        $s_ca = "Sectigo" ascii wide nocase

    condition:
        // Validate PE Header Magic (MZ)
        uint16(0) == 0x5A4D and
        filesize < 25MB and
        $s_publisher and
        $s_ca
}