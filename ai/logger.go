// logger.go provides file-based logging for ALL AI interactions.
//
// Logs are written to ~/.paisql/logs/ai.log with timestamps.
// Covers: Chat, SuggestIndexes, GenerateQueryPlan.
package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	logOnce sync.Once
	logFile *os.File
)

// initLog opens (or creates) the log file. Called once lazily.
func initLog() {
	logOnce.Do(func() {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return
		}
		logDir := filepath.Join(homeDir, ".paisql", "logs")
		if err := os.MkdirAll(logDir, 0700); err != nil {
			return
		}
		logPath := filepath.Join(logDir, "ai.log")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return
		}
		logFile = f
	})
}

func logWrite(s string) {
	initLog()
	if logFile != nil {
		logFile.WriteString(s) //nolint:errcheck
	}
}

// ─────────────────────────────────────────────────────────────────
// Generic AI request / response logging
// ─────────────────────────────────────────────────────────────────

// LogAIRequest logs any AI request with the given operation name and input details.
func LogAIRequest(operation string, provider string, details map[string]string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		"\n%s\n"+
			"════════════════════════════════════════════════════════════════\n"+
			"[REQUEST] %s  |  Op: %s  |  Provider: %s\n"+
			"════════════════════════════════════════════════════════════════\n",
		ts, ts, operation, provider,
	))
	for k, v := range details {
		sb.WriteString(fmt.Sprintf("%s:\n%s\n────────────────────────────────────────\n", k, v))
	}
	logWrite(sb.String())
}

// LogAIResponse logs any AI response with the given operation name.
func LogAIResponse(operation string, response string, err error) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	var errStr string
	if err != nil {
		errStr = err.Error()
	} else {
		errStr = "(none)"
	}
	entry := fmt.Sprintf(
		"[RESPONSE] %s  |  Op: %s\n"+
			"────────────────────────────────────────\n"+
			"Error: %s\n"+
			"────────────────────────────────────────\n"+
			"Response:\n%s\n"+
			"════════════════════════════════════════════════════════════════\n\n",
		ts, operation, errStr, response,
	)
	logWrite(entry)
}

// ─────────────────────────────────────────────────────────────────
// Query plan specific logging (richer detail)
// ─────────────────────────────────────────────────────────────────

// LogQueryPlanRequest logs a query plan generation request.
func LogQueryPlanRequest(provider string, schemaContext string, userQuestion string, dataViewState string) {
	LogAIRequest("QueryPlan", provider, map[string]string{
		"User Question":   userQuestion,
		"Data View State": dataViewState,
		"Schema Context":  schemaContext,
	})
}

// LogQueryPlanResponse logs a query plan generation response with parsed details.
func LogQueryPlanResponse(rawResponse string, plan *QueryPlan, sql string, err error) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	var errStr string
	if err != nil {
		errStr = err.Error()
	} else {
		errStr = "(none)"
	}

	var planSummary string
	if plan != nil {
		planSummary = fmt.Sprintf(
			"Tables: %s\nJoins: %s\nFilters: %s\nSelect: %s\nLimit: %d, Page: %d\nAction: %s\nDescription: %s",
			strings.Join(plan.Tables, ", "),
			strings.Join(plan.Joins, ", "),
			strings.Join(plan.Filters, ", "),
			strings.Join(plan.Select, ", "),
			plan.Limit, plan.Page,
			plan.Action, plan.Description,
		)
		if plan.Sort != nil {
			planSummary += fmt.Sprintf("\nSort: %s %s", plan.Sort.Column, plan.Sort.Order)
		}
		if plan.NeedOtherTables {
			planSummary += "\nNeedOtherTables: true"
		}
	} else {
		planSummary = "(nil)"
	}

	entry := fmt.Sprintf(
		"[RESPONSE] %s  |  Op: QueryPlan\n"+
			"────────────────────────────────────────\n"+
			"Error: %s\n"+
			"────────────────────────────────────────\n"+
			"Raw AI Response:\n%s\n"+
			"────────────────────────────────────────\n"+
			"Parsed Plan:\n%s\n"+
			"────────────────────────────────────────\n"+
			"Generated SQL:\n%s\n"+
			"════════════════════════════════════════════════════════════════\n\n",
		ts, errStr, rawResponse, planSummary, sql,
	)
	logWrite(entry)
}
