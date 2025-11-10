import os

import requests


def download(url: str, path: str):
    if os.path.isdir(path):
        path = os.path.join(path, os.path.basename(url))

    print(f"Downloading {url} to {path}")
    response = requests.get(url, stream=True, timeout=None)
    response.raise_for_status()

    with open(path, "wb") as writer:
        name = path.split("/")[-1]
        total = int(response.headers.get('content-length', 0)) or None

        import rich.progress

        with rich.progress.Progress(
            rich.progress.SpinnerColumn(),
            rich.progress.TextColumn("[progress.description]{task.description}"),
            rich.progress.BarColumn(),
            rich.progress.DownloadColumn(),
            rich.progress.TransferSpeedColumn(),
            rich.progress.TimeRemainingColumn(),
        ) as progress:
            task = progress.add_task(f"Downloading {name}", total=total)
            for chunk in response.iter_content(chunk_size=4096):
                writer.write(chunk)
                progress.update(task, advance=len(chunk))

    return path
