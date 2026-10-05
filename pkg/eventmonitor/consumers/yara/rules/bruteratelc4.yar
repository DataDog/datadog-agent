rule bruteratelc4 {
    meta:
        author = "spyw4re"
        description = "A Rule to detect brute ratel stager payloads."
        yarahub_author_twitter = "@CryptDeriveKey"
        date = "2023-10-06"
        yarahub_uuid = "950ced7c-f32b-4e02-a343-e2ee18b865ea"
        yarahub_reference_md5 = "2aef21ef6759026b3008e5a9a1cff67f"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.brute_ratel_c4"

    strings:
        $api_hashing = {ac 84 c0 74 07 c1 cf 0d 01 c7 eb f4}
        $push_stack = {50 68 ?? ?? ?? ??}
    
    condition:
        (uint16(0) == 0x5A4D) and all of them
}  

