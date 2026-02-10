# ddlogs

A fast CLI tool for searching and exporting Datadog logs via the V2 API with Flex storage tier support.

## Features

- **V2 Logs API** with Flex storage tier — no index required
- **Automatic pagination** — retrieves all matching logs across any time range
- **Concurrent fetch/write** — Go channels overlap API calls with disk I/O
- **CSV output** (default) — flat, token-efficient format ideal for LLM analysis
- **JSON output** — full structured data with all nesting preserved
- **Streaming writes** — logs hit disk page-by-page, no memory accumulation
- **Live progress** — real-time page count, log count, elapsed time, and rate on stderr

## Installation

```bash
go build -o ddlogs .
```

Optionally add an alias to your shell:

```bash
echo 'alias ddlogs="/path/to/ddlogs"' >> ~/.zshrc
source ~/.zshrc
```

## Configuration

Set the following environment variables:

| Variable | Required | Description |
|---|---|---|
| `DD_API_KEY` | Yes | Datadog API key |
| `DD_APP_KEY` | Yes | Datadog Application key |
| `DD_SITE` | No | Datadog site (default: `datadoghq.com`) |

```bash
export DD_API_KEY="your-api-key"
export DD_APP_KEY="your-app-key"
export DD_SITE="datadoghq.com"
```

## Usage

```bash
# Search last hour, CSV to stdout
ddlogs search -q "service:web" --from 1h

# Search last 24 hours, write CSV to file
ddlogs search -q "status:error" --from 24h -o errors.csv

# Search last 72 hours with compound query
ddlogs search -q 'service:(api OR web) @customer_id:"abc123"' --from 72h -o logs.csv

# JSON output
ddlogs search -q "host:prod-*" --from 30m -f json

# Custom time window (2 hours ago to 30 minutes ago)
ddlogs search -q "service:api" --from 2h --to 30m -o logs.csv
```

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--query` | `-q` | | Datadog logs query string (required) |
| `--from` | | `15m` | Start of time range as relative duration |
| `--to` | | `now` | End of time range |
| `--output` | `-o` | stdout | Output file path |
| `--format` | `-f` | `csv` | Output format: `csv` or `json` |

## Time Range Reference

Both `--from` and `--to` accept Go duration strings relative to now:

| Duration | Meaning |
|---|---|
| `5m` | 5 minutes ago |
| `15m` | 15 minutes ago |
| `1h` | 1 hour ago |
| `6h` | 6 hours ago |
| `24h` | 1 day ago |
| `72h` | 3 days ago |
| `168h` | 7 days ago |
| `720h` | 30 days ago |

## CSV Output

Default columns: `timestamp`, `host`, `service`, `status`, `message`, `tags`

Custom attributes (e.g. `@customer_id`, `@source.OAuthClientID`) are auto-discovered from the first page of results and added as extra columns.
