# Logs Agent Release QA

A collection of scripts that generate release QA tasks for the logs agent in a Trello board.

## Setup

1. `python3 -m venv venv`
2. `source venv/bin/activate`
3. `pip install -r requirements.txt`

In order to actually make a new board, create a `.env` file (or supply env vars via CLI) with:

```
API_KEY=<TRELLO API KEY>
API_SECRET=<TRELLO API SECRET>
ORG_ID=<TRELLO ORG ID>
```

If these are omitted, then the board is rendered to the console.

See "Getting the Secrets" below for more information

## Generate the board

1. `python release_qa.py <AGENT_VERSION>`


### Adding a new test

1. Create a new test file in `test_cases` or add to an existing one.
2. Subclass `TestCase` - if your test runs on multiple platforms you can use the provided `config` to change the test behavior based on which platform it is currently being generated for.
3. Add your test to an existing `Suite` in `release_qa.py`

### Adding a new suite

1. In `release_qa.py` create a new `Suite` instance with a configuration and a list of tests.
2. The `build` function takes a callback to render the test content. The is usually a trello column.

### Getting the Secrets
`api_key` and `api_secret` can be generated/found at https://trello.com/app-key

The `ORG_ID` is a bit trickier to find, but assuming you're the member of _any_
board in the org you're looking for, you can find it by checking the
`idOrganization` field on your boards.

```
curl 'https://api.trello.com/1/members/me/boards?key={api_key}&token={api_token}' | jq '.[] | {BoardName: .name, OrgId: .idOrganization} '
```

This will yield a list of boards and the organization they belong to, simply
select the appropriate org id.
