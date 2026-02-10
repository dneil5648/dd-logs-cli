package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ddlogs",
	Short: "A CLI for querying Datadog logs",
	Long: `ddlogs is a command-line tool for searching and streaming Datadog logs
from the Flex storage tier via the Datadog V2 Logs API.

Environment Variables:
  DD_API_KEY   (required)  Your Datadog API key
  DD_APP_KEY   (required)  Your Datadog Application key
  DD_SITE      (optional)  Datadog site (default: datadoghq.com)
                           Examples: datadoghq.eu, us3.datadoghq.com, us5.datadoghq.com

Quick Start:
  export DD_API_KEY="your-api-key"
  export DD_APP_KEY="your-app-key"
  ddlogs search -q "service:web" --from 1h`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
