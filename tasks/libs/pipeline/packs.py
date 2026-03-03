import functools
import os
import subprocess

import requests

# Default datacenter
DATACENTER = "us1.ddbuild.io"
HOLOCENE_URL = f"https://holocene.{DATACENTER}/v1/packs"


def get_token():
    try:
        # Check if we have a token in the environment
        if "HOLOCENE_TOKEN" in os.environ:
            return os.environ["HOLOCENE_TOKEN"]

        # Fallback to ddtool
        return (
            subprocess.check_output(
                ["ddtool", "auth", "token", "holocene", "--datacenter", DATACENTER],
                stderr=subprocess.DEVNULL,
                timeout=2,
            )
            .decode()
            .strip()
        )
    except Exception:
        return None


@functools.lru_cache(maxsize=1)
def fetch_all_packs():
    token = get_token()
    if not token:
        return []

    packs = []
    next_page_token = ""
    while True:
        try:
            url = f"{HOLOCENE_URL}?pageSize=1000"
            if next_page_token:
                url += f"&next_page_token={next_page_token}"

            response = requests.get(url, headers={"Authorization": f"Bearer {token}"}, timeout=10)
            if response.status_code != 200:
                break

            data = response.json()
            packs.extend(data.get("packs", []))
            next_page_token = data.get("next_page_token")
            if not next_page_token:
                break
        except Exception:
            break
    return packs


@functools.lru_cache(maxsize=1)
def get_packs_map():
    packs = fetch_all_packs()
    # Map pack ID (which corresponds to team slug) to communication channels
    return {p['id'].lower(): p.get('communication_channels', {}) for p in packs}


def get_team_channels(team_name):
    """
    Returns (notification_channel, review_channel) for a team name (e.g. '@datadog/agent-devx').
    """
    packs_map = get_packs_map()

    # Remove @datadog/ prefix and lowercase
    pack_id = team_name.replace("@datadog/", "").lower()
    channels = packs_map.get(pack_id, {})

    notif = channels.get("notification_channel")
    contact = channels.get("contact_channel")
    review = channels.get("review_channel")

    # Fallbacks as per requirements
    # Notification: notification_channel or contact_channel
    final_notif = notif or contact

    # Review: review_channel or contact_channel
    final_review = review or contact

    return final_notif, final_review
