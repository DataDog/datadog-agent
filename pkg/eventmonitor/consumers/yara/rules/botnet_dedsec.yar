rule botnet_dedsec {
    meta:
        author = "NDA0E"
        date = "2024-07-22"
	description = "dedsec botnet"
        yarahub_uuid = "9f39c4f3-7329-4de7-bff9-811bb8bfc49d"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6dde652b28f73f978e834412b835a740"
    strings:
	$dedsec = "dedsecrunsyoulilassnigga" ascii
    condition: 
	uint16(0) == 0x457f and all of them
}