package cmd

import (
	"fmt"
	"os"

	"github.com/dneil5648/dd-logs-cli/handlers"
	"github.com/spf13/cobra"
)

var (
	searchQuery  string
	searchFrom   string
	searchTo     string
	searchOutput string
	searchFormat string
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search Datadog logs",
	Long: `Search Datadog logs with a query string and time range.

Automatically paginates through all matching results using the Datadog V2 API
with Flex storage tier. Fetching and writing run concurrently via Go channels
for maximum throughput.

Output Formats:
  csv   (default)  Flat columns, token-efficient for LLM analysis.
                   Fixed columns: timestamp, host, service, status, message, tags.
                   Custom attributes (@fields) are auto-discovered and added as columns.
  json             Full structured JSON array, preserves all nesting.

Time Range (--from / --to):
  Both flags accept duration strings relative to now. The value is sent to the
  Datadog API as "now-<duration>", e.g. --from 1h becomes "now-1h".

  Common durations:
    5m       5 minutes ago
    15m      15 minutes ago (default for --from)
    1h       1 hour ago
    6h       6 hours ago
    24h      1 day ago
    72h      3 days ago
    168h     7 days ago
    720h     30 days ago

  --to defaults to "now" (the current time). Set it to a duration to define an
  end boundary in the past, creating a fixed window:

    --from 48h --to 24h    logs from 2 days ago to 1 day ago
    --from 2h  --to 30m    logs from 2 hours ago to 30 minutes ago

  Note: Go durations use "h" for hours and "m" for minutes. There is no "d" unit,
  so use 24h for 1 day, 168h for 7 days, etc.

Progress:
  A live status line on stderr shows: page number, log count, elapsed time, and rate.`,
	Example: `  # Search last hour, CSV to stdout
  ddlogs search -q "service:web" --from 1h

  # Search last 24 hours, write CSV to file
  ddlogs search -q "status:error" --from 24h -o errors.csv

  # Search last 72 hours with compound query, output to file
  ddlogs search -q 'service:(api OR web) @customer_id:"abc123"' --from 72h -o logs.csv

  # JSON output
  ddlogs search -q "host:prod-*" --from 30m -f json

  # Custom time window (30 min ago to 5 min ago)
  ddlogs search -q "service:api" --from 30m --to 5m -o logs.csv`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := os.Getenv("DD_API_KEY")
		appKey := os.Getenv("DD_APP_KEY")
		site := os.Getenv("DD_SITE")

		if apiKey == "" {
			return fmt.Errorf("DD_API_KEY environment variable is required")
		}
		if appKey == "" {
			return fmt.Errorf("DD_APP_KEY environment variable is required")
		}
		if site == "" {
			site = "datadoghq.com"
		}

		if searchFormat != "csv" && searchFormat != "json" {
			return fmt.Errorf("--format must be csv or json")
		}

		handler := handlers.NewDDHandler(site, apiKey, appKey)
		return handler.Query(searchQuery, searchFrom, searchTo, searchOutput, searchFormat)
	},
}

func init() {
	searchCmd.Flags().StringVarP(&searchQuery, "query", "q", "", "Datadog logs query string (required)")
	searchCmd.Flags().StringVar(&searchFrom, "from", "15m", "Start of time range as a relative duration (e.g. 15m, 1h, 24h, 72h)")
	searchCmd.Flags().StringVar(&searchTo, "to", "now", "End of time range (e.g. 5m, now)")
	searchCmd.Flags().StringVarP(&searchOutput, "output", "o", "", "Output file path (default: stdout)")
	searchCmd.Flags().StringVarP(&searchFormat, "format", "f", "csv", "Output format: csv or json")
	searchCmd.MarkFlagRequired("query")
	rootCmd.AddCommand(searchCmd)
}
