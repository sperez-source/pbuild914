# Meridian CLI

Admin and developer tooling.

## Setup

```bash
cd apps/cli
pip install -r requirements.txt
```

## Usage

```bash
export MERIDIAN_TOKEN=<jwt>
export MERIDIAN_API_URL=http://localhost:8000
python -m meridian_cli tasks export --project-id <uuid>
```

## Scenario 10

On branch `scenario/10-cli-export`, the `tasks export` command is a stub. See `docs/agent-scenarios/10-add-cli-export.md`.