rule BAT_DbatLoader {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-07-25"
        description = "Detects base64 and hex encoded MZ header used by DbatLoader"
        yarahub_uuid = "0ebcf373-d592-4d54-9eec-bbd15f4958e9"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a7ecf2d80475a31c10bfdddd8c060548"
        malpedia_family = "win.dbatloader"

    strings:
        $x509_crl_begin = "-----BEGIN X509 CRL-----" ascii
        $mz = "NGQ1YTUwMDAwMjAwMDAwMDA0MDAwZjAwZmZmZjAwMDBiODAwMDAwMDAwMDAwMDAw" ascii //base64 and hex encoded MZ header
        $x509_crl_end = "-----END X509 CRL-----" ascii
    condition: 
        all of them
}