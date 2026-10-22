import boto3
import time
import os

BUCKET_NAME = os.environ.get('BUCKET_NAME')
BUCKET_PREFIX = os.environ.get('BUCKET_PREFIX', '')
MAX_AGE_HOURS = os.environ.get('MAX_AGE_HOURS', 4)
MAX_RETRY = os.environ.get('MAX_RETRY', 5)
SLEEPING_INTERVAL = os.environ.get('SLEEPING_INTERVAL', 3)

client = boto3.client('s3')

def delete_with_retry(bucket, key, max_retry, sleeping_interval):
    deleted = False
    trials = 0
    while not deleted and trials < max_retry:
        try:
            client.delete_object(Bucket=BUCKET_NAME, Key=key)
        except Exception as error:
            print("Failed to delete: ", key, " error: ", error)
            trials += 1
            if trials < max_retry:
                print("Retrying...")
                time.sleep(sleeping_interval)
        else:
            deleted = True
            print("Deleted ", key)

if __name__ == "__main__":
    all_processed = False
    next_marker = ""
    while not all_processed:
        obj = client.list_objects(Bucket=BUCKET_NAME, Prefix=BUCKET_PREFIX, Marker=next_marker)
        if obj['IsTruncated'] == False:
            all_processed = True

        if 'Contents' in obj:
            for element in obj['Contents']:
                now = time.time()

                last_modified = element['LastModified']
                key = element['Key']

                age_hours = (now - last_modified.timestamp()) / 3600

                if age_hours > float(MAX_AGE_HOURS):
                    print("Deleting: ", key)
                    delete_with_retry(BUCKET_NAME, key, MAX_RETRY, SLEEPING_INTERVAL)
                else:
                    next_marker = key
