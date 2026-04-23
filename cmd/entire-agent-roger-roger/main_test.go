package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestDecodeStoredSessionRoundTrip(t *testing.T) {
	want := agentSessionJSON{
		SessionID:  "test-session",
		AgentName:  agentName,
		RepoPath:   "/tmp/repo",
		SessionRef: "/tmp/repo/.roger-roger/sessions/test-session.jsonl",
		StartTime:  "2026-03-24T13:43:02Z",
		NativeData: []byte(`{"test":true}`),
		ModFiles:   []string{"file1.go", "file2.go"},
		NewFiles:   []string{"file3.go"},
		DelFiles:   []string{"file4.go"},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}

	got, ok := decodeStoredSession(data)
	if !ok {
		t.Fatal("decodeStoredSession reported failure for a valid stored session")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded session mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestDecodeStoredSessionRejectsNonSessionJSON(t *testing.T) {
	if _, ok := decodeStoredSession([]byte(`{"test":"data"}`)); ok {
		t.Fatal("decodeStoredSession accepted unrelated JSON as a stored session")
	}
}

func TestParseHookInputEmptyReturnsNil(t *testing.T) {
	got, err := parseHookInput(nil)
	if err != nil {
		t.Fatalf("parseHookInput returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("parseHookInput should return nil for empty stdin, got %#v", got)
	}
}

func TestBuildHookEventUsesProtocolFields(t *testing.T) {
	timestamp := "2026-03-24T13:43:02Z"
	event := buildHookEvent("user-prompt-submit", parsedHookInput{
		SessionID:  "test-session",
		SessionRef: "/tmp/repo/transcript.jsonl",
		UserPrompt: "fix the failure",
		Timestamp:  timestamp,
	})
	if event == nil {
		t.Fatal("buildHookEvent returned nil for a supported hook")
	}
	if event.Type != eventTurnStart {
		t.Fatalf("event type mismatch: got %d want %d", event.Type, eventTurnStart)
	}
	if event.SessionID != "test-session" {
		t.Fatalf("session_id mismatch: got %q", event.SessionID)
	}
	if event.SessionRef != "/tmp/repo/transcript.jsonl" {
		t.Fatalf("session_ref mismatch: got %q", event.SessionRef)
	}
	if event.Prompt != "fix the failure" {
		t.Fatalf("prompt mismatch: got %q", event.Prompt)
	}
	if event.Timestamp != timestamp {
		t.Fatalf("timestamp mismatch: got %q want %q", event.Timestamp, timestamp)
	}
}

func TestBuildHookEventSupportsLegacyFieldNames(t *testing.T) {
	event := buildHookEvent("session-end", parsedHookInput{
		SessionID:      "legacy-session",
		TranscriptPath: "/tmp/repo/legacy.jsonl",
		Prompt:         "legacy prompt",
	})
	if event == nil {
		t.Fatal("buildHookEvent returned nil for a supported hook")
	}
	if event.Type != eventSessionEnd {
		t.Fatalf("event type mismatch: got %d want %d", event.Type, eventSessionEnd)
	}
	if event.SessionRef != "/tmp/repo/legacy.jsonl" {
		t.Fatalf("session_ref mismatch: got %q", event.SessionRef)
	}
	if event.Prompt != "legacy prompt" {
		t.Fatalf("prompt mismatch: got %q", event.Prompt)
	}
	if _, err := time.Parse(time.RFC3339, event.Timestamp); err != nil {
		t.Fatalf("timestamp is not RFC3339: %q (%v)", event.Timestamp, err)
	}
}

func TestChunkJSONLRejectsNonPositiveMaxSize(t *testing.T) {
	if _, err := chunkJSONL([]byte("test"), 0); err == nil {
		t.Fatal("chunkJSONL should reject maxSize <= 0")
	}
}

func TestBuildCompactTranscriptConvertsNativeTranscript(t *testing.T) {
	raw := []byte(
		`{"type":"user","uuid":"user-1","timestamp":"2026-04-23T18:00:00Z","message":{"content":"Fix the login bug"}}` + "\n" +
			`{"type":"assistant","uuid":"assistant-1","timestamp":"2026-04-23T18:00:02Z","message":{"content":[{"type":"text","text":"I updated the file."}]}}` + "\n",
	)

	got, err := buildCompactTranscript(raw, agentName, "0.42.0")
	if err != nil {
		t.Fatalf("buildCompactTranscript returned error: %v", err)
	}

	want := []byte(
		`{"v":1,"agent":"roger-roger","cli_version":"0.42.0","type":"user","ts":"2026-04-23T18:00:00Z","content":[{"text":"Fix the login bug"}]}` + "\n" +
			`{"v":1,"agent":"roger-roger","cli_version":"0.42.0","type":"assistant","ts":"2026-04-23T18:00:02Z","id":"assistant-1","content":[{"type":"text","text":"I updated the file."}]}` + "\n",
	)

	if !bytes.Equal(got, want) {
		t.Fatalf("compact transcript mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestBuildCompactTranscriptRejectsAssistantWithoutText(t *testing.T) {
	raw := []byte(`{"type":"assistant","uuid":"assistant-1","message":{"content":[]}}` + "\n")

	if _, err := buildCompactTranscript(raw, agentName, "0.42.0"); err == nil {
		t.Fatal("buildCompactTranscript should reject assistant messages without text blocks")
	}
}

func TestEncodeBase64UsesStdEncoding(t *testing.T) {
	data := []byte("hello\n")
	got := encodeBase64(data)
	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Fatalf("decoded payload mismatch: got %q want %q", decoded, data)
	}
}
