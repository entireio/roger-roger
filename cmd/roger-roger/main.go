package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fatal("getwd: %v", err)
	}

	sessionID := uuid.New().String()
	transcriptPath := setupTranscript(sessionID)

	fireHook(dir, "session-start", map[string]string{
		"session_id":      sessionID,
		"transcript_path": transcriptPath,
	})

	fmt.Fprint(os.Stdout, "> ")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "exit" || line == "quit" {
			break
		}
		runTurn(dir, sessionID, transcriptPath, line)
		fmt.Fprint(os.Stdout, "> ")
	}

	fireHook(dir, "session-end", map[string]string{
		"session_id":      sessionID,
		"transcript_path": transcriptPath,
	})
}

func runTurn(dir, sessionID, transcriptPath, prompt string) {
	fireHook(dir, "user-prompt-submit", map[string]any{
		"session_id":      sessionID,
		"transcript_path": transcriptPath,
		"prompt":          prompt,
	})

	appendTranscript(transcriptPath, "user", prompt)

	response := handlePrompt(dir, prompt)

	appendTranscript(transcriptPath, "assistant", response)
	fmt.Fprintln(os.Stdout, response)

	fireHook(dir, "stop", map[string]string{
		"session_id":      sessionID,
		"transcript_path": transcriptPath,
	})
}

// --- Prompt Handling ---

var createFileRe = regexp.MustCompile(`(?i)create\s+(?:a\s+)?(?:\w+\s+)*?file\s+(?:at\s+|called\s+)?([^\s,]+\.\w+)`)

func handlePrompt(dir, prompt string) string {
	if m := createFileRe.FindStringSubmatch(prompt); m != nil {
		path := m[1]
		createFile(dir, path, fmt.Sprintf("# %s\n", path))
		return fmt.Sprintf("Created %s.", path)
	}
	return "roger roger"
}

func createFile(dir, path, content string) {
	abs := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", filepath.Dir(abs), err)
		return
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", abs, err)
	}
}

// --- Hook Firing ---

func fireHook(dir, hookName string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	cmd := exec.Command("entire", "hooks", "roger-roger", hookName)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(data)
	cmd.Env = append(os.Environ(), "ENTIRE_TEST_TTY=0")
	cmd.CombinedOutput()
}

// --- Transcript ---

// transcriptLine matches the standard JSONL transcript format used by Claude Code / Cursor.
// The "message" field is a raw JSON object whose structure depends on the "type".
type transcriptLine struct {
	Type      string          `json:"type"`
	UUID      string          `json:"uuid"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type userMessage struct {
	Content string `json:"content"`
}

type assistantMessage struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func setupTranscript(sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		fatal("home dir: %v", err)
	}
	transcriptDir := filepath.Join(home, ".roger-roger", "sessions")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		fatal("mkdir transcript dir: %v", err)
	}
	path := filepath.Join(transcriptDir, sessionID+".jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		fatal("create transcript: %v", err)
	}
	return path
}

func appendTranscript(path, role, content string) {
	var msg json.RawMessage
	switch role {
	case "user":
		msg, _ = json.Marshal(userMessage{Content: content})
	case "assistant":
		msg, _ = json.Marshal(assistantMessage{
			Content: []contentBlock{{Type: "text", Text: content}},
		})
	}

	entry := transcriptLine{
		Type:      role,
		UUID:      uuid.New().String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message:   msg,
	}
	data, _ := json.Marshal(entry)
	data = append(data, '\n')
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "roger-roger: "+format+"\n", args...)
	os.Exit(1)
}
