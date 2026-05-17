package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type fileWithTime struct {
	path  string
	mtime time.Time
}

func defaultSessionsDir() string {
	if value := os.Getenv("CODEX_SESSIONS_DIR"); value != "" {
		return expandHome(value)
	}
	if value := os.Getenv("CODEX_HOME"); value != "" {
		return filepath.Join(expandHome(value), "sessions")
	}
	return filepath.Join(homeDir(), ".codex", "sessions")
}

func collectSessions(root string, limit int, cwdFilter string) ([]SessionSummary, error) {
	if limit <= 0 {
		return []SessionSummary{}, nil
	}
	files, err := listSessionFiles(root)
	if err != nil {
		return nil, err
	}
	if cwdFilter == "" {
		maxRead := limit * 4
		if maxRead < limit || maxRead < 40 {
			maxRead = 40
		}
		if len(files) > maxRead {
			files = files[:maxRead]
		}
	}
	sessions := make([]SessionSummary, 0, limit)
	for _, file := range files {
		summary, err := summarizeSession(file.path)
		if err != nil {
			continue
		}
		if !cwdMatches(summary.CWD, cwdFilter) {
			continue
		}
		sessions = append(sessions, summary)
		if len(sessions) >= limit {
			break
		}
	}
	return sessions, nil
}

func listSessionFiles(root string) ([]fileWithTime, error) {
	var files []fileWithTime
	err := filepath.WalkDir(expandHome(root), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		files = append(files, fileWithTime{path: path, mtime: info.ModTime()})
		return nil
	})
	if os.IsNotExist(err) {
		return files, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime.After(files[j].mtime)
	})
	return files, nil
}

func summarizeSession(file string) (SessionSummary, error) {
	info, err := os.Stat(file)
	if err != nil {
		return SessionSummary{}, err
	}
	summary := SessionSummary{
		ID:        sessionIDFromPath(file),
		Path:      file,
		UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
		Messages:  []MessageBrief{},
	}
	handle, err := os.Open(file)
	if err != nil {
		return summary, err
	}
	defer handle.Close()

	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		var item map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			continue
		}
		if ts, ok := item["timestamp"].(string); ok && ts != "" {
			summary.UpdatedAt = normalizeTime(ts)
		}
		switch item["type"] {
		case "session_meta":
			payload := asMap(item["payload"])
			summary.ID = str(payload["id"], summary.ID)
			summary.Timestamp = normalizeTime(str(payload["timestamp"], summary.Timestamp))
			summary.CWD = str(payload["cwd"], summary.CWD)
			summary.Source = str(payload["source"], summary.Source)
			summary.Model = str(payload["model"], summary.Model)
		case "event_msg":
			payload := asMap(item["payload"])
			if payload["type"] == "token_count" && payload["rate_limits"] != nil {
				var rate RateLimits
				data, _ := json.Marshal(payload["rate_limits"])
				if json.Unmarshal(data, &rate) == nil {
					summary.Quota = &rate
				}
			}
		case "response_item":
			payload := asMap(item["payload"])
			if payload["type"] != "message" {
				continue
			}
			role := str(payload["role"], "")
			if role != "user" && role != "assistant" {
				continue
			}
			text := truncate(textFromContent(payload["content"]), 180)
			if text == "" || strings.HasPrefix(text, "--- SUPERVISION ---") {
				continue
			}
			summary.Messages = append(summary.Messages, MessageBrief{Role: role, Text: text})
			if len(summary.Messages) > 8 {
				summary.Messages = summary.Messages[len(summary.Messages)-8:]
			}
		}
	}
	return summary, scanner.Err()
}

func cwdMatches(sessionCWD string, filter string) bool {
	if filter == "" {
		return true
	}
	if sessionCWD == "" {
		return false
	}
	filterPath, err := filepath.Abs(expandHome(filter))
	if err != nil {
		filterPath = filepath.Clean(expandHome(filter))
	}
	sessionPath, err := filepath.Abs(expandHome(sessionCWD))
	if err != nil {
		sessionPath = filepath.Clean(expandHome(sessionCWD))
	}
	rel, err := filepath.Rel(filterPath, sessionPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func formatSession(summary SessionSummary) string {
	lines := []string{
		summary.ID + "  " + summary.UpdatedAt,
		"  cwd: " + emptyAs(summary.CWD, "(unknown)"),
	}
	if quota := formatQuota(summary.Quota); quota != "" {
		lines = append(lines, "  quota: "+quota)
	}
	for _, msg := range lastMessages(summary.Messages, 5) {
		prefix := "A"
		if msg.Role == "user" {
			prefix = "U"
		}
		lines = append(lines, "  "+prefix+": "+msg.Text)
	}
	lines = append(lines, "  resume: "+appName+" resume --session "+summary.ID)
	return strings.Join(lines, "\n")
}

func textFromContent(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
				continue
			}
			m := asMap(item)
			for _, key := range []string{"text", "input_text", "output_text"} {
				if text := str(m[key], ""); text != "" {
					parts = append(parts, text)
					break
				}
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func sessionIDFromPath(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), ".jsonl")
	idx := strings.LastIndex(base, "-")
	if idx < 0 {
		return base
	}
	if len(base) >= 36 {
		tail := base[len(base)-36:]
		if strings.Count(tail, "-") == 4 {
			return tail
		}
	}
	return base[idx+1:]
}

func lastMessages(messages []MessageBrief, n int) []MessageBrief {
	if len(messages) <= n {
		return messages
	}
	return messages[len(messages)-n:]
}

func normalizeTime(value string) string {
	if value == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return value
}
