package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/meseery/skill-observatory/internal/store"
)

const maxPromptChars = 2000

var (
	userQueryRe   = regexp.MustCompile(`(?s)<user_query>\s*(.*?)\s*</user_query>`)
	skillNameRe   = regexp.MustCompile(`(?m)^Skill Name:\s*(.+)$`)
	skillPathRe   = regexp.MustCompile(`(?m)^Path:\s*(.+\.md)\s*$`)
	attachedBlock = regexp.MustCompile(`(?s)<manually_attached_skills>.*?</manually_attached_skills>`)
)

type line struct {
	Role    string  `json:"role"`
	Message message `json:"message"`
}

type message struct {
	Content json.RawMessage `json:"content"`
}

type contentPart struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ScanRoot walks ~/.cursor/projects/*/agent-transcripts/**/*.jsonl.
func ScanRoot(root string) ([]store.Invocation, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat transcripts root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("transcripts root is not a directory: %s", root)
	}

	var events []store.Invocation
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		if !strings.Contains(filepath.ToSlash(path), "/agent-transcripts/") {
			return nil
		}
		found, err := ParseFile(path)
		if err != nil {
			return err
		}
		events = append(events, found...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking transcripts: %w", err)
	}
	return events, nil
}

func ParseFile(path string) (events []store.Invocation, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening transcript %s: %w", path, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat transcript %s: %w", path, err)
	}
	invokedAt := info.ModTime().UTC().Format(time.RFC3339)
	project, conversationID := locate(path)
	events, err = parse(f, path, project, conversationID, invokedAt)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return events, nil
}

func locate(path string) (project, conversationID string) {
	slash := filepath.ToSlash(path)
	const marker = "/agent-transcripts/"
	idx := strings.Index(slash, marker)
	if idx >= 0 {
		before := slash[:idx]
		project = filepath.Base(before)
		rest := slash[idx+len(marker):]
		conversationID = strings.Split(rest, "/")[0]
	}
	if conversationID == "" {
		conversationID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if project == "" {
		project = "unknown"
	}
	return project, conversationID
}

func parse(r io.Reader, transcriptPath, project, conversationID, invokedAt string) ([]store.Invocation, error) {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 1024*1024)
	sc.Buffer(buf, 32*1024*1024)

	var events []store.Invocation
	turn := -1
	var prompt string
	var truncated bool
	seen := map[string]struct{}{}

	flushKey := func(name, path, kind string) string {
		return fmt.Sprintf("%d|%s|%s|%s", turn, name, kind, path)
	}

	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var ln line
		if err := json.Unmarshal([]byte(raw), &ln); err != nil {
			continue
		}
		text, parts := decodeContent(ln.Message.Content)
		switch ln.Role {
		case "user":
			turn++
			prompt, truncated = extractPrompt(text)
			seen = map[string]struct{}{}
			for _, inv := range parseManual(text) {
				key := flushKey(inv.name, inv.path, "manual")
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				events = append(events, store.Invocation{
					ConversationID:  conversationID,
					Project:         project,
					TranscriptPath:  transcriptPath,
					TurnIndex:       turn,
					Prompt:          prompt,
					PromptTruncated: truncated,
					SkillName:       inv.name,
					SkillPath:       inv.path,
					Kind:            "manual",
					InvokedAt:       invokedAt,
				})
			}
		case "assistant":
			if turn < 0 {
				turn = 0
			}
			for _, part := range parts {
				if part.Type != "tool_use" {
					continue
				}
				toolPath := toolPath(part)
				if toolPath == "" {
					continue
				}
				name := skillNameFromPath(toolPath)
				kind := ""
				switch {
				case strings.EqualFold(filepath.Base(toolPath), "SKILL.md"):
					kind = "auto"
				case isFollowOn(toolPath):
					kind = "followon"
					name = skillNameFromPath(toolPath)
				default:
					continue
				}
				if name == "" {
					continue
				}
				key := flushKey(name, toolPath, kind)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				events = append(events, store.Invocation{
					ConversationID:  conversationID,
					Project:         project,
					TranscriptPath:  transcriptPath,
					TurnIndex:       turn,
					Prompt:          prompt,
					PromptTruncated: truncated,
					SkillName:       name,
					SkillPath:       toolPath,
					Kind:            kind,
					InvokedAt:       invokedAt,
				})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

type namedPath struct {
	name string
	path string
}

func parseManual(text string) []namedPath {
	block := attachedBlock.FindString(text)
	if block == "" {
		return nil
	}
	names := skillNameRe.FindAllStringSubmatch(block, -1)
	paths := skillPathRe.FindAllStringSubmatch(block, -1)
	n := len(names)
	if len(paths) < n {
		n = len(paths)
	}
	var out []namedPath
	for i := 0; i < n; i++ {
		name := strings.TrimSpace(names[i][1])
		path := strings.TrimSpace(paths[i][1])
		if name == "" {
			name = skillNameFromPath(path)
		}
		out = append(out, namedPath{name: name, path: path})
	}
	return out
}

func extractPrompt(text string) (string, bool) {
	if m := userQueryRe.FindStringSubmatch(text); len(m) == 2 {
		text = m[1]
	} else {
		text = attachedBlock.ReplaceAllString(text, "")
		text = stripHugeXML(text)
	}
	text = strings.TrimSpace(text)
	if utf8Len := len([]rune(text)); utf8Len > maxPromptChars {
		return string([]rune(text)[:maxPromptChars]), true
	}
	return text, false
}

func stripHugeXML(text string) string {
	for _, tag := range []string{
		"agent_transcripts", "mcp_meta_tools", "mcp_meta_tool_servers",
		"available_skills", "agent_skills", "dynamic_tool_catalog",
		"available_subagent_types", "user_info", "system_reminder",
	} {
		re := regexp.MustCompile(`(?s)<` + tag + `>.*?</` + tag + `>`)
		text = re.ReplaceAllString(text, "")
	}
	return text
}

func decodeContent(raw json.RawMessage) (string, []contentPart) {
	if len(raw) == 0 {
		return "", nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", nil
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
			b.WriteByte('\n')
		}
	}
	return b.String(), parts
}

func toolPath(part contentPart) string {
	if strings.ToLower(part.Name) != "read" {
		return ""
	}
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(part.Input, &input); err != nil {
		return ""
	}
	return input.Path
}

func skillNameFromPath(path string) string {
	dir := filepath.Dir(path)
	for dir != filepath.Dir(dir) {
		parent := filepath.Dir(dir)
		switch filepath.Base(parent) {
		case "skills", "skills-cursor":
			return filepath.Base(dir)
		}
		dir = parent
	}
	if strings.EqualFold(filepath.Base(path), "SKILL.md") {
		return filepath.Base(filepath.Dir(path))
	}
	return ""
}

func isFollowOn(path string) bool {
	slash := filepath.ToSlash(path)
	if !strings.Contains(slash, "/skills/") && !strings.Contains(slash, "/skills-cursor/") {
		return false
	}
	return strings.Contains(slash, "/references/") ||
		strings.Contains(slash, "/scripts/") ||
		strings.Contains(slash, "/assets/")
}
