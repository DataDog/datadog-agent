rule Hunt_ManageEngine_UEMS_Agent_Bundle {
    meta:
        description = "Detects archives, installers, or files containing ManageEngine UEMS Agent and Remote Control bundled components"
        author = "Serhii Kocherhan"
        date = "2026-09-05"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "428f029a-e4e6-43bc-9630-29599f78252a"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "190579e1fc46b6d62ac7f995572b1e0b"

    strings:
        // Bundled Filenames (ASCII & UTF-16 Wide string forms)
        $file_msi    = "UEMSAgent.msi" ascii wide nocase
        $file_ca1    = "DMRootCA.crt" ascii wide nocase
        $file_mst    = "UEMSAgent.mst" ascii wide nocase
        $file_json   = "DCAgentServerInfo.json" ascii wide nocase
        $file_ca2    = "DMRootCA-Server.crt" ascii wide nocase

        // Optional Product / Service Reference String
        $prod_name   = "ManageEngine" ascii wide nocase

    condition:
        filesize < 200MB and
        (
            // Primary Match: At least 3 of the standard bundle filenames present
            3 of ($file_*)
            or
            // Secondary Match: Specific combination of key configuration JSON and Root CA
            (
                $file_json and 1 of ($file_ca1, $file_ca2) and $prod_name
            )
        )
}