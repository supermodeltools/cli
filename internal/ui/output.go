package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// Format controls how command output is rendered.
type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
)

// ParseFormat converts a string to a Format, defaulting to FormatHuman.
func ParseFormat(s string) Format {
	if s == "json" {
		return FormatJSON
	}
	return FormatHuman
}

// Step prints a progress step to stderr.
func Step(msg string) {
	fmt.Fprintf(os.Stderr, "→ %s\n", msg)
}

// Success prints a success message to stderr.
func Success(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "✓ "+format+"\n", args...)
}

// Warn prints a warning to stderr.
func Warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}

// JSON writes v as indented JSON to w.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table renders rows as an aligned table with a header and separator.
func Table(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	seps := make([]string, len(headers))
	for i, h := range headers {
		seps[i] = strings.Repeat("-", len(h))
	}
	fmt.Fprintln(tw, strings.Join(seps, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	tw.Flush()
}

// Spinner provides simple braille-spinner progress indication on stderr.
type Spinner struct {
	done chan struct{}
}

// Start creates and begins a spinner with the given message.
func Start(msg string) *Spinner {
	s := &Spinner{done: make(chan struct{})}
	go s.run(msg)
	return s
}

// Stop halts the spinner and clears its line.
func (s *Spinner) Stop() {
	close(s.done)
	time.Sleep(10 * time.Millisecond) // let goroutine clear the line
}

func (s *Spinner) run(msg string) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			fmt.Fprintf(os.Stderr, "\r%-70s\r", "")
			return
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "\r%s %s", frames[i%len(frames)], msg)
			i++
		}
	}
}
