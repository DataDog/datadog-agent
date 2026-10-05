rule Detect_PyInstaller : PyInstaller
{
    meta:
        author                       = "Obscurity Labs LLC"
        description                  = "Detects PyInstaller compiled executables across platforms"
        date                         = "2025-06-06"
        version                      = "1.0.0"
        yarahub_author_twitter       = "@obscuritylabs"
        yarahub_reference_md5        = "8f1c4e4a402a65f0fbe470ba0bd58bdd"
        yarahub_uuid                 = "90626ac0-e544-4d5b-b8c3-e70a7feb2b73"
        yarahub_license              = "CC BY-NC 4.0"
        yarahub_rule_matching_tlp    = "TLP:WHITE"
        yarahub_rule_sharing_tlp     = "TLP:WHITE"
        mitre_attack_tactic          = "T1027.002"
        mitre_attack_technique       = "TA0005"

    strings:
        // PyInstaller specific strings
        // _MEIPASS2 dates back to 2005 and seems to be cross-platform
        // https://github.com/pyinstaller/pyinstaller/blob/v1.0/source/windows/winmain.c#L123
        $meipass2      = "_MEIPASS2"         nocase wide ascii
        $pyi_runtime   = "PyInstaller"       nocase wide ascii
        
        // Common PyInstaller file patterns
        $pyi_prefix   = "pyi-"               nocase wide ascii
        $pyi_prexix2  = "Py_"               nocase wide ascii
        
        // Python runtime indicators
        $python_dll   = "python"             nocase wide ascii
        $py_import    = "PyImport_"          nocase wide ascii

    condition:
        // Must have at least one PyInstaller specific string
        any of ($meipass2, $pyi_runtime) and
        // And at least one of the following
        (
            // Either PyInstaller archive/manifest
            any of ($pyi_prefix, $pyi_prexix2)
            or
            // Or Python runtime indicators
            (any of ($python_dll) and any of ($py_import))
        )
} 