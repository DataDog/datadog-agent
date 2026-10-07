rule MAL_JS_Gootloader_jQuery_Compactv2_17Dec24 {
	meta:		
		description = "Detects malicious Gootloader JS hidden in the Query Compat JavaScript Library v3.0.0-alpha1"
		author = "@Gootloader"
		date = "2024-12-17"
		tlp = "CLEAR"
		yarahub_reference_md5 = "95238ad5a91d721c6e8fdf4c36187798"
		yarahub_uuid = "7330bdd3-38ae-437d-bcc8-d750f2363048"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		malpedia_family = "js.gootloader"
	strings:		
		$string1 = "jQuery Compat JavaScript Library v3.0.0-alpha1"
		$string2 = "');"
		
	condition:
		#string1 >= 1
		and #string2 >= 1
		and all of them
}