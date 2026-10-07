import "elf"

rule ELF_Loader_KrustyLoader_Feb2024 {
    meta:
        Description = "Detects Krusty Loader written in Rust which was linked to Ivanti ConnectSecure compromises"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Credits = "Awesome analysis by Synacktiv which includes an extractor script and YARA rule based on Hex Sequences"
        Reference = "https://www.synacktiv.com/publications/krustyloader-rust-malware-linked-to-ivanti-connectsecure-compromises"
        Suggested_Reading = "Good source on ELF Headers: https://www.sco.com/developers/gabi/latest/ch4.eheader.html"
        Hash = "030eb56e155fb01d7b190866aaa8b3128f935afd0b7a7b2178dc8e2eb84228b0"
        date = "2024-02-04"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "deff93081ccb3fda7a12f6e9e3ad15ad"
        yarahub_uuid = "9f3cbd3a-bd6d-41ba-9756-3f160963f8c4"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "elf.krustyloader"

    strings:
        $rust1 = "/rustc/90c541806f23a127002de5b4038be731ba1458ca/library/"
        $rust2 = "/tmp//cargo/registry/src/index.crates.io-6f17d22bba15001f/"
        $tmp = "/tmp/f9fbd9e96355a0b9197508e03238d855befe342220c7f531ca1490f628ca4b6d0168ba12c9f719e5aa4f19f5756f7b8c24b5eb337ce31c5296"
        $cmd1 = "/proc/self/task/%d/comm"
        $cmd2 = "/usr/local/bin:/bin:/usr/bin"
        $cmd3 = "/proc/self/exe"
        $llvm1 = "/checkout/src/llvm-project/libunwind/src/DwarfInstructions.hpp"
        $llvm2 = "/checkout/src/llvm-project/libunwind/src/DwarfParser.hpp"
        $net1 = "Socket is connected"
        $net2 = "Network is down"
        $net3 = "Connection aborted"
        $net4 = "Connection reset by peer"
        $net5 = "http://://scheme"
        $net6 = "HTTP/2.0"
        $net7 = "nameserver"
        $net8 = "domain"
        $net9 = "127.0.0.1"

    condition:
        elf.number_of_sections == 26 
        and elf.dynsym_entries == 1
        and $tmp
        and any of ($rust*)
        and any of ($llvm*)
        and 2 of ($cmd*)
        and 5 of ($net*)
       
 }
