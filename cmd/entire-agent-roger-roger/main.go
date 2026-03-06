package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"strconv"
	"time"
)

const agentName = "roger-roger"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: entire-agent-roger-roger <subcommand> [args...]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "info":
		cmdInfo()
	case "detect":
		cmdDetect()
	case "get-session-id":
		cmdGetSessionID()
	case "get-session-dir":
		cmdGetSessionDir()
	case "resolve-session-file":
		cmdResolveSessionFile()
	case "read-session":
		cmdReadSession()
	case "write-session":
		cmdWriteSession()
	case "read-transcript":
		cmdReadTranscript()
	case "chunk-transcript":
		cmdChunkTranscript()
	case "reassemble-transcript":
		cmdReassembleTranscript()
	case "format-resume-command":
		cmdFormatResumeCommand()
	case "parse-hook":
		cmdParseHook()
	case "install-hooks":
		cmdInstallHooks()
	case "uninstall-hooks":
		// no-op
	case "are-hooks-installed":
		cmdAreHooksInstalled()
	case "write-hook-response":
		cmdWriteHookResponse()
	case "get-transcript-position":
		cmdGetTranscriptPosition()
	case "extract-modified-files":
		cmdExtractModifiedFiles()
	case "extract-prompts":
		cmdExtractPrompts()
	case "extract-summary":
		cmdExtractSummary()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// --- Protocol types ---

type infoResponse struct {
	ProtocolVersion int          `json:"protocol_version"`
	Name            string       `json:"name"`
	Type            string       `json:"type"`
	Description     string       `json:"description"`
	IsPreview       bool         `json:"is_preview"`
	ProtectedDirs   []string     `json:"protected_dirs"`
	HookNames       []string     `json:"hook_names"`
	Capabilities    declaredCaps `json:"capabilities"`
}

type declaredCaps struct {
	Hooks              bool `json:"hooks"`
	TranscriptAnalyzer bool `json:"transcript_analyzer"`
	TranscriptPreparer bool `json:"transcript_preparer"`
	TokenCalculator    bool `json:"token_calculator"`
	TextGenerator      bool `json:"text_generator"`
	HookResponseWriter bool `json:"hook_response_writer"`
}

type hookInputJSON struct {
	HookType   string `json:"hook_type"`
	SessionID  string `json:"session_id"`
	SessionRef string `json:"session_ref"`
	Timestamp  string `json:"timestamp"`
	UserPrompt string `json:"user_prompt,omitempty"`
}

type agentSessionJSON struct {
	SessionID  string   `json:"session_id"`
	AgentName  string   `json:"agent_name"`
	RepoPath   string   `json:"repo_path"`
	SessionRef string   `json:"session_ref"`
	StartTime  string   `json:"start_time"`
	NativeData []byte   `json:"native_data"`
	ModFiles   []string `json:"modified_files"`
	NewFiles   []string `json:"new_files"`
	DelFiles   []string `json:"deleted_files"`
}

