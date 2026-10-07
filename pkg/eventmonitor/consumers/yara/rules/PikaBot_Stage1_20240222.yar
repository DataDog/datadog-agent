rule PikaBot_Stage1_20240222
{
	meta:
		author = "Nicholas Dhaeyer - @DhaeyerWolf"
		date_created = "2024-03-11"
		date_last_modified = "2024-03-11"
		description = "Attempts to identify common strings used in a stage 1 Pikabot maldoc. During the infection, the malicious .js file this rule attempts to detect was observed in a ZIP file."
		yarahub_uuid = "9c58db83-6b79-40f2-bb2f-14f3850306c5"
		date = "2024-03-11"
		yarahub_author_twitter = "@DhaeyerWolf"
		yarahub_license = "CC BY-SA 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "1ab44b19be472634d648de85991aefeb"
		malpedia_family = "win.pikabot"

    strings:
		$start = "$ = " //script starts with definition of a variable.
		
		$s_fromCharCode = "\\x66\\x72\\x6F\\x6D\\x43\\x68\\x61\\x72\\x43\\x6F\\x64\\x65" 	//fromCharCode
		$s_forEach = "\\x66\\x6F\\x72\\x45\\x61\\x63\\x68" 						//forEach
		$s_charAt = "\\x63\\x68\\x61\\x72\\x41\\x74" 							//charAt
		$s_split = "\\x73\\x70\\x6C\\x69\\x74" 								//split
		$s_replace = "\\x72\\x65\\x70\\x6C\\x61\\x63\\x65" 						//replace
		$s_slice = "\\x73\\x6C\\x69\\x63\\x65" 								//slice
		$s_prototype = "\\x70\\x72\\x6F\\x74\\x6F\\x74\\x79\\x70\\x65"				//prototype
		$s_call = "\\x63\\x61\\x6C\\x6C" 									//call
		$s_length = "\\x6C\\x65\\x6E\\x67\\x74\\x68" 							//length
		

    condition:
		$start at 0 and 1 of ($s_*)
}