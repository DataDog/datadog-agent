rule MX_fin_custom_allakore_rat {
    meta:
        author = "BlackBerry Threat Research & Intelligence Team"
        description = "Find MX fin custom function names and prefixes."
        date = "2023-12-19"
        yarahub_uuid = "1ae525ed-ef60-408c-8b61-0bec8b5a9828"
        yarahub_reference_md5 = "33cc3be935639f1e0d1d7483b8286d7c"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $main = "<|MAINSOCKET|>"
        $cnc1 = "<|MANDAFIRMA|>"
        $cnc2 = "<|FIRMASANTA|>"
        $cnc3 = "<|MENSAJE" wide
        $cnc4 = "<|DESTRABA" wide
        $cnc5 = "<|TOKEN" wide
        $cnc6 = "<|TRABAR" wide
        $cnc7 = "<|USU" wide
        $cnc8 = "<|ACTUALIZA|>" wide
        $cnc9 = "<|BANA" wide
        $cnc10 = "<|CLAVE" wide
    condition:
      uint16(0) == 0x5A4D and $main and 2 of ($cnc*) and filesize > 5MB and filesize < 12MB
}