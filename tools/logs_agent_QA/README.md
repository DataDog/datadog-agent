# Logs Agent Release QA

A collection of scripts that generate release QA tasks for the logs agent in a Trello board. 

## Setup

1. `python3 -m venv venv`
2. `source venv/bin/activate`
3. `pip install -r requirements.txt`

create a `.env` file (or supply env vars via CLI) with:

```
API_KEY=<TRELLO API KEY>
API_SECRET=<TRELLO API SECRET>
ORG_ID=<TRELLO ORG ID>
```

## Generate the board 

1. `python releaseQA.py <AGENT_VERSION>`