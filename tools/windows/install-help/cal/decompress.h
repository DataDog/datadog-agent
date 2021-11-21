#pragma once
#include <filesystem>

int decompress_archive(std::filesystem::path const &archivePath, std::filesystem::path const &destinationFolder);
