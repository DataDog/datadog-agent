"""
S3 utilities for common operations
"""

import os

from tasks.libs.common.color import Color, color_message


def upload_file_to_s3(file_path: str, s3_path: str) -> bool:
    """
    Upload a file to S3 using boto3.

    Args:
        file_path: Local path to the file to upload
        s3_path: S3 path (bucket name and key)

    Returns:
        bool: True if upload was successful, False otherwise

    Raises:
        FileNotFoundError: If the local file doesn't exist
        ClientError: If S3 upload fails
    """
    import boto3
    from botocore.exceptions import ClientError

    if not os.path.exists(file_path):
        raise FileNotFoundError(f"File not found: {file_path}")

    if s3_path.startswith("s3://"):
        s3_path = s3_path.removeprefix("s3://")
    s3_bucket_name = s3_path.split("/")[0]
    s3_key = "/".join(s3_path.split("/")[1:])

    try:
        s3_client = boto3.client('s3')

        # Upload the file
        s3_client.upload_file(Filename=file_path, Bucket=s3_bucket_name, Key=s3_key)

        print(color_message(f"Successfully uploaded {file_path} to s3://{s3_bucket_name}/{s3_key}", Color.GREEN))
        return True

    except ClientError as e:
        error_code = e.response['Error']['Code']
        error_message = e.response['Error']['Message']
        print(color_message(f"Failed to upload {file_path} to S3: {error_code} - {error_message}", Color.RED))
        raise
    except Exception as e:
        print(color_message(f"Unexpected error uploading {file_path} to S3: {e}", Color.RED))
        raise


def download_file_from_s3(s3_path: str, local_path: str) -> bool:
    """
    Download a file from S3 to a specified local path.
    """
    import boto3
    from botocore.exceptions import ClientError

    if s3_path.startswith("s3://"):
        s3_path = s3_path.removeprefix("s3://")
    s3_bucket_name = s3_path.split("/")[0]
    s3_key = "/".join(s3_path.split("/")[1:])

    try:
        s3_client = boto3.client('s3')
        s3_client.download_file(s3_bucket_name, s3_key, local_path)
        return True

    except ClientError as e:
        error_code = e.response['Error']['Code']
        error_message = e.response['Error']['Message']
        print(color_message(f"Failed to download file from S3: {error_code} - {error_message}", Color.RED))
        raise
    except Exception as e:
        print(color_message(f"Unexpected error downloading file from S3: {e}", Color.RED))
        raise


def download_folder_from_s3(s3_path: str, local_path: str) -> bool:
    """
    Download a folder from S3 to a specified local path.

    Args:
        s3_path: S3 path (bucket name and key)
        local_path: Local directory path where to download the folder

    Returns:
        bool: True if download was successful, False otherwise

    Raises:
        ClientError: If S3 download fails
        OSError: If local directory creation fails
    """
    import boto3
    from botocore.exceptions import ClientError

    if s3_path.startswith("s3://"):
        s3_path = s3_path.removeprefix("s3://")
    s3_bucket_name = s3_path.split("/")[0]
    s3_prefix = "/".join(s3_path.split("/")[1:])

    try:
        s3_client = boto3.client('s3')

        # Ensure local directory exists
        os.makedirs(local_path, exist_ok=True)

        # List all objects in the S3 prefix
        paginator = s3_client.get_paginator('list_objects_v2')
        page_iterator = paginator.paginate(Bucket=s3_bucket_name, Prefix=s3_prefix)

        downloaded_files = 0

        for page in page_iterator:
            if 'Contents' not in page:
                print(color_message(f"No objects found in s3://{s3_bucket_name}/{s3_prefix}", Color.ORANGE))
                return True

            for obj in page['Contents']:
                s3_key = obj['Key'].rstrip('/')

                # Skip if it's the prefix itself (directory marker)
                if s3_key == s3_prefix:
                    continue

                # Calculate local file path
                print("S3 KEY", s3_key)
                print("S3 PREFIX", s3_prefix)
                relative_path = s3_key[len(s3_prefix) :].lstrip('/')
                local_file_path = os.path.join(local_path, relative_path)
                print("LOCAL FILE PATH", local_file_path)

                # Ensure local directory exists for this file
                local_dir = os.path.dirname(local_file_path)
                if local_dir:
                    os.makedirs(local_dir, exist_ok=True)

                # Download the file
                print(color_message(f"Downloading {s3_key} to {local_file_path}", Color.GREEN))
                s3_client.download_file(s3_bucket_name, s3_key, local_file_path)
                downloaded_files += 1

        print(
            color_message(
                f"Successfully downloaded {downloaded_files} files from s3://{s3_bucket_name}/{s3_prefix} to {local_path}",
                Color.GREEN,
            )
        )
        return True

    except ClientError as e:
        error_code = e.response['Error']['Code']
        error_message = e.response['Error']['Message']
        print(color_message(f"Failed to download folder from S3: {error_code} - {error_message}", Color.RED))
        raise
    except OSError as e:
        print(color_message(f"Failed to create local directory {local_path}: {e}", Color.RED))
        raise
    except Exception as e:
        print(color_message(f"Unexpected error downloading folder from S3: {e}", Color.RED))
        raise


def list_sorted_keys_in_s3(s3_path: str, filename: str) -> list[str]:
    """
    List all direct subfolders of a given S3 path, that contains a file with the given filename.
    The list is sorted by the date of the last modification.

    Args:
        s3_path: S3 path (e.g., "s3://bucket/path/" or "bucket/path/")
        filename: Filename to search for in the subfolders
    Returns:
        list[str]: List of direct subfolder names (without the full path), sorted by the date of the last modification

    Example:

    Raises:
        ClientError: If S3 operation fails
    """
    import boto3
    from botocore.exceptions import ClientError

    if s3_path.startswith("s3://"):
        s3_path = s3_path.removeprefix("s3://")

    s3_bucket_name = s3_path.split("/")[0]
    s3_prefix = "/".join(s3_path.split("/")[1:])

    # Ensure prefix ends with '/' for proper folder listing
    if s3_prefix and not s3_prefix.endswith('/'):
        s3_prefix += '/'

    try:
        s3_client = boto3.client('s3')
        # List all objects in the S3 prefix
        response = s3_client.list_objects_v2(Bucket=s3_bucket_name, Prefix=s3_prefix)

        # Group by folder and find latest date per folder
        keys_date = {}
        for obj in response.get('Contents', []):
            if obj['Key'].endswith(filename):
                if obj['Key'] not in keys_date or obj['LastModified'] > keys_date[obj['Key']]:
                    keys_date[obj['Key']] = obj['LastModified']

        sorted_keys = sorted(keys_date.items(), key=lambda x: x[1], reverse=True)
        return [key.removeprefix(s3_prefix) for key, _ in sorted_keys]

    except ClientError as e:
        error_code = e.response['Error']['Code']
        error_message = e.response['Error']['Message']
        print(color_message(f"Failed to list subfolders in S3: {error_code} - {error_message}", Color.RED))
        raise
