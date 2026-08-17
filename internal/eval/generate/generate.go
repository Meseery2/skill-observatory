package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/meseery/skill-observatory/internal/eval/quality"
	"github.com/meseery/skill-observatory/internal/eval/trigger"
	"github.com/meseery/skill-observatory/internal/llm"
	"github.com/meseery/skill-observatory/internal/skill"
)

type Result struct {
	TriggersPath string         `json:"triggers_path"`
	EvalsPath    string         `json:"evals_path"`
	Triggers     []trigger.Case `json:"triggers"`
	Evals        []quality.Case `json:"evals"`
}

func Draft(ctx context.Context, client llm.Client, sk skill.Skill, evalsDir string) (Result, error) {
	if client == nil {
		return Result{}, fmt.Errorf("llm client is required")
	}
	body, err := os.ReadFile(sk.Path)
	if err != nil {
		return Result{}, fmt.Errorf("reading skill: %w", err)
	}

	user := fmt.Sprintf(`Skill name: %s
Description: %s

SKILL.md:
%s

Create evaluation fixtures for this skill.

Return JSON:
{
  "triggers": [
    {"id":"pos-1","query":"...","should_trigger":true,"notes":"..."},
    {"id":"neg-1","query":"...","should_trigger":false,"notes":"near-miss"}
  ],
  "evals": [
    {"id":"q1","prompt":"...","expected_output":"...","assertions":["..."]}
  ]
}

Rules:
- 10 should_trigger=true queries with varied phrasing (not just the skill name).
- 10 should_trigger=false near-misses that share keywords but need a different skill.
- 3 quality evals that a single-turn completion can attempt (no multi-step agent tools).
- Realistic developer or job-seeker phrasing.
`, sk.Name, sk.Description, truncate(string(body), 8000))

	resp, err := client.Complete(ctx, llm.Request{
		System:      "You write evaluation datasets for agent skills. Return JSON only.",
		User:        user,
		JSON:        true,
		Temperature: 0.4,
		MaxTokens:   4096,
	})
	if err != nil {
		return Result{}, fmt.Errorf("generating evals: %w", err)
	}

	var parsed struct {
		Triggers []trigger.Case `json:"triggers"`
		Evals    []quality.Case `json:"evals"`
	}
	if err := json.Unmarshal([]byte(llm.ExtractJSON(resp.Text)), &parsed); err != nil {
		return Result{}, fmt.Errorf("decoding generated evals: %w", err)
	}
	if len(parsed.Triggers) == 0 {
		return Result{}, fmt.Errorf("generator returned no trigger cases")
	}

	dir := filepath.Join(evalsDir, sk.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, fmt.Errorf("creating evals dir: %w", err)
	}
	trigPath := filepath.Join(dir, "triggers.json")
	evalPath := filepath.Join(dir, "evals.json")
	if err := writeJSON(trigPath, parsed.Triggers); err != nil {
		return Result{}, err
	}
	file := quality.File{SkillName: sk.Name, Evals: parsed.Evals}
	if err := writeJSON(evalPath, file); err != nil {
		return Result{}, err
	}
	return Result{
		TriggersPath: trigPath,
		EvalsPath:    evalPath,
		Triggers:     parsed.Triggers,
		Evals:        parsed.Evals,
	}, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]..."
}
