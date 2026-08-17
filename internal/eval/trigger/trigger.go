package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meseery/skill-observatory/internal/llm"
	"github.com/meseery/skill-observatory/internal/skill"
)

type Case struct {
	ID            string `json:"id"`
	Query         string `json:"query"`
	ShouldTrigger bool   `json:"should_trigger"`
	Notes         string `json:"notes,omitempty"`
}

type CatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Options struct {
	Target      skill.Skill
	Catalog     []CatalogEntry
	Cases       []Case
	Repeats     int
	CatalogMode string
	Client      llm.Client
}

type Trial struct {
	CaseID        string   `json:"case_id"`
	Query         string   `json:"query"`
	ShouldTrigger bool     `json:"should_trigger"`
	Repetition    int      `json:"repetition"`
	Selected      []string `json:"selected"`
	Hit           bool     `json:"hit"`
	Clash         bool     `json:"clash"`
	InputTokens   int      `json:"input_tokens"`
	OutputTokens  int      `json:"output_tokens"`
	LatencyMS     int64    `json:"latency_ms"`
}

type Metrics struct {
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	TN        int     `json:"tn"`
	FN        int     `json:"fn"`
	Clashes   int     `json:"clashes"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	Trials    int     `json:"trials"`
}

type Result struct {
	Skill       string  `json:"skill"`
	CatalogMode string  `json:"catalog_mode"`
	Metrics     Metrics `json:"metrics"`
	Trials      []Trial `json:"trials"`
}

func LoadCases(evalsDir, skillName string) ([]Case, error) {
	path := filepath.Join(evalsDir, skillName, "triggers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cases []Case
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	return cases, nil
}

func Run(ctx context.Context, opts Options) (Result, error) {
	if opts.Repeats < 1 {
		opts.Repeats = 1
	}
	if opts.Client == nil {
		return Result{}, fmt.Errorf("llm client is required")
	}
	if opts.Target.DisableModelInvocation {
		return Result{}, fmt.Errorf("skill %s has disable-model-invocation; skip auto-trigger evals", opts.Target.Name)
	}

	var trials []Trial
	for _, c := range opts.Cases {
		for rep := 1; rep <= opts.Repeats; rep++ {
			selected, resp, err := route(ctx, opts.Client, opts.Catalog, c.Query)
			if err != nil {
				return Result{}, fmt.Errorf("routing %s rep %d: %w", c.ID, rep, err)
			}
			hit := containsFold(selected, opts.Target.Name)
			clash := c.ShouldTrigger && !hit && len(selected) > 0
			trials = append(trials, Trial{
				CaseID:        c.ID,
				Query:         c.Query,
				ShouldTrigger: c.ShouldTrigger,
				Repetition:    rep,
				Selected:      selected,
				Hit:           hit,
				Clash:         clash,
				InputTokens:   resp.InputTokens,
				OutputTokens:  resp.OutputTokens,
				LatencyMS:     resp.Latency.Milliseconds(),
			})
		}
	}
	return Result{
		Skill:       opts.Target.Name,
		CatalogMode: opts.CatalogMode,
		Metrics:     Score(opts.Target.Name, trials),
		Trials:      trials,
	}, nil
}

func Score(target string, trials []Trial) Metrics {
	var m Metrics
	m.Trials = len(trials)
	for _, t := range trials {
		hit := t.Hit
		if t.Selected != nil {
			hit = containsFold(t.Selected, target)
		}
		switch {
		case t.ShouldTrigger && hit:
			m.TP++
		case t.ShouldTrigger && !hit:
			m.FN++
			if t.Clash || len(t.Selected) > 0 {
				m.Clashes++
			}
		case !t.ShouldTrigger && hit:
			m.FP++
		default:
			m.TN++
		}
	}
	if m.TP+m.FP > 0 {
		m.Precision = float64(m.TP) / float64(m.TP+m.FP)
	}
	if m.TP+m.FN > 0 {
		m.Recall = float64(m.TP) / float64(m.TP+m.FN)
	}
	if m.Precision+m.Recall > 0 {
		m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
	}
	return m
}

func route(ctx context.Context, client llm.Client, catalog []CatalogEntry, query string) ([]string, llm.Response, error) {
	var b strings.Builder
	b.WriteString("Available skills (name + description only):\n")
	for _, e := range catalog {
		fmt.Fprintf(&b, "- %s: %s\n", e.Name, e.Description)
	}
	system := `You are an AI coding assistant. Skills are listed by name and description.
When a skill is relevant, you would read its SKILL.md before answering.
Select only skills you would actually read. Prefer precision over recall.
Return JSON: {"skills": ["skill-name", ...]} with an empty array if none apply.`

	resp, err := client.Complete(ctx, llm.Request{
		System:      system,
		User:        b.String() + "\nUser request:\n" + query + "\n",
		JSON:        true,
		Temperature: 0,
		MaxTokens:   512,
	})
	if err != nil {
		return nil, resp, err
	}
	var parsed struct {
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal([]byte(llm.ExtractJSON(resp.Text)), &parsed); err != nil {
		return nil, resp, fmt.Errorf("decoding router json: %w", err)
	}
	return parsed.Skills, resp, nil
}

func containsFold(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

func CatalogFrom(skills []skill.Skill, autoOnly bool) []CatalogEntry {
	var out []CatalogEntry
	for _, s := range skills {
		if autoOnly && !s.AutoInvocable() {
			continue
		}
		out = append(out, CatalogEntry{Name: s.Name, Description: s.Description})
	}
	return out
}

func StartedFinished() (string, string) {
	now := time.Now().UTC().Format(time.RFC3339)
	return now, now
}
