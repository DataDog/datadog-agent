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

1. `python release_qa.py <AGENT_VERSION>`


### Adding a new test

1. Create a new test file in `test_cases` or add to an existing one. 
2. Subclass `TestCase` - if your test runs on multiple platforms you can use the provided `config` to change the test behavior based on which platform it is currently being generated for.
3. Add your test to an existing `Suite` in `release_qa.py`

### Adding a new suite

1. In `release_qa.py` create a new `Suite` instance with a configuration and a list of tests. 
2. The `build` function takes a callback to render the test content. The is usually a trello column. 