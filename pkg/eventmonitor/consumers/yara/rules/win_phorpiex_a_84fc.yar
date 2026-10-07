rule win_phorpiex_a_84fc {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-12-13"
        description               = "detects unpacked Phorpiex samples"
        hash_md5                  = "6b6398fa7d461b09b8652ec0f8bafeb4"
        hash_sha1                 = "43bf88ea96bb4de9f4bbc66686820260033cd2d7"
        hash_sha256               = "bd2976d327a94f87c933a3632a1c56d0050b047506f5146b1a47d2b9fd5b798d"
        malpedia_family           = "win.phorpiex"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "6b6398fa7d461b09b8652ec0f8bafeb4"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "84fc2940-d204-4d75-9f17-89cce6b1dea2"

    strings:
        $str_1 = ":--tLdr--:"
        $str_2 = "T-449505056674060607" wide

        $path_1 = "\\public_html" wide
        $path_2 = "\\htdocs" wide
        $path_3 = "\\httpdocs" wide
        $path_4 = "\\wwwroot" wide
        $path_5 = "\\ftproot" wide
        $path_6 = "\\share" wide
        $path_7 = "\\income" wide
        $path_8 = "\\upload" wide

        $cmd_0 = "/c start _ & _\\DeviceManager.exe & exit" wide
        $cmd_1 = "%ls\\_\\DeviceConfigManager.exe" wide
        $cmd_2 = "%ls\\_\\DeviceManager.exe" wide
        $cmd_3 = "/c rmdir /q /s \"%ls\"" wide
        $cmd_4 = "/c move /y \"%ls\", \"%ls\"" wide

    condition:
        uint16(0) == 0x5A4D and
        all of ($str*) or
        all of ($path*) or
        all of ($cmd*)
}
