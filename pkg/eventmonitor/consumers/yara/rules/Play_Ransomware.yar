rule Play_Ransomware
{
    meta:
		description = "Detects Play Ransomware"
		author = "Mickaël Walter (I-Tracing)"
		date = "2022-07-04"
        yarahub_reference_md5 = "0ba1d5a26f15f5f7942d0435fa63947e"
        yarahub_uuid = "3dad72db-1b26-42e9-93aa-403b132d956b"
        yarahub_license = "CC BY-SA 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

	strings:
		$a1 = "OpaqueKeyBlob" wide
		$b1 = { 83 c1 01 ba 01 00 00 00 d3 e2 f7 d2 8b 45 18 03 45 fc 0f be 08 23 ca 8b 55 18 03 55 fc 88 0a } // Extract of deobfuscation code
		$b2 = { 8b 4d f4 83 c1 01 ba 01 00 00 00 d3 e2 f7 d2 8b 45 f8 03 45 fc 0f be 08 23 ca 8b 55 f8 03 55 fc 88 0a } // Another extract

    condition:
        uint16(0) == 0x5a4d and 2 of ($a1, $b1, $b2) and filesize < 200KB
}