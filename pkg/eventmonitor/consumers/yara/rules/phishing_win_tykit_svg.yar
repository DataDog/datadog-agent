rule phishing_win_tykit_svg {
    meta:
        version = "1.0"
        description = "Detects Tykit phishing .svg"
        author = "Zara Chacha"
        source = "https://any.run/cybersecurity-blog/tykit-technical-analysis/"
        creation_date = "2025-10-23"
        yarahub_reference_md5 = "7c8b761ec97551d76198ae527c77bfb2"
        yarahub_uuid = "417db8be-478a-4445-9919-1d25ec2100bf"
        yarahub_license = "CC0 1.0" 
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $s1 = "http://www.w3.org/2000/svg"
        $s2 = "isMobile()"
        $s3 = "parseInt"
        $s4 = "charCodeAt"
        $s5 = "fromCodePoint"
        $s6 = "['\\x65', '\\x76', '\\x61', '\\x6c'].join('')"
        $s7 = "padding" nocase

    condition:
        all of them    
}