// transcriptLine matches the standard JSONL transcript format (Claude Code / Cursor).
type transcriptLine struct {
	Type    string          `json:"type"`
	UUID    string          `json:"uuid"`
	Message json.RawMessage `json:"message"`
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

type eventJSON struct {
	Type       int    `json:"type"`
	SessionID  string `json:"session_id"`
	SessionRef string `json:"session_ref,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	Timestamp  string `json:"timestamp,omitempty"`
}

const (
	eventSessionStart = 1
	eventTurnStart    = 2
	eventTurnEnd      = 3
	eventSessionEnd   = 5
)

// --- Subcommands ---

func cmdInfo() {
	writeJSON(infoResponse{
		ProtocolVersion: 1,
		Name:            agentName,
		Type:            "Roger Roger Agent",
		Description:     "Roger Roger - just a test agent",
		ProtectedDirs:   []string{".roger-roger"},
		HookNames:       []string{"session-start", "session-end", "stop", "user-prompt-submit"},
		Capabilities: declaredCaps{
			Hooks:              true,
			HookResponseWriter: true,
			TranscriptAnalyzer: true,
		},
	})
}

func cmdDetect() {
	writeJSON(map[string]bool{"present": false})
}

func cmdGetSessionID() {
	var input hookInputJSON
	readJSONStdin(&input)
	writeJSON(map[string]string{"session_id": input.SessionID})
}

func cmdGetSessionDir() {
	_ = getFlag("--repo-path")
	if override := os.Getenv("ENTIRE_TEST_ROGER_ROGER_PROJECT_DIR"); override != "" {
		writeJSON(map[string]string{"session_dir": override})
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fatal("get home dir: %v", err)
	}
	writeJSON(map[string]string{"session_dir": filepath.Join(home, ".roger-roger", "sessions")})
}

func cmdResolveSessionFile() {
	sessionDir := getFlag("--session-dir")
	sessionID := getFlag("--session-id")
	writeJSON(map[string]string{
		"session_file": filepath.Join(sessionDir, sessionID+".jsonl"),
	})
}

func cmdReadSession() {
	var input hookInputJSON
	readJSONStdin(&input)
	resp := agentSessionJSON{
		SessionID:  input.SessionID,
		AgentName:  agentName,
		SessionRef: input.SessionRef,
		StartTime:  time.Now().Format(time.RFC3339),
	}
	if input.SessionRef != "" {
		data, err := os.ReadFile(input.SessionRef)
		if err == nil {
			resp.NativeData = data
		}
	}
	writeJSON(resp)
}

func cmdWriteSession() {
	var session agentSessionJSON
	readJSONStdin(&session)
	if session.SessionRef == "" {
		fatal("session reference is required")
	}
	if err := os.MkdirAll(filepath.Dir(session.SessionRef), 0o750); err != nil {
		fatal("create session dir: %v", err)
	}
	if err := os.WriteFile(session.SessionRef, session.NativeData, 0o600); err != nil {
		fatal("write session: %v", err)
	}
}

func cmdReadTranscript() {
	sessionRef := getFlag("--session-ref")
	data, err := os.ReadFile(sessionRef)
	if err != nil {
		fatal("read transcript: %v", err)
	}
	os.Stdout.Write(data)
}

func cmdChunkTranscript() {
	maxSize, err := strconv.Atoi(getFlag("--max-size"))
	if err != nil {
		fatal("invalid --max-size: %v", err)
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal("read stdin: %v", err)
	}
	writeJSON(map[string][][]byte{"chunks": chunkJSONL(data, maxSize)})
}

func cmdReassembleTranscript() {
	var input struct {
		Chunks [][]byte `json:"chunks"`
	}
	readJSONStdin(&input)
	var result []byte
	for _, chunk := range input.Chunks {
		result = append(result, chunk...)
	}
	os.Stdout.Write(result)
}

func cmdFormatResumeCommand() {
	sessionID := getFlag("--session-id")
	writeJSON(map[string]string{
		"command": "roger-roger --session-id " + sessionID,
	})
}

func cmdParseHook() {
	hookName := getFlag("--hook")
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal("read stdin: %v", err)
	}

	var raw struct {
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
		Prompt         string `json:"prompt"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		fatal("parse hook input: %v", err)
	}

	ts := time.Now().Format(time.RFC3339)
	var eventType int
	switch hookName {
	case "session-start":
		eventType = eventSessionStart
	case "user-prompt-submit":
		eventType = eventTurnStart
	case "stop":
		eventType = eventTurnEnd
	case "session-end":
		eventType = eventSessionEnd
	default:
		fmt.Fprint(os.Stdout, "null")
		return
	}

	writeJSON(eventJSON{
		Type:       eventType,
		SessionID:  raw.SessionID,
		SessionRef: raw.TranscriptPath,
		Prompt:     raw.Prompt,
		Timestamp:  ts,
	})
}

func cmdInstallHooks() {
	writeJSON(map[string]int{"hooks_installed": 0})
}

func cmdAreHooksInstalled() {
	writeJSON(map[string]bool{"installed": false})
}

func cmdWriteHookResponse() {
	fmt.Fprintln(os.Stdout, getFlag("--message"))
}

// --- TranscriptAnalyzer subcommands ---

