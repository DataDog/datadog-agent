#include "stdafx.h"
#include "decompress.h"
#include <fstream>
#include <sstream>
#include <archive.h>
#include <archive_entry.h>

static la_ssize_t copy_data(struct archive *ar, struct archive *aw)
{
    const void *buff;
    size_t size;
    la_int64_t offset;

    for (;;)
    {
        la_ssize_t r = archive_read_data_block(ar, &buff, &size, &offset);
        if (r == ARCHIVE_EOF)
        {
            return ARCHIVE_OK;
        }
        if (r < ARCHIVE_OK)
        {
            return r;
        }
        r = archive_write_data_block(aw, buff, size, offset);
        if (r < ARCHIVE_OK)
        {
            return r;
        }
    }
}

int decompress_archive(std::filesystem::path const &archivePath, std::filesystem::path const &destinationFolder)
{
    struct archive *a = archive_read_new();
    archive_read_support_compression_lzma(a);
    archive_read_support_format_7zip(a);

    const int flags = ARCHIVE_EXTRACT_TIME | ARCHIVE_EXTRACT_PERM | ARCHIVE_EXTRACT_ACL | ARCHIVE_EXTRACT_FFLAGS;
    struct archive *ext = archive_write_disk_new();
    archive_write_disk_set_options(ext, flags);
    archive_write_disk_set_standard_lookup(ext);

    la_ssize_t r;
    // Note, the construct ".string().c_str()" is ok since the temporary string
    // will be copied internally by archive_read_open_filename.
    if ((r = archive_read_open_filename(a, archivePath.string().c_str(), 10240)))
    {
        return 1;
    }

    for (;;)
    {
        struct archive_entry *entry;
        r = archive_read_next_header(a, &entry);
        if (r == ARCHIVE_EOF)
        {
            break;
        }
        if (r < ARCHIVE_OK)
        {
            WcaLog(LOGMSG_STANDARD, "Extracting archive failed (archive_read_next_header): %s\n", archive_error_string(a));
        }
        if (r < ARCHIVE_WARN)
        {
            return 1;
        }
        std::filesystem::path destFilepath(destinationFolder / archive_entry_pathname(entry));
        // Note, the construct ".string().c_str()" is ok since the temporary string
        // will be copied internally by archive_entry_set_pathname.
        archive_entry_set_pathname(entry, destFilepath.string().c_str());

        r = archive_write_header(ext, entry);
        if (r < ARCHIVE_OK)
        {
            WcaLog(LOGMSG_STANDARD, "Extracting archive failed (archive_write_header): %s\n", archive_error_string(ext));
        }
        else if (archive_entry_size(entry) > 0)
        {
            r = copy_data(a, ext);
            if (r < ARCHIVE_OK)
            {
                WcaLog(LOGMSG_STANDARD, "Extracting archive failed (copy_data): %s\n", archive_error_string(ext));
            }
            if (r < ARCHIVE_WARN)
            {
                return 1;
            }
        }
        r = archive_write_finish_entry(ext);
        if (r < ARCHIVE_OK)
        {
            WcaLog(LOGMSG_STANDARD, "Extracting archive failed (archive_write_finish_entry): %s\n", archive_error_string(ext));
        }
        if (r < ARCHIVE_WARN)
        {
            return 1;
        }
    }

    archive_read_close(a);
    archive_read_free(a);

    archive_write_close(ext);
    archive_write_free(ext);

    return 0;
}
