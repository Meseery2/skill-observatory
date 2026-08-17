package discover

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/meseery/skill-observatory/internal/fsutil"
	"github.com/meseery/skill-observatory/internal/skill"
)

// Options controls which roots are walked.
type Options struct {
	Home     string
	Projects []string
}

// Result is a discovered catalog with duplicate groups.
type Result struct {
	Skills     []skill.Skill
	Duplicates []Duplicate
}

// Duplicate is the same skill name with differing content hashes.
type Duplicate struct {
	Name  string
	Paths []string
}

// DefaultUserRoots returns Cursor-compatible skill directories under home.
func DefaultUserRoots(home string) []string {
	return []string{
		filepath.Join(home, ".cursor", "skills"),
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".cursor", "skills-cursor"),
		filepath.Join(home, ".cursor", "plugins"),
	}
}

func projectSkillRoots(project string) []string {
	return []string{
		filepath.Join(project, ".cursor", "skills"),
		filepath.Join(project, ".agents", "skills"),
		filepath.Join(project, ".claude", "skills"),
		filepath.Join(project, ".codex", "skills"),
	}
}

// Scan walks configured roots and returns parsed skills.
func Scan(opts Options) (Result, error) {
	home := opts.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Result{}, fmt.Errorf("resolving home: %w", err)
		}
	}

	type root struct {
		path   string
		source skill.Source
	}
	var roots []root
	for _, p := range DefaultUserRoots(home) {
		roots = append(roots, root{path: p, source: classifyUserRoot(home, p)})
	}
	for _, project := range opts.Projects {
		project = fsutil.ExpandHome(project)
		for _, p := range projectSkillRoots(project) {
			roots = append(roots, root{path: p, source: skill.SourceProject})
		}
	}

	seenPath := make(map[string]struct{})
	var skills []skill.Skill
	for _, r := range roots {
		found, err := walkRoot(r.path, r.source)
		if err != nil {
			return Result{}, err
		}
		for _, s := range found {
			if _, ok := seenPath[s.Path]; ok {
				continue
			}
			seenPath[s.Path] = struct{}{}
			skills = append(skills, s)
		}
	}

	sort.Slice(skills, func(i, j int) bool {
		if skills[i].Name == skills[j].Name {
			return skills[i].Path < skills[j].Path
		}
		return skills[i].Name < skills[j].Name
	})

	return Result{Skills: skills, Duplicates: findDuplicates(skills)}, nil
}

func classifyUserRoot(home, path string) skill.Source {
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return skill.SourceUser
	}
	switch {
	case strings.Contains(filepath.ToSlash(rel), ".cursor/skills-cursor"):
		return skill.SourceBuiltin
	case strings.Contains(filepath.ToSlash(rel), ".cursor/plugins"):
		return skill.SourcePlugin
	default:
		return skill.SourceUser
	}
}

func walkRoot(root string, source skill.Source) ([]skill.Skill, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, nil
	}

	var skills []skill.Skill
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == ".git" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		s, err := skill.ParseFile(path)
		if err != nil {
			return fmt.Errorf("discover %s: %w", path, err)
		}
		s.Source = source
		if source == skill.SourcePlugin || strings.Contains(filepath.ToSlash(path), "/plugins/") {
			s.Source = skill.SourcePlugin
		}
		skills = append(skills, s)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", root, err)
	}
	return skills, nil
}

func findDuplicates(skills []skill.Skill) []Duplicate {
	type group struct {
		hashes map[string]struct{}
		paths  []string
	}
	byName := make(map[string]*group)
	for _, s := range skills {
		g, ok := byName[s.Name]
		if !ok {
			g = &group{hashes: map[string]struct{}{}}
			byName[s.Name] = g
		}
		g.hashes[s.ContentHash] = struct{}{}
		g.paths = append(g.paths, s.Path)
	}
	var dups []Duplicate
	for name, g := range byName {
		if len(g.hashes) > 1 {
			dups = append(dups, Duplicate{Name: name, Paths: g.paths})
		}
	}
	sort.Slice(dups, func(i, j int) bool { return dups[i].Name < dups[j].Name })
	return dups
}

// DedupeByHash keeps one copy per name+content hash, preferring user over plugin/builtin.
func DedupeByHash(skills []skill.Skill) []skill.Skill {
	type key struct{ name, hash string }
	best := make(map[key]skill.Skill)
	rank := func(src skill.Source) int {
		switch src {
		case skill.SourceUser:
			return 0
		case skill.SourceProject:
			return 1
		case skill.SourceBuiltin:
			return 2
		default:
			return 3
		}
	}
	for _, s := range skills {
		k := key{s.Name, s.ContentHash}
		cur, ok := best[k]
		if !ok || rank(s.Source) < rank(cur.Source) {
			best[k] = s
		}
	}
	out := make([]skill.Skill, 0, len(best))
	for _, s := range best {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Path < out[j].Path
		}
		return out[i].Name < out[j].Name
	})
	return out
}
