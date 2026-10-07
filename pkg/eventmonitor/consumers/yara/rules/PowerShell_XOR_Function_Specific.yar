rule PowerShell_XOR_Function_Specific
{
    meta:
        description = "Detects a specific PowerShell function that performs XOR encoding and decoding."
        author = "Gemini"
        date = "2025-08-29"
        reference = "Internal Research"
	yarahub_reference_md5 = "598fda378d66cc1b703b4e2f4790ae98"
	yarahub_uuid = "5615f3a5-95cf-477c-982f-6105289d27e3"
	yarahub_license = "CC0 1.0"
	yarahub_rule_matching_tlp = "TLP:WHITE"
	yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        // Function definition and parameters
        $func = "function xor{ param($string, $method, $key)" ascii wide

        // Key operations
        $op_bxor = "-bxor $xorkey" ascii wide
        $op_b64_decode = "[System.Convert]::FromBase64String($string)" ascii wide
        $op_b64_encode = "[System.Convert]::ToBase64String($xordData)" ascii wide
        $op_replace = "-replace '/', '_'" ascii wide
        $op_encoding = "[System.Text.Encoding]::UTF8" ascii wide

    condition:
        // A high-confidence match requires the function definition, the core XOR operation,
        // and at least two other characteristic operations.
        $func and $op_bxor and 2 of ($op_b64_decode, $op_b64_encode, $op_replace, $op_encoding)
}
