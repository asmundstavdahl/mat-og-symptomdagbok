package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// parseTimestamp parses a timestamp string in the format "2006-01-02T15:04".
// This function is no longer used directly for parsing form inputs in main.go
// because time.ParseInLocation is used there to handle local time correctly.
func parseTimestamp(timestampStr string) (time.Time, error) {
	return time.Parse(timestampFormat, timestampStr)
}

// parseRFC3339 parses a timestamp string in RFC3339 format.
// It assumes the input string is in UTC and returns a time.Time in UTC.
func parseRFC3339(timestampStr string) (time.Time, error) {
	return time.Parse(time.RFC3339, timestampStr)
}

// parseDateOnly parses a date string in the format "2006-01-02".
func parseDateOnly(dateStr string) (time.Time, error) {
	return time.Parse(dateFormat, dateStr)
}

// writeJSONError writes an error response as JSON.
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// writeJSONResponse writes a successful JSON response.
func writeJSONResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
}
