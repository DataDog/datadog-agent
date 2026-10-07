rule win_bitcoin_genesis_b9_ce9f {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-07-22"
        description               = "detects a downloader with a DGA based on the Bitcoin Genesis Block"
        hash_md5                  = "5c13ee5dbe45d02ed74ef101b2e82ae6"
        hash_sha1                 = "bdc36bc233675e7a96faa2c4917e9b756cc2a2a0"
        hash_sha256               = "ad1e39076212d8d58ff45d1e24d681fe0c600304bd20388cddcf9182b1d28c2f"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "5c13ee5dbe45d02ed74ef101b2e82ae6"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "ce9f9e49-464a-489b-90fb-d4c81e98e360"

    strings:
        $str_json_1 = "\"bytes\": ["
        $str_json_2 = "\"subtype\": "
        $str_json_3 = "{\"bytes\":["
        $str_json_4 = "],\"subtype\":"
        $str_json_5 = "null}"
        $str_json_6 = "<discarded>"
        $str_json_7 = "[json.exception."

        /*
            mov     dl, [ebp+var_14]
            mov     [eax+ecx], dl
            mov     byte ptr [eax+ecx+1], 0
            jmp     short loc_3CBF9F
        */
        $split_hash_1 = {8A 55 ?? 88 14 08 C6 44 08 01 00 EB}
        /*
            inc     ebx
            cmp     ebx, 10h
            jl      loc_3CBF10
        */
        $split_hash_2 = {43 83 FB 10 0F 8C}

        /*
            push    0
            push    0
            mov     [ebp-14h], edx
            mov     [ebp-18h], eax
        */
        $format_the_date = {6A 00 6A 00 89 55 EC 89 45 E8}

    condition:
        uint16(0) == 0x5A4D and
        all of ($str_json_*) and
        all of ($split_hash_*) and
        $format_the_date
}
