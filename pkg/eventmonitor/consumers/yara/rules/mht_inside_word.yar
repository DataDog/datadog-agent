rule mht_inside_word{
	meta:
		author = "dPhish"
		description = "Detect embedded mht files inside microsfot word."
		date = "2025-07-28"
		yarahub_reference_md5 = "24E5E160DB26CD18ED094F9514BB8688"
		yarahub_uuid = "3ee65036-6000-423c-b7e2-bfde20e7494a"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
	strings:
	    $mht = ".mht"
	condition:
        	 $mht
}