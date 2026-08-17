package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Source classifies where a skill was found.
type Source string

const (
	SourceUser    Source = "user"
	SourceBuiltin Source = "builtin"
	SourcePlugin  Source = "plugin"
	SourceProject Source = "project"
)

// Skill is a parsed SKILL.md plus inventory metadata.
type Skill struct {
	Name                    string   `json:"name"`
	Description             string   `json:"description"`
	Path                    string   `json:"path"`
	Dir                     string   `json:"dir"`
	Source                  Source   `json:"source"`
	DisableModelInvocation  bool     `json:"disable_model_invocation"`
	Paths                   []string `json:"paths,omitempty"`
	DescriptionChars        int      `json:"description_chars"`
	DescriptionTokensApprox int      `json:"description_tokens_approx"`
	BodyLines               int      `json:"body_lines"`
	ContentHash             string   `json:"content_hash"`
	Flags                   []string `json:"flags,omitempty"`
}

type frontmatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
	Paths                  Paths  `yaml:"paths"`
	Globs                  Paths  `yaml:"globs"`
}

// Paths unmarshals a YAML string, comma-separated string, or sequence.
type Paths []string

func (p *Paths) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		raw := strings.TrimSpace(value.Value)
		if raw == "" {
			*p = nil
			return nil
		}
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		*p = out
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		*p = items
	default:
		return fmt.Errorf("paths: unexpected yaml node kind %v", value.Kind)
	}
	return nil
}

// ParseFile reads and parses a SKILL.md file.
func ParseFile(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("reading skill %s: %w", path, err)
	}
	return Parse(path, data)
}

// Parse parses SKILL.md bytes. Name falls back to the parent directory.
func Parse(path string, data []byte) (Skill, error) {
	sum := sha256.Sum256(data)
	s := Skill{
		Path:        path,
		Dir:         filepath.Dir(path),
		Name:        filepath.Base(filepath.Dir(path)),
		ContentHash: hex.EncodeToString(sum[:]),
	}

	fm, body, err := splitFrontmatter(string(data))
	if err != nil {
		return Skill{}, fmt.Errorf("parsing frontmatter in %s: %w", path, err)
	}
	if fm != "" {
		var meta frontmatter
		if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
			return Skill{}, fmt.Errorf("decoding frontmatter in %s: %w", path, err)
		}
		if meta.Name != "" {
			s.Name = meta.Name
		}
		s.Description = strings.TrimSpace(meta.Description)
		s.DisableModelInvocation = meta.DisableModelInvocation
		if len(meta.Paths) > 0 {
			s.Paths = []string(meta.Paths)
		} else if len(meta.Globs) > 0 {
			s.Paths = []string(meta.Globs)
		}
	}

	s.BodyLines = countLines(strings.TrimSpace(body))
	s.DescriptionChars = utf8.RuneCountInString(s.Description)
	s.DescriptionTokensApprox = (s.DescriptionChars + 3) / 4
	s.Flags = flagsFor(s)
	return s, nil
}

func splitFrontmatter(raw string) (frontmatterText, body string, err error) {
	trimmed := strings.TrimPrefix(raw, "\ufeff")
	if !strings.HasPrefix(trimmed, "---") {
		return "", trimmed, nil
	}
	rest := strings.TrimPrefix(trimmed, "---")
	rest = strings.TrimLeft(rest, "\r\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("unclosed yaml frontmatter")
	}
	return rest[:idx], strings.TrimLeft(rest[idx+len("\n---"):], "\r\n"), nil
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n") + 1
	if strings.HasSuffix(s, "\n") {
		n--
	}
	return n
}

func flagsFor(s Skill) []string {
	var flags []string
	if s.DisableModelInvocation {
		flags = append(flags, "slash-only")
	}
	if strings.TrimSpace(s.Description) == "" {
		flags = append(flags, "empty-description")
	} else if s.DescriptionChars < 40 {
		flags = append(flags, "vague-description")
	} else {
		lower := strings.ToLower(s.Description)
		if !strings.Contains(lower, "when") && !strings.Contains(lower, "use ") {
			flags = append(flags, "no-trigger-terms")
		}
	}
	return flags
}

// AutoInvocable reports whether the agent may load the skill from context.
func (s Skill) AutoInvocable() bool {
	return !s.DisableModelInvocation
}
