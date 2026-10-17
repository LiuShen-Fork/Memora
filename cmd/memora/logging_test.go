package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The log page asks for as much history as it means to show, and the stream
// answers with that much of the file rather than with a fixed window.
func TestTheLogStreamSendsTheHistoryThePageAsksFor(t *testing.T) {
	app := newTestApp(t)
	app.logs = NewLogBuffer(filepath.Join(t.TempDir(), "logs", "app.log"))
	for i := 1; i <= 420; i++ {
		app.logs.Add("queue", "line "+itoa(int64(i)))
	}

	whole := readLogStream(t, app, "/api/system/logs?initial=all", "")
	if count := strings.Count(whole, "data: "); count != 420 {
		t.Fatalf("the whole log is %d lines, want 420", count)
	}
	if !strings.Contains(whole, "[queue] line 1\n") {
		t.Fatal("the whole log does not reach back to the first line")
	}
	// Every line says where it ends, which is what a stream the browser has to
	// renew continues from.
	if lastLogOffset(t, whole) == "" {
		t.Fatal("the stream tagged no line with the position it ends at")
	}

	// A page that asks for less is sent less, from the end of the log.
	recent := readLogStream(t, app, "/api/system/logs?initial=5", "")
	if count := strings.Count(recent, "data: "); count != 5 {
		t.Fatalf("the last five lines are %d lines", count)
	}
	if strings.Contains(recent, "[queue] line 415") || !strings.Contains(recent, "[queue] line 416") {
		t.Fatal("five lines are not the last five lines")
	}

	// A page that asks for nothing in particular keeps the modest backlog the
	// endpoint has always answered with.
	backlog := readLogStream(t, app, "/api/system/logs", "")
	if count := strings.Count(backlog, "data: "); count != 400 {
		t.Fatalf("the default backlog is %d lines, want 400", count)
	}
	if strings.Contains(backlog, "[queue] line 20\n") {
		t.Fatal("the default backlog reaches further back than it says")
	}
}

// A stream that the browser has to renew continues where it stopped: the page
// keeps what it has, and is not handed the history it already shows again.
func TestARenewedLogStreamCarriesOnWhereItStopped(t *testing.T) {
	app := newTestApp(t)
	app.logs = NewLogBuffer(filepath.Join(t.TempDir(), "logs", "app.log"))
	app.logs.Add("queue", "line 1")
	app.logs.Add("queue", "line 2")
	first := readLogStream(t, app, "/api/system/logs?initial=all", "")

	app.logs.Add("queue", "line 3")
	renewed := readLogStream(t, app, "/api/system/logs?initial=all", lastLogOffset(t, first))
	if strings.Contains(renewed, "line 1") || strings.Contains(renewed, "line 2") {
		t.Fatalf("a renewed stream read the history again: %q", renewed)
	}
	if count := strings.Count(renewed, "data: "); count != 1 || !strings.Contains(renewed, "[queue] line 3\n") {
		t.Fatalf("a renewed stream carried on with %q", renewed)
	}
	if strings.Contains(renewed, "data: ") && lastLogOffset(t, renewed) == lastLogOffset(t, first) {
		t.Fatal("the renewed stream would continue from where it was renewed")
	}
}

// A line is handed over once it is whole, and every line says where it ends so
// that a reader can pick the file up again from any of them.
func TestTheLogFileIsReadFromWhereTheLastReadStopped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "app.log")
	buffer := NewLogBuffer(path)
	buffer.Add("server", "first")
	buffer.Add("server", "second")

	written, size, err := buffer.Tail(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 || logText(written[0].text) != "[server] first" || logText(written[1].text) != "[server] second" {
		t.Fatalf("the log reads %+v", written)
	}
	if written[len(written)-1].offset != size {
		t.Fatalf("the last line ends at %d, and the file is %d long", written[len(written)-1].offset, size)
	}
	// A reader that is up to date is handed nothing rather than the same lines
	// over again.
	if lines, _, err := buffer.Since(size); err != nil || len(lines) != 0 {
		t.Fatalf("a reader up to date reads %+v (%v)", lines, err)
	}

	appendToFile(t, path, "[server] half")
	if lines, _, err := buffer.Since(size); err != nil || len(lines) != 0 {
		t.Fatalf("a half-written line was handed over: %+v (%v)", lines, err)
	}
	appendToFile(t, path, " done\n")
	finished, next, err := buffer.Since(size)
	if err != nil {
		t.Fatal(err)
	}
	if len(finished) != 1 || finished[0].text != "[server] half done" || finished[0].offset != next {
		t.Fatalf("the finished line reads %+v, and the file is %d long", finished, next)
	}

	// A log that was rolled away and started over is read from its beginning
	// rather than from a position it no longer has.
	if err := os.WriteFile(path, []byte("[server] fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rolled, _, err := buffer.Since(next)
	if err != nil {
		t.Fatal(err)
	}
	if len(rolled) != 1 || rolled[0].text != "[server] fresh" {
		t.Fatalf("a log smaller than the reader's position reads %+v", rolled)
	}
}

// A line says when it was written and who wrote it. The page shows both, and
// the log file is read by people long after the fact, so the stamp has to
// survive into the file and out of the stream.
func TestEveryLineIsStampedWithTheMomentItWasWritten(t *testing.T) {
	app := newTestApp(t)
	app.logs = NewLogBuffer(filepath.Join(t.TempDir(), "logs", "app.log"))
	before := time.Now().Add(-time.Second)
	app.logs.Add("queue", "processed photo abc")
	after := time.Now().Add(time.Second)

	stream := readLogStream(t, app, "/api/system/logs?initial=all", "")
	stamped, ok := strings.CutPrefix(stream, "id: "+lastLogOffset(t, stream)+"\ndata: ")
	if !ok {
		t.Fatalf("the stream opened with %q", stream)
	}
	line, rest, ok := strings.Cut(strings.TrimSuffix(stamped, "\n\n"), " [queue] ")
	if !ok || rest != "processed photo abc" {
		t.Fatalf("the line reads %q", stamped)
	}
	written, err := time.Parse(logLineTimeFormat, line)
	if err != nil {
		t.Fatalf("the line is stamped %q: %v", line, err)
	}
	if written.Before(before) || written.After(after) {
		t.Fatalf("the line claims to have been written at %s, outside %s..%s", written, before, after)
	}

	// The file is where the history lives, so the same stamp has to be in it.
	saved, err := os.ReadFile(app.logs.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), line+" [queue] processed photo abc\n") {
		t.Fatalf("the log file holds %q", saved)
	}
}

// logText is a line as it was written, without the moment it was written at.
func logText(line string) string {
	if cut, rest, ok := strings.Cut(line, " ["); ok && cut != "" {
		return "[" + rest
	}
	return line
}

// readLogStream asks for the log stream and answers with what it wrote before it
// settled down to wait for new lines: the request is already over, so the stream
// ends as soon as it opens and the test does not wait thirty seconds for it.
func readLogStream(t *testing.T, app *App, target, resumedAt string) string {
	t.Helper()
	request := adminRequest(t, app, http.MethodGet, target, nil)
	if resumedAt != "" {
		request.Header.Set("Last-Event-ID", resumedAt)
	}
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	cancel()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request.WithContext(ctx))
	return response.Body.String()
}

// lastLogOffset is the position the last tagged line of a stream ends at.
func lastLogOffset(t *testing.T, body string) string {
	t.Helper()
	offset := ""
	for _, line := range strings.Split(body, "\n") {
		if value, ok := strings.CutPrefix(line, "id: "); ok {
			offset = strings.TrimSpace(value)
		}
	}
	return offset
}

func appendToFile(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
