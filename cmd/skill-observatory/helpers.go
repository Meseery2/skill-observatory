package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/meseery/skill-observatory/internal/clierr"
	"github.com/meseery/skill-observatory/internal/discover"
	"github.com/meseery/skill-observatory/internal/fsutil"
	"github.com/meseery/skill-observatory/internal/llm"
	"github.com/meseery/skill-observatory/internal/skill"
	"github.com/meseery/skill-observatory/internal/store"
	"github.com/spf13/viper"
)

func openStore() (*store.Store, error) {
	path := fsutil.ExpandHome(viper.GetString("db"))
	s, err := store.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	return s, nil
}

func closeErr(c io.Closer, errp *error) {
	*errp = errors.Join(*errp, c.Close())
}

func scanSkills() (discover.Result, error) {
	return discover.Scan(discover.Options{
		Projects: viper.GetStringSlice("project"),
	})
}

func loadInventory(ctx context.Context, st *store.Store) ([]skill.Skill, error) {
	skills, err := st.ListSkills(ctx)
	if err != nil {
		return nil, err
	}
	if len(skills) > 0 {
		return skills, nil
	}
	res, err := scanSkills()
	if err != nil {
		return nil, err
	}
	if err := st.ReplaceSkills(ctx, res.Skills); err != nil {
		return nil, err
	}
	return res.Skills, nil
}

func findSkill(skills []skill.Skill, name string) (skill.Skill, error) {
	var matches []skill.Skill
	for _, s := range skills {
		if s.Name == name {
			matches = append(matches, s)
		}
	}
	if len(matches) == 0 {
		return skill.Skill{}, clierr.Usage(fmt.Errorf("skill %q not found in inventory; run discover", name))
	}
	best := matches[0]
	rank := map[skill.Source]int{
		skill.SourceUser:    0,
		skill.SourceProject: 1,
		skill.SourceBuiltin: 2,
		skill.SourcePlugin:  3,
	}
	for _, s := range matches[1:] {
		if rank[s.Source] < rank[best.Source] {
			best = s
		}
	}
	return best, nil
}

func newLLM() (llm.Client, error) {
	provider := viper.GetString("provider")
	if provider == "" {
		provider = viper.GetString("llm.provider")
	}
	model := viper.GetString("model")
	if model == "" {
		model = viper.GetString("llm.model")
	}
	if model == "" {
		model = "gpt-4.1"
	}
	return llm.New(llm.Config{
		Provider:  provider,
		BaseURL:   viper.GetString("llm.base_url"),
		Model:     model,
		APIKeyEnv: viper.GetString("llm.api_key_env"),
	})
}

func evalsDir() string {
	return fsutil.ExpandHome(viper.GetString("evals_dir"))
}

func listFixtureSkills(filename string) ([]string, error) {
	dir := evalsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading evals dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), filename)); err == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func uniqueNames(skills []skill.Skill) map[string]int {
	counts := map[string]int{}
	hashes := map[string]map[string]struct{}{}
	for _, s := range skills {
		if hashes[s.Name] == nil {
			hashes[s.Name] = map[string]struct{}{}
		}
		hashes[s.Name][s.ContentHash] = struct{}{}
	}
	for name, set := range hashes {
		counts[name] = len(set)
	}
	return counts
}

func joinFlags(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	return strings.Join(flags, ",")
}
