import boto3
import time
import argparse


client = boto3.client("ec2")


def main():
    parser = argparse.ArgumentParser(
        prog="Ec2Cleaner", description="Clean zombies ec2 instances on AWS"
    )
    parser.add_argument("--max-age-hours", default=4, type=float)
    parser.add_argument("--batch-size", default=10, type=int)
    parser.add_argument("--max-retry", default=5, type=int)
    parser.add_argument("--sleeping-interval", default=3, type=int)
    parser.add_argument("--dry-run", action="store_true")

    args = parser.parse_args()

    all_processed = False
    next_token = None

    now = time.time()

    instance_filter = [
        {"Name": "tag:CreatedBy", "Values": ["datadog-agent-kitchen-tests"]},
        {"Name": "instance-state-name", "Values": ["running"]},
    ]
    while not all_processed:
        if next_token:
            instances = client.describe_instances(
                NextToken=next_token, Filters=instance_filter
            )
        else:
            instances = client.describe_instances(Filters=instance_filter)
        zombie_instance_ids = []

        for reservation in instances["Reservations"]:
            for instance in reservation["Instances"]:
                launch_time = instance["LaunchTime"]
                age_hours = (now - launch_time.timestamp()) / 3600
                if age_hours > args.max_age_hours:
                    zombie_instance_ids.append(instance["InstanceId"])
        print("Found the following zombie instances: ", zombie_instance_ids)
        if not args.dry_run:
            batch_delete_with_retry(
                zombie_instance_ids,
                args.batch_size,
                args.max_retry,
                args.sleeping_interval,
            )
        else:
            print("Not deleting the instances because dry run is set to True")
        if "NextToken" not in instances:
            all_processed = True
        else:
            next_token = instances["NextToken"]


def batch_delete_with_retry(instances, batch_size, max_retry, sleeping_interval):
    idx_to_delete = 0

    while idx_to_delete < len(instances):
        trials = 0
        terminated = False
        while not terminated and trials < max_retry:
            try:
                max_batch_size = min(idx_to_delete + batch_size, len(instances))
                client.terminate_instances(
                    InstanceIds=instances[idx_to_delete:max_batch_size]
                )
                print(
                    f"Terminated instances {instances[idx_to_delete : min(idx_to_delete + batch_size, len(instances))]}"
                )
                terminated = True
            except Exception as error:
                print(
                    f"Failed to terminate instances {instances[idx_to_delete : min(idx_to_delete + batch_size, len(instances))]}, error: {error}"
                )
                trials += 1
                if trials < max_retry:
                    print("Retrying...")
                    time.sleep(sleeping_interval)
                else:
                    raise RuntimeError(error)
        idx_to_delete += batch_size


if __name__ == "__main__":
    main()
