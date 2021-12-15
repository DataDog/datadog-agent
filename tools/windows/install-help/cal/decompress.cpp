#include "stdafx.h"
#include "decompress.h"
#include <archive.h>
#include <archive_entry.h>
#include <fstream>
#include <sstream>
#include <stdexcept>

/// <summary>
/// Wrapper class around libarchive's API and resources.
/// This class ensures that all resources allocated are properly cleaned up.
/// </summary>
class DecompressionContext
{
private:
    struct archive* _archive;
    struct archive* _extractor;

public:
    explicit DecompressionContext(const std::filesystem::path &archivePath)
    {
        _archive = archive_read_new();
        archive_read_support_compression_lzma(_archive);
        archive_read_support_format_7zip(_archive);

        const int flags = ARCHIVE_EXTRACT_TIME | ARCHIVE_EXTRACT_PERM | ARCHIVE_EXTRACT_ACL | ARCHIVE_EXTRACT_FFLAGS;
        _extractor = archive_write_disk_new();
        archive_write_disk_set_options(_extractor, flags);
        archive_write_disk_set_standard_lookup(_extractor);

        // Note, the construct ".string().c_str()" is ok since the temporary string
        // will be copied internally by archive_read_open_filename.
        if (archive_read_open_filename(_archive, archivePath.string().c_str(), 10240) != ARCHIVE_OK)
        {
            throw std::runtime_error(archive_error_string(_archive));
        }
    }

    struct archive_entry* read_next_header()
    {
        struct archive_entry* entry;
        la_ssize_t r = archive_read_next_header(_archive, &entry);
        if (r == ARCHIVE_EOF)
        {
            return nullptr;
        }
        if (r < ARCHIVE_OK)
        {
            throw std::runtime_error(archive_error_string(_archive));
        }
        return entry;
    }

    void write_header(struct archive_entry* entry)
    {
        if (archive_write_header(_extractor, entry) < ARCHIVE_OK)
        {
            throw std::runtime_error(archive_error_string(_extractor));
        }
    }

    void write_finish_entry()
    {
        if (archive_write_finish_entry(_extractor) < ARCHIVE_OK)
        {
            throw std::runtime_error(archive_error_string(_extractor));
        }
    }

    void copy_data()
    {
        const void* buff;
        size_t size;
        la_int64_t offset;

        for (;;)
        {
            const la_ssize_t result = archive_read_data_block(_archive, &buff, &size, &offset);
            if (result == ARCHIVE_EOF)
            {
                break;
            }
            if (result < ARCHIVE_OK)
            {
                throw std::runtime_error(archive_error_string(_archive));
            }
            if (archive_write_data_block(_extractor, buff, size, offset) < ARCHIVE_OK)
            {
                throw std::runtime_error(archive_error_string(_extractor));
            }
        }
    }

    ~DecompressionContext()
    {
        archive_read_close(_archive);
        archive_read_free(_archive);

        archive_write_close(_extractor);
        archive_write_free(_extractor);
    }
};

int decompress_archive(const std::filesystem::path &archivePath, const std::filesystem::path &destinationFolder)
{
    try
    {
        DecompressionContext context(archivePath);
        for (;;)
        {
            struct archive_entry* entry = context.read_next_header();
            if (entry == nullptr)
            {
                return 0;
            }

            std::filesystem::path destFilepath(destinationFolder / archive_entry_pathname(entry));
            // Note, the construct ".string().c_str()" is ok since the temporary string
            // will be copied internally by archive_entry_set_pathname.
            archive_entry_set_pathname(entry, destFilepath.string().c_str());

            context.write_header(entry);
            if (archive_entry_size(entry) > 0)
            {
                context.copy_data();
            }
            context.write_finish_entry();
        }
    }
    catch (const std::exception& e)
    {
        WcaLog(LOGMSG_STANDARD, "Extracting archive failed: %s", e.what());
        return 1;
    }
}
