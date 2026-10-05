rule botnet_RyM {
    meta:
        author = "NDA0E"
        date = "2024-07-22"
	description = "RyM botnet"
        yarahub_uuid = "4aaa9b2f-992f-4416-a119-5a1c4dd63b1c"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6dde652b28f73f978e834412b835a740"
    strings:
	$RyM = "RyM..." ascii
	$RyMGang = "RyMGang" ascii
    condition: 
	uint16(0) == 0x457f and any of them
}