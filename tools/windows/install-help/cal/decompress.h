#pragma once
#include <filesystem>

int decompress_archive(const std::filesystem::path &archivePath, const std::filesystem::path &destinationFolder);
