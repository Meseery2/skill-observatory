package report

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/meseery/skill-observatory/internal/skill"
	"github.com/meseery/skill-observatory/internal/store"
)

//go:embed template.html
var htmlTemplate string

type SkillRow struct {
	Name           string   `json:"name"`
	Source         string   `json:"source"`
	SlashOnly      bool     `json:"slash_only"`
	Flags          []string `json:"flags"`
	Invocations    int      `json:"invocations"`
	LastSeen       string   `json:"last_seen,omitempty"`
	SamplePrompt   string   `json:"sample_prompt,omitempty"`
	TriggerF1      *float64 `json:"trigger_f1,omitempty"`
	TriggerMode    string   `json:"trigger_mode,omitempty"`
	QualityWinRate *float64 `json:"quality_win_rate,omitempty"`
	Copies         int      `json:"copies"`
}

type Report struct {
	GeneratedAt string     `json:"generated_at"`
	SkillCount  int        `json:"skill_count"`
	Dead        []SkillRow `json:"dead"`
	Hot         []SkillRow `json:"hot"`
	Skills      []SkillRow `json:"skills"`
}

func Build(skills []skill.Skill, inv []store.Invocation, runs []store.EvalRun) Report {
	type agg struct {
		row     SkillRow
		last    time.Time
		prompts []string
	}
	byName := map[string]*agg{}
	for _, s := range skills {
		a, ok := byName[s.Name]
		if !ok {
			a = &agg{row: SkillRow{
				Name:      s.Name,
				Source:    string(s.Source),
				SlashOnly: s.DisableModelInvocation,
				Flags:     s.Flags,
			}}
			byName[s.Name] = a
		}
		a.row.Copies++
		if s.DisableModelInvocation {
			a.row.SlashOnly = true
		}
	}
	for _, e := range inv {
		a, ok := byName[e.SkillName]
		if !ok {
			a = &agg{row: SkillRow{Name: e.SkillName, Source: "unknown"}}
			byName[e.SkillName] = a
		}
		if e.Kind != "followon" {
			a.row.Invocations++
		}
		if ts, err := time.Parse(time.RFC3339, e.InvokedAt); err == nil && ts.After(a.last) {
			a.last = ts
			a.row.LastSeen = e.InvokedAt
		}
		if e.Prompt != "" && len(a.prompts) < 1 {
			a.prompts = append(a.prompts, e.Prompt)
			a.row.SamplePrompt = clip(e.Prompt, 160)
		}
	}
	for _, run := range runs {
		a, ok := byName[run.SkillName]
		if !ok {
			continue
		}
		switch run.Kind {
		case "trigger":
			var summary struct {
				F1 float64 `json:"f1"`
			}
			if err := json.Unmarshal([]byte(run.SummaryJSON), &summary); err == nil {
				f1 := summary.F1
				a.row.TriggerF1 = &f1
				a.row.TriggerMode = run.CatalogMode
			}
		case "quality":
			var summary struct {
				WinRate float64 `json:"win_rate"`
			}
			if err := json.Unmarshal([]byte(run.SummaryJSON), &summary); err == nil {
				wr := summary.WinRate
				a.row.QualityWinRate = &wr
			}
		}
	}

	rep := Report{GeneratedAt: time.Now().UTC().Format(time.RFC3339), SkillCount: len(skills)}
	for _, a := range byName {
		rep.Skills = append(rep.Skills, a.row)
		if a.row.Invocations == 0 && a.row.Copies > 0 {
			rep.Dead = append(rep.Dead, a.row)
		}
	}
	sort.Slice(rep.Skills, func(i, j int) bool { return rep.Skills[i].Name < rep.Skills[j].Name })
	sort.Slice(rep.Dead, func(i, j int) bool { return rep.Dead[i].Name < rep.Dead[j].Name })
	hot := append([]SkillRow(nil), rep.Skills...)
	sort.Slice(hot, func(i, j int) bool { return hot[i].Invocations > hot[j].Invocations })
	for _, row := range hot {
		if row.Invocations == 0 {
			continue
		}
		rep.Hot = append(rep.Hot, row)
		if len(rep.Hot) == 15 {
			break
		}
	}
	return rep
}

func WriteHTML(rep Report, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating reports dir: %w", err)
	}
	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"pct": func(p *float64) string {
			if p == nil {
				return "—"
			}
			return fmt.Sprintf("%.0f%%", *p*100)
		},
		"join": strings.Join,
	}).Parse(htmlTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing html template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, rep); err != nil {
		return "", fmt.Errorf("rendering html: %w", err)
	}
	name := time.Now().UTC().Format("20060102-150405") + ".html"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("writing html: %w", err)
	}
	return path, nil
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