func cmdGetTranscriptPosition() {
	path := getFlag("--path")
	lines := readTranscriptLines(path)
	writeJSON(map[string]int{"position": len(lines)})
}

func cmdExtractModifiedFiles() {
	path := getFlag("--path")
	offset, err := strconv.Atoi(getFlag("--offset"))
	if err != nil {
		fatal("invalid --offset: %v", err)
	}

	lines := readTranscriptLines(path)
	var files []string
	seen := map[string]bool{}

	for i := offset; i < len(lines); i++ {
		var line transcriptLine
		if err := json.Unmarshal(lines[i], &line); err != nil {
			continue
		}
		if line.Type != "assistant" {
			continue
		}
		text := extractAssistantText(line.Message)
		if m := createdFileRe.FindStringSubmatch(text); m != nil {
			f := m[1]
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}

	writeJSON(map[string]any{
		"files":            files,
		"current_position": len(lines),
	})
}

func cmdExtractPrompts() {
	sessionRef := getFlag("--session-ref")
	offset, err := strconv.Atoi(getFlag("--offset"))
	if err != nil {
		fatal("invalid --offset: %v", err)
	}

	lines := readTranscriptLines(sessionRef)
	var prompts []string

	for i := offset; i < len(lines); i++ {
		var line transcriptLine
		if err := json.Unmarshal(lines[i], &line); err != nil {
			continue
		}
		if line.Type == "user" {
			text := extractUserText(line.Message)
			if text != "" {
				prompts = append(prompts, text)
			}
		}
	}

	writeJSON(map[string][]string{"prompts": prompts})
}

func cmdExtractSummary() {
	sessionRef := getFlag("--session-ref")
	lines := readTranscriptLines(sessionRef)

	var prompts []string
	for _, raw := range lines {
		var line transcriptLine
		if err := json.Unmarshal(raw, &line); err != nil {
			continue
		}
		if line.Type == "user" {
			text := extractUserText(line.Message)
			if text != "" {
				prompts = append(prompts, text)
			}
		}
	}

	if len(prompts) == 0 {
		writeJSON(map[string]any{"summary": "", "has_summary": false})
		return
	}

	summary := prompts[0]
	if len(summary) > 100 {
		summary = summary[:100] + "..."
	}
	writeJSON(map[string]any{"summary": summary, "has_summary": true})
}

var createdFileRe = regexp.MustCompile(`^Created ([^\s]+)\.$`)

// extractUserText extracts the text content from a user message.
func extractUserText(raw json.RawMessage) string {
	var msg userMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}
	return msg.Content
}

// extractAssistantText extracts concatenated text from an assistant message's content blocks.
func extractAssistantText(raw json.RawMessage) string {
	var msg assistantMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}
	var texts []string
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// readTranscriptLines reads a JSONL file and returns non-empty lines.
func readTranscriptLines(path string) [][]byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines [][]byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

// --- Helpers ---

func getFlag(name string) string {
	for i := 1; i < len(os.Args)-1; i++ {
		if os.Args[i] == name {
			return os.Args[i+1]
		}
	}
	fatal("missing required flag: %s", name)
	return ""
}

func writeJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		fatal("marshal JSON: %v", err)
	}
	os.Stdout.Write(data)
}

func readJSONStdin(v any) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal("read stdin: %v", err)
	}
	if len(data) == 0 {
		fatal("empty stdin")
	}
	if err := json.Unmarshal(data, v); err != nil {
		fatal("parse JSON: %v", err)
	}
}

func chunkJSONL(data []byte, maxSize int) [][]byte {
	if len(data) == 0 {
		return nil
	}
	if maxSize <= 0 || len(data) <= maxSize {
		return [][]byte{data}
	}
	var chunks [][]byte
	var current []byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		lineWithNewline := append(line, '\n')
		if len(current)+len(lineWithNewline) > maxSize && len(current) > 0 {
			chunks = append(chunks, current)
			current = nil
		}
		current = append(current, lineWithNewline...)
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "entire-agent-roger-roger: "+format+"\n", args...)
	os.Exit(1)
}
