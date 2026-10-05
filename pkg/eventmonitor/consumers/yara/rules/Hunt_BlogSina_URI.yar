rule Hunt_BlogSina_URI {
    meta:
        description = "Detects files containing URI targeting blog.sina.com.cn profile"
        author = "Serhii Kocherhan"
        date = "2026-08-16"
        yarahub_uuid = "7619241c-2288-4bdc-8da3-ac79bf7f7d31"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "36e3fb5964d663272cf1169e1e1ca478"

    strings:
        $target_uri = "blog.sina.com.cn/u/5655029807" ascii wide nocase

    condition:
        $target_uri
}