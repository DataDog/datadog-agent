rule DetectEncryptedVariants
{
    meta:
        description = "Detects 'encrypted' in ASCII, Unicode, base64, or hex-encoded"
        author = "Zinyth"
        date = "2025-06-20"
	Description = "This rule is meant to catch different types of ransomware."
	date = "2024-09-02"
	yarahub_reference_md5 = "b0fd45162c2219e14bdccab76f33946e"
	yarahub_uuid = "0d185fc2-9c49-498e-b7ce-b28db1b9f36b"
	yarahub_license = "CC0 1.0"
	yarahub_rule_matching_tlp = "TLP:WHITE"
	yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        // Plain ASCII
        $ascii = "encrypted" nocase

        // UTF-16LE (little endian)
        $unicode_le = "e\x00n\x00c\x00r\x00y\x00p\x00t\x00e\x00d\x00" nocase

        // UTF-16BE (big endian)
        $unicode_be = "\x00e\x00n\x00c\x00r\x00y\x00p\x00t\x00e\x00d" nocase

        // Base64: 'encrypted' -> 'ZW5jcnlwdGVk'
        $base64 = "ZW5jcnlwdGVk"

        // Hex encoded as ASCII: 'encrypted' -> '656E63727970746564'
        $hex = "656E63727970746564"

    condition:
        any of them
}
