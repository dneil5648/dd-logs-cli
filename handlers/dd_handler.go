package handlers

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

const maxLogsPerRequest int32 = 1000

type DDHandler struct {
	Site   string
	ApiKey string
	AppKey string
}

func NewDDHandler(site, apiKey, appKey string) *DDHandler {
	return &DDHandler{
		Site:   site,
		ApiKey: apiKey,
		AppKey: appKey,
	}
}

func toDatadogTime(value string) string {
	if value == "now" {
		return "now"
	}
	return "now-" + value
}

// fetchResult is sent from the fetch goroutine to the write goroutine.
type fetchResult struct {
	logs []datadogV2.Log
	page int
}

func (h *DDHandler) Query(query, from, to, outputFile, format string) error {
	fromStr := toDatadogTime(from)
	toStr := toDatadogTime(to)

	ctx := context.Background()
	ctx = context.WithValue(ctx, datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: h.ApiKey},
		"appKeyAuth": {Key: h.AppKey},
	})
	ctx = context.WithValue(ctx, datadog.ContextServerVariables, map[string]string{
		"site": h.Site,
	})

	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	api := datadogV2.NewLogsApi(apiClient)

	storageTier := datadogV2.LOGSSTORAGETIER_FLEX

	// Channel to send fetched pages to the writer goroutine.
	// Buffer of 2 so the fetcher can stay one page ahead of the writer.
	pageCh := make(chan fetchResult, 2)

	// Shared state for progress reporting
	var mu sync.Mutex
	totalLogs := 0
	lastPage := 0
	start := time.Now()

	// Fetch error from the fetcher goroutine
	var fetchErr error

	// --- Fetcher goroutine: fetches pages sequentially, sends to channel ---
	go func() {
		defer close(pageCh)

		var cursor *string
		page := 1

		for {
			body := datadogV2.LogsListRequest{
				Filter: &datadogV2.LogsQueryFilter{
					Query:       datadog.PtrString(query),
					From:        datadog.PtrString(fromStr),
					To:          datadog.PtrString(toStr),
					StorageTier: &storageTier,
				},
				Sort: datadogV2.LOGSSORT_TIMESTAMP_ASCENDING.Ptr(),
				Page: &datadogV2.LogsListRequestPage{
					Limit: datadog.PtrInt32(maxLogsPerRequest),
				},
			}
			if cursor != nil {
				body.Page.Cursor = cursor
			}

			resp, r, err := api.ListLogs(ctx, *datadogV2.NewListLogsOptionalParameters().WithBody(body))
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nFull HTTP response: %v\n", r)
				fetchErr = fmt.Errorf("calling LogsApi.ListLogs: %w", err)
				return
			}

			logs := resp.GetData()

			pageCh <- fetchResult{logs: logs, page: page}

			// Update progress
			mu.Lock()
			totalLogs += len(logs)
			lastPage = page
			elapsed := time.Since(start).Seconds()
			rate := float64(totalLogs) / elapsed
			fmt.Fprintf(os.Stderr, "\rFetching... page %d | %d logs | %.1fs | %.0f logs/sec", lastPage, totalLogs, elapsed, rate)
			mu.Unlock()

			// Check for next page
			meta, ok := resp.GetMetaOk()
			if !ok {
				return
			}
			respPage, ok := meta.GetPageOk()
			if !ok {
				return
			}
			after, ok := respPage.GetAfterOk()
			if !ok || *after == "" {
				return
			}
			if int32(len(logs)) < maxLogsPerRequest {
				return
			}
			cursor = after
			page++
		}
	}()

	// --- Writer: runs on main goroutine, reads from channel ---
	var dest io.Writer = os.Stdout
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		dest = f
	}
	bw := bufio.NewWriterSize(dest, 256*1024)
	defer bw.Flush()

	var writer logWriter
	if format == "json" {
		writer = newJSONWriter(bw)
	} else {
		writer = newCSVWriter(bw)
	}

	writer.Start()

	firstPage := true
	for result := range pageCh {
		for _, log := range result.logs {
			if err := writer.WriteLog(log); err != nil {
				return fmt.Errorf("writing log: %w", err)
			}
		}

		// For CSV: after the first page, flush the buffered logs and write headers
		if firstPage {
			if err := writer.FlushPage(); err != nil {
				return fmt.Errorf("flushing first page: %w", err)
			}
			firstPage = false
		}

		if err := bw.Flush(); err != nil {
			return fmt.Errorf("flushing output: %w", err)
		}
	}

	// Check if fetcher hit an error
	if fetchErr != nil {
		return fetchErr
	}

	writer.End()

	mu.Lock()
	elapsed := time.Since(start).Seconds()
	fmt.Fprintf(os.Stderr, "\rDone: %d logs retrieved in %.1fs across %d page(s)\n", totalLogs, elapsed, lastPage)
	if outputFile != "" {
		fmt.Fprintf(os.Stderr, "Output written to %s\n", outputFile)
	}
	mu.Unlock()

	return nil
}

