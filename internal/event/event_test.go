package event

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestProtocolIsOneJSONObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	em := New(&buf)

	if err := em.Step("preflight", "Checking node", 5); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if err := em.Progress("helm", Running, 55, "Installing chart"); err != nil {
		t.Fatalf("Progress: %v", err)
	}
	if err := em.Done("https://naslos.local/authelia"); err != nil {
		t.Fatalf("Done: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %q", len(lines), buf.String())
	}
	for i, line := range lines {
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d is not JSON: %v (%q)", i, err, line)
		}
		if strings.ContainsAny(line, "\n") {
			t.Fatalf("line %d contains a newline", i)
		}
	}

	var last Event
	if err := json.Unmarshal([]byte(lines[2]), &last); err != nil {
		t.Fatal(err)
	}
	if last.Step != "done" || last.Status != OK || last.Pct == nil || *last.Pct != 100 {
		t.Fatalf("unexpected terminal event: %+v", last)
	}
}

func TestProgressDataCarriesStepPayload(t *testing.T) {
	var buf bytes.Buffer
	em := New(&buf)

	uri := "otpauth://totp/naslos.local:admin?secret=JBSWY3DPEHPK3PXP"
	if err := em.ProgressData("totp", Running, 94, "Scan this code", map[string]string{
		"otpauth": uri,
		"secret":  "JBSWY3DPEHPK3PXP",
	}); err != nil {
		t.Fatalf("ProgressData: %v", err)
	}

	var ev Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Step != "totp" || ev.Data["otpauth"] != uri {
		t.Fatalf("unexpected data event: %+v", ev)
	}
}

func TestFailEmitsErrorEventAndReturnsError(t *testing.T) {
	var buf bytes.Buffer
	em := New(&buf)

	err := em.Fail("bootstrap", "admin creation failed", "HTTP 401")
	if err == nil || !strings.Contains(err.Error(), "admin creation failed") {
		t.Fatalf("want error, got %v", err)
	}

	var ev Event
	line := strings.TrimSpace(buf.String())
	if e := json.Unmarshal([]byte(line), &ev); e != nil {
		t.Fatalf("unmarshal: %v", e)
	}
	if ev.Error == nil || ev.Error.Step != "bootstrap" || ev.Error.Output != "HTTP 401" {
		t.Fatalf("unexpected error event: %+v", ev)
	}
	if ev.Step != "" || ev.Status != "" {
		t.Fatalf("error event must not also carry step/status: %+v", ev)
	}
}
