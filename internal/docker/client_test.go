package docker

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func frame(stream byte, payload string) []byte {
	buf := make([]byte, 8+len(payload))
	buf[0] = stream
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(payload)))
	copy(buf[8:], payload)
	return buf
}

func TestParseMultiplexedLogsSplitsStreamsAndTimestamps(t *testing.T) {
	var body bytes.Buffer
	body.Write(frame(1, "2026-09-03T10:00:00.123456789Z hello\n"))
	body.Write(frame(2, "2026-09-03T10:00:01.000000000Z oops\nsecond line without stamp\n"))

	lines, err := parseMultiplexedLogs(&body)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0].Stream != "stdout" || lines[0].Text != "hello" || lines[0].Time.IsZero() {
		t.Fatalf("bad first line %+v", lines[0])
	}
	if lines[1].Stream != "stderr" || lines[1].Text != "oops" {
		t.Fatalf("bad second line %+v", lines[1])
	}
	if lines[2].Text != "second line without stamp" || !lines[2].Time.IsZero() {
		t.Fatalf("bad third line %+v", lines[2])
	}
}

func TestParseRawLogsForTTY(t *testing.T) {
	body := bytes.NewBufferString("2026-09-03T10:00:00Z one\r\n2026-09-03T10:00:01Z two\n")
	lines, err := parseRawLogs(body, "stdout")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0].Text != "one" || lines[1].Text != "two" {
		t.Fatalf("unexpected %+v", lines)
	}
}