// logWriter abstracts CSV vs JSON streaming output.
type logWriter interface {
	Start()
	WriteLog(log datadogV2.Log) error
	FlushPage() error
	End()
}

// --- JSON writer ---

type jsonWriter struct {
	bw    *bufio.Writer
	count int
}

func newJSONWriter(bw *bufio.Writer) *jsonWriter {
	return &jsonWriter{bw: bw}
}

func (w *jsonWriter) Start() {
	w.bw.WriteString("[\n")
}

func (w *jsonWriter) WriteLog(log datadogV2.Log) error {
	if w.count > 0 {
		w.bw.WriteString(",\n")
	}
	entry, err := json.MarshalIndent(log, "  ", "  ")
	if err != nil {
		return err
	}
	w.bw.WriteString("  ")
	w.bw.Write(entry)
	w.count++
	return nil
}

func (w *jsonWriter) FlushPage() error { return nil }

func (w *jsonWriter) End() {
	w.bw.WriteString("\n]\n")
}

// --- CSV writer ---

var fixedColumns = []string{"timestamp", "host", "service", "status", "message", "tags"}

type csvWriter struct {
	w       *csv.Writer
	headers []string
	attrSet map[string]bool
	buffer  []datadogV2.Log
	started bool
}

func newCSVWriter(bw *bufio.Writer) *csvWriter {
	return &csvWriter{
		w:       csv.NewWriter(bw),
		attrSet: make(map[string]bool),
	}
}

func (c *csvWriter) Start() {}

func (c *csvWriter) WriteLog(log datadogV2.Log) error {
	attrs := log.GetAttributes()
	for key := range attrs.GetAttributes() {
		c.attrSet[key] = true
	}

	if !c.started {
		c.buffer = append(c.buffer, log)
		return nil
	}
	return c.writeRow(log)
}

func (c *csvWriter) flushBuffer() error {
	var attrCols []string
	for k := range c.attrSet {
		attrCols = append(attrCols, k)
	}
	sort.Strings(attrCols)
	c.headers = append(fixedColumns, attrCols...)

	if err := c.w.Write(c.headers); err != nil {
		return err
	}
	for _, log := range c.buffer {
		if err := c.writeRow(log); err != nil {
			return err
		}
	}
	c.buffer = nil
	c.started = true
	c.w.Flush()
	return c.w.Error()
}

func (c *csvWriter) writeRow(log datadogV2.Log) error {
	attrs := log.GetAttributes()
	customAttrs := attrs.GetAttributes()

	row := make([]string, len(c.headers))
	for i, col := range c.headers {
		switch col {
		case "timestamp":
			if t, ok := attrs.GetTimestampOk(); ok && t != nil {
				row[i] = t.Format(time.RFC3339)
			}
		case "host":
			row[i] = attrs.GetHost()
		case "service":
			row[i] = attrs.GetService()
		case "status":
			row[i] = attrs.GetStatus()
		case "message":
			row[i] = attrs.GetMessage()
		case "tags":
			row[i] = strings.Join(attrs.GetTags(), ";")
		default:
			if v, ok := customAttrs[col]; ok {
				row[i] = flattenValue(v)
			}
		}
	}
	return c.w.Write(row)
}

func (c *csvWriter) FlushPage() error {
	if !c.started {
		return c.flushBuffer()
	}
	c.w.Flush()
	return c.w.Error()
}

func (c *csvWriter) End() {
	if !c.started {
		c.flushBuffer()
	}
	c.w.Flush()
}

func flattenValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}
