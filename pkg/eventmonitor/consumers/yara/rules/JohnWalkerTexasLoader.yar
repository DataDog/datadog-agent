rule JohnWalkerTexasLoader {
    meta:
	author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-10-25"
        description = "Detects JohnWalkerTexasLoader"
        yarahub_uuid = "75e0d918-1446-4c71-8c99-428f518c6c1b"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "f4e63c81d5af37d0ba01e00b35d950de"
    
    strings:
        $a1 = "proc=" ascii
        $a2 = "status=" ascii
        $a3 = "wallets=" ascii
        $a4 = "av=" ascii

        $w1 = "proc=" wide ascii
        $w2 = "status=" wide ascii
        $w3 = "wallets=" wide ascii
        $w4 = "av=" wide ascii
		
	$x = /sendopen.{0,1}/ ascii

    condition:
        ($x) and
	(all of ($a*) or all of ($w*)) and
	uint16(0) == 0x5a4d
}