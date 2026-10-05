rule elf_rekoobe_b3_06c9 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-09-02"
        description               = "detects the Rekoobe Linux backdoor"
        hash1_md5                 = "55ab7e652976d25997875f678c935de7"
        hash1_sha1                = "dc6beb5019ee21ab207c146ece5080d00f20a103"
        hash1_sha256              = "a89ebd7157336141eb14ed9084491cc5bdfce103b4db065e433dff47a1803731"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "55ab7e652976d25997875f678c935de7"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "06c95657-8897-443c-bc8e-f0f5cf6cf055"

    strings:
        $sha_1  = {01 23 45 67 [0-10] 89 AB CD EF [0-10] FE DC BA 98 [0-10] 76 54 32 10 [0-10] F0 E1 D2 C3}

        $hmac_1 = {36 36 36 36 36 36 36 36}
        $hmac_2 = {5C 5C 5C 5C 5C 5C 5C 5C}

        $str_term_1  = {C6 00 54}
        $str_term_2  = {C6 40 03 4D}
        $str_term_3  = {C6 40 01 45}
        $str_term_4  = {C6 40 04 3D}
        $str_term_5  = {C6 40 02 52}
        $str_term_6  = {C6 40 02 52}

        $str_histfile_1 = {C6 00 48}
        $str_histfile_2 = {C6 40 05 49}
        $str_histfile_3 = {C6 40 01 49}
        $str_histfile_4 = {C6 40 06 4C}
        $str_histfile_5 = {C6 40 02 53}
        $str_histfile_6 = {C6 40 07 45}
        $str_histfile_7 = {C6 40 03 54}
        $str_histfile_8 = {C6 40 08 3D}
        $str_histfile_9 = {C6 40 04 46}

    condition:
        uint32(0) == 0x464C457F and
        (
            all of them
        )
}


