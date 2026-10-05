rule botnet_Vixaati {
    meta:
        author = "NDA0E"
        date = "2024-07-22"
	description = "Vixaati botnet"
        yarahub_uuid = "dfba00d2-e090-4db5-b7b8-fd0a65185cec"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6dde652b28f73f978e834412b835a740"
    strings:
	$VixaatiServices = "VixaatiServices Pain SRC runs your shit niggaaaa lol xdxdxdxd" ascii
	$Vixaati = "Vixaati" ascii
    condition: 
	uint16(0) == 0x457f and all of them
}