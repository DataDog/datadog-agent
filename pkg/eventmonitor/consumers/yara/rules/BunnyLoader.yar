rule BunnyLoader {
	meta: 
		author = "indest"
		description = "generic crypto/card stealer rule"
		date = "2025-12-06"
		yarahub_uuid = "daf0e396-f224-412c-84fe-129669c1b662"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "dbf727e1effc3631ae634d95a0d88bf3"
		sha256 = "454bd68088f17718527b300134cae3eed1c7db3ba7ed9e08d291ef7729229a79"
		
		
	strings:
		
		$wallet_1 = "\\Exodus\\exodus.wallet"
		$wallet_2 = "\\Ethereum\\keystore"
		$wallet_3 = "\\Electrum\\wallets"
		$wallet_4 = "\\Coinomi\\wallets"
		
		$database_1 = "\\com.liberty.jaxx\\IndexedDB\\file__0.indexeddb.leveldb"
		$database_2 = "\\atomic\\Local Storage\\leveldb"
		$database_3 = "\\Guarda\\Local Storage\\leveldb"

        $card_1  = "3[47][0-9]{13}"
        $card_2  = "(6541|6556)[0-9]{12}"
        $card_3  = "389[0-9]{11}"
        $card_4  = "3(?:0[0-5]|[68][0-9])[0-9]{11}"
        $card_5  = "65[4-9][0-9]{13}|64[4-9][0-9]{13}|6011[0-9]{12}|622(12[6-9]|1[3-9][0-9]|[2-8][0-9]{2}|9[01][0-9]|92[0-5])[0-9]{10}"
        $card_6  = "63[7-9][0-9]{13}"
        $card_7  = "(?:2131|1800|35[0-9]{3})[0-9]{11}"
        $card_8  = "9[0-9]{15}"
        $card_9  = "(6304|6706|6709|6771)[0-9]{12,15}"
        $card_10 = "(5018|5020|5038|6304|6759|6761|6763)[0-9]{8,15}"
        $card_11 = "(5[1-5][0-9]{14}|2(22[1-9][0-9]{12}|2[3-9][0-9]{13}|[3-6][0-9]{14}|7[01][0-9]{13}|720[0-9]{12}))"
        $card_12 = "(6334|6767)[0-9]{12,15}"
        $card_13 = "(4903|4905|4911|4936|6333|6759)[0-9]{12,15}|564182[0-9]{10,13}|633110[0-9]{10,13}"
        $card_14 = "62[0-9]{14,17}"
        $card_15 = "4[0-9]{12}(?:[0-9]{3})?"
        $card_16 = "(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14})"
	
	condition:
		uint16(0) == 0x5A4D and
		(
		2 of ($wallet_*) or
		2 of ($database_*) or 
		2 of ($card_*)
		)
}