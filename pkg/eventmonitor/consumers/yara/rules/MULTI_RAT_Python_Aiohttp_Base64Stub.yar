rule MULTI_RAT_Python_Aiohttp_Base64Stub {
    meta:
        description = "RAT Python asynchrone aiohttp embarque dans un stub base64 : CLIENT_CODE/EXECUTABLE_PATH, commandes shell_command/file_data"
        author      = "Marjoriefort"
        yarahub_reference_md5 = "0ea83b8466201809092e74150e7a7e40"
        date        = "2026-09-19"
        reference   = "Veille glissante-2026-09-19_08 / 0a813de9.py"
        confidence  = "high"
        yarahub_uuid = "e74c2d2e-42e1-4abc-a104-780b0d53b864"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "CLIENT_CODE = base64.b64decode(" ascii
        $b = "EXECUTABLE_PATH = sys.executable" ascii
        $c = "X0g9J0NvbW1hbmQgaXMgcmVxdWlyZWQuJwpfRz0nc2hlbGxfY29tbWFuZCcK" ascii
    condition:
        ($a and $b) or ($a and $c)
}
