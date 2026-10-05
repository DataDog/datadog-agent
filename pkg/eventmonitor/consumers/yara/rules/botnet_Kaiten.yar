rule botnet_Kaiten {
    meta:
        author = "NDA0E"
        date = "2024-07-22"
	description = "Kaiten botnet"
        yarahub_uuid = "fb12c1fb-e14d-48b4-ac9c-995d3b263be2"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6dde652b28f73f978e834412b835a740"
    strings:
	$KaitenBotnet = "KaitenBotnet" ascii
    condition: 
	uint16(0) == 0x457f and all of them
}