rule botnet_unknown {
    meta:
        author = "NDA0E"
        date = "2024-07-22"
	description = "unknown botnet"
        yarahub_uuid = "244e449d-005a-4ecb-8db4-2c7517c094f7"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6dde652b28f73f978e834412b835a740"
    strings:
	$j = "jay is a faggot" ascii
	$a = "add illuminate#0038 for gay sex" ascii
	$p = "pls dont patch this pls dont patch this pls dont patch this" ascii
	$d = "discord dot gg slash bddHzGgKG7" ascii
	$L = "Lb32N7BOTNETYt4WLWrWnrm0iqhijcu2N7zTH8iGFqb65w62U6RNnyikqB6Yi4PJb32TP5uQVyQRMrRMzjRB7rTPVyQR8iGFF" ascii
	$h = "All hail Hitler!" ascii
    condition: 
	uint16(0) == 0x457f and any of them
}