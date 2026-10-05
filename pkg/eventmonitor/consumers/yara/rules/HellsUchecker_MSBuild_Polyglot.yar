rule HellsUchecker_MSBuild_Polyglot
{
    meta:
        id = "7HOcyNketAsPJBkLPs4PLr"
        fingerprint = "2454f4a4d7897ac24b7ea219c45b698fef0257df611ea1107e5d2bf1cfa7373d"
        version = "1.0"
        date = "2026-03-11"
        modified = "2026-03-11"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "HellsUchecker BAT/MSBuild polyglot - batch script header with embedded MSBuild XML project containing C# inline task. Base91-encoded .NET assembly payload between custom delimiters."
        category = "MALWARE"
        malware = "HELLSUCHECKER"
        mitre_att = "T1127.001"
        reference = "https://derp.ca/blog/hellsuchecker-clickfix-etherhiding"
        triage_score = 10
        triage_description = "HellsUchecker BAT/MSBuild polyglot detected. Batch script wrapping MSBuild inline C# task with base91-encoded payload."
        yarahub_uuid = "7dbaba8d-7306-49e4-a344-d618ccef5bf4"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "465d5847030acbbe1b6aaad39088d2f1"

    strings:
        $bat_header = "@echo off" ascii nocase
        $msbuild_project = "<Project" ascii
        $code_task_factory = "CodeTaskFactory" ascii
        $csharp_inline = "<Code Type=\"Class\" Language=\"cs\">" ascii
        $base91_delim = "::85a210c1::" ascii
        $task_class = "T_a84ee9c6" ascii

    condition:
        filesize > 1MB
        and filesize < 50MB
        and $bat_header in (0..100)
        and $msbuild_project
        and $code_task_factory
        and $csharp_inline
        and ($base91_delim or $task_class)
}
