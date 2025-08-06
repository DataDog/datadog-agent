"""
S3 utilities for common operations
"""

import os

import boto3
from botocore.exceptions import ClientError

from tasks.libs.common.color import Color, color_message


def upload_file_to_s3(file_path: str, s3_bucket: str, s3_key: str) -> bool:
    """
    Upload a file to S3 using boto3.

    Args:
        file_path: Local path to the file to upload
        s3_bucket: S3 bucket name
        s3_key: S3 key (path) where to upload the file

    Returns:
        bool: True if upload was successful, False otherwise

    Raises:
        FileNotFoundError: If the local file doesn't exist
        ClientError: If S3 upload fails
    """
    if not os.path.exists(file_path):
        raise FileNotFoundError(f"File not found: {file_path}")

    try:
        s3_client = boto3.client('s3')

        # Upload the file
        s3_client.upload_file(Filename=file_path, Bucket=s3_bucket, Key=s3_key)

        print(color_message(f"Successfully uploaded {file_path} to s3://{s3_bucket}/{s3_key}", Color.GREEN))
        return True

    except ClientError as e:
        error_code = e.response['Error']['Code']
        error_message = e.response['Error']['Message']
        print(color_message(f"Failed to upload {file_path} to S3: {error_code} - {error_message}", Color.RED))
        raise
    except Exception as e:
        print(color_message(f"Unexpected error uploading {file_path} to S3: {e}", Color.RED))
        raise


def download_folder_from_s3(s3_bucket: str, s3_prefix: str, local_path: str) -> bool:
    """
    Download a folder from S3 to a specified local path.

    Args:
        s3_bucket: S3 bucket name
        s3_prefix: S3 prefix (folder path) to download
        local_path: Local directory path where to download the folder

    Returns:
        bool: True if download was successful, False otherwise

    Raises:
        ClientError: If S3 download fails
        OSError: If local directory creation fails
    """
    try:
        s3_client = boto3.client('s3')

        # Ensure local directory exists
        os.makedirs(local_path, exist_ok=True)

        # List all objects in the S3 prefix
        paginator = s3_client.get_paginator('list_objects_v2')
        page_iterator = paginator.paginate(Bucket=s3_bucket, Prefix=s3_prefix)

        downloaded_files = 0

        for page in page_iterator:
            if 'Contents' not in page:
                print(color_message(f"No objects found in s3://{s3_bucket}/{s3_prefix}", Color.ORANGE))
                return True

            for obj in page['Contents']:
                s3_key = obj['Key']

                # Skip if it's the prefix itself (directory marker)
                if s3_key == s3_prefix:
                    continue

                # Calculate local file path
                relative_path = s3_key[len(s3_prefix) :].lstrip('/')
                local_file_path = os.path.join(local_path, relative_path)

                # Ensure local directory exists for this file
                local_dir = os.path.dirname(local_file_path)
                if local_dir:
                    os.makedirs(local_dir, exist_ok=True)

                # Download the file
                print(color_message(f"Downloading {s3_key} to {local_file_path}", Color.GREEN))
                s3_client.download_file(s3_bucket, s3_key, local_file_path)
                downloaded_files += 1

        print(
            color_message(
                f"Successfully downloaded {downloaded_files} files from s3://{s3_bucket}/{s3_prefix} to {local_path}",
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
