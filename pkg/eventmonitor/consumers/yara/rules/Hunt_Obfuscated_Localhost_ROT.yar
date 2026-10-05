rule Hunt_Obfuscated_Localhost_ROT
{
    meta:
        description = "Detects obfuscated/rotated 127.0.0.1 strings while excluding raw plaintext"
        author = "Serhii Kocherhan"
        date = "2026-08-17"
        yarahub_uuid = "1e65af43-77de-4450-8d86-5cf743d50ba7"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        // Plaintext un-encrypted string to explicitly exclude
        $plaintext = "127.0.0.1" ascii wide

        // Generates all 256 single-byte XOR / ROT variations of the string
        $rotated_localhost = "127.0.0.1" xor ascii wide

    condition:
        // Match if any obfuscated/rotated variant exists BUT the raw plaintext does NOT
        $rotated_localhost and not $plaintext
}