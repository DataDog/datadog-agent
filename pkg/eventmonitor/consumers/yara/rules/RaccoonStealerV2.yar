rule RaccoonStealerV2 {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-10-26"
        description = "Detects RecordBreaker, Raccoon Stealer 2.0"
        yarahub_uuid = "37f1293d-154f-4e0a-8ddb-b19ae4bc696c"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "24bdb92d93d301d2e58b84f4e5161909"
	malpedia_family = "win.recordbreaker"

    strings:        
        $x1 = "MachineGuid" ascii
        $x2 = "SELECT name_on_card, card_number_encrypted, expiration_month, expiration_year FROM credit_cards" ascii
        $x3 = "SELECT service, encrypted_token FROM token_service" ascii
        
        $s1 = "&configId=" ascii
        $s2 = "machineId=" ascii
        $s3 = "URL:%s" ascii
        $s4 = "USR:%s" ascii
        $s5 = "PASS:%s" ascii

	$xor_decrypt = {
		68 ?? ?? ?? ??		//PUSH       s_65c47fbc812d076b01ac0a36a19adf62_00415d88
		ff 15 ?? ?? ?? ??	//CALL       dword ptr [->KERNEL32.DLL::lstrlenA]
		8b ??			//MOV        param_1,EAX
		33 ??			//XOR        EDX,EDX
		8b ?? ??		//MOV        EAX,dword ptr [EBP + -0xc]
		f7 f? 			//DIV        param_1
		8a ??			//MOV        param_1,byte ptr [EBX]
	}

    condition:
        uint16(0) == 0x5a4d and
        (all of ($x*) or 
        all of ($s*) or 
        $xor_decrypt)
}