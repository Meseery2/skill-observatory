package quality

import (
	"context"
	"crypto/rand"
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
	ID             string   `json:"id"`
	Prompt         string   `json:"prompt"`
	ExpectedOutput string   `json:"expected_output"`
	Assertions     []string `json:"assertions,omitempty"`
	Files          []string `json:"files,omitempty"`
	ReferenceFiles []string `json:"reference_files,omitempty"`
}

type File struct {
	SkillName string `json:"skill_name"`
	Evals     []Case `json:"evals"`
}

type Options struct {
	Target skill.Skill
	Cases  []Case
	Client llm.Client
}

type Pair struct {
	CaseID           string            `json:"case_id"`
	WithText         string            `json:"with_text"`
	WithoutText      string            `json:"without_text"`
	Winner           string            `json:"winner"` // with, without, tie
	Reason           string            `json:"reason"`
	AssertionResults []AssertionResult `json:"assertion_results,omitempty"`
	WithTokens       int               `json:"with_tokens"`
	WithoutTokens    int               `json:"without_tokens"`
	WithLatencyMS    int64             `json:"with_latency_ms"`
	WithoutLatencyMS int64             `json:"without_latency_ms"`
}

type AssertionResult struct {
	Text     string `json:"text"`
	WithPass bool   `json:"with_pass"`
	Without  bool   `json:"without_pass"`
	Evidence string `json:"evidence"`
}

type Summary struct {
	Cases          int     `json:"cases"`
	WithWins       int     `json:"with_wins"`
	WithoutWins    int     `json:"without_wins"`
	Ties           int     `json:"ties"`
	WinRate        float64 `json:"win_rate"`
	TokenDelta     int     `json:"token_delta"`
	LatencyDeltaMS int64   `json:"latency_delta_ms"`
}

type Result struct {
	Skill   string  `json:"skill"`
	Summary Summary `json:"summary"`
	Pairs   []Pair  `json:"pairs"`
}

func LoadCases(evalsDir, skillName string) ([]Case, error) {
	path := filepath.Join(evalsDir, skillName, "evals.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		var cases []Case
		if err2 := json.Unmarshal(data, &cases); err2 != nil {
			return nil, fmt.Errorf("decoding %s: %w", path, err)
		}
		return cases, nil
	}
	if len(file.Evals) > 0 {
		return file.Evals, nil
	}
	return nil, fmt.Errorf("%s: no evals", path)
}

func Run(ctx context.Context, opts Options) (Result, error) {
	if opts.Client == nil {
		return Result{}, fmt.Errorf("llm client is required")
	}
	body, err := os.ReadFile(opts.Target.Path)
	if err != nil {
		return Result{}, fmt.Errorf("reading skill body: %w", err)
	}
	refs, err := loadReferences(opts.Target.Dir, opts.Cases)
	if err != nil {
		return Result{}, err
	}

	var pairs []Pair
	for _, c := range opts.Cases {
		withSys := "Follow the skill instructions exactly.\n\n" + string(body)
		if extra := refs[c.ID]; extra != "" {
			withSys += "\n\nAdditional skill references:\n" + extra
		}
		withoutSys := "You are a helpful assistant. Answer the user's request directly."

		withResp, err := opts.Client.Complete(ctx, llm.Request{
			System:    withSys,
			User:      c.Prompt,
			MaxTokens: 2048,
		})
		if err != nil {
			return Result{}, fmt.Errorf("with-skill run %s: %w", c.ID, err)
		}
		withoutResp, err := opts.Client.Complete(ctx, llm.Request{
			System:    withoutSys,
			User:      c.Prompt,
			MaxTokens: 2048,
		})
		if err != nil {
			return Result{}, fmt.Errorf("without-skill run %s: %w", c.ID, err)
		}

		pair, err := judge(ctx, opts.Client, c, withResp, withoutResp)
		if err != nil {
			return Result{}, fmt.Errorf("judging %s: %w", c.ID, err)
		}
		pairs = append(pairs, pair)
	}
	return Result{Skill: opts.Target.Name, Summary: summarize(pairs), Pairs: pairs}, nil
}

func loadReferences(dir string, cases []Case) (map[string]string, error) {
	out := map[string]string{}
	for _, c := range cases {
		if len(c.ReferenceFiles) == 0 {
			continue
		}
		var b strings.Builder
		for _, rel := range c.ReferenceFiles {
			p := rel
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, rel)
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("reading reference %s: %w", p, err)
			}
			fmt.Fprintf(&b, "\n--- %s ---\n%s\n", rel, data)
		}
		out[c.ID] = b.String()
	}
	return out, nil
}

func judge(ctx context.Context, client llm.Client, c Case, withResp, withoutResp llm.Response) (Pair, error) {
	aIsWith := coinFlip()
	labelA, labelB := withoutResp.Text, withResp.Text
	if aIsWith {
		labelA, labelB = withResp.Text, withoutResp.Text
	}
	assertions := strings.Join(c.Assertions, "\n- ")
	if assertions != "" {
		assertions = "- " + assertions
	}
	user := fmt.Sprintf(`Task: %s
Expected: %s
Assertions:
%s

Output A:
%s

Output B:
%s

Return JSON:
{"winner":"a"|"b"|"tie","reason":"...","assertions":[{"text":"...","passed_a":true,"passed_b":false,"evidence":"..."}]}
Pick the output that better satisfies the expected outcome and assertions. If they are equivalent, use tie.`,
		c.Prompt, c.ExpectedOutput, assertions, labelA, labelB)

	resp, err := client.Complete(ctx, llm.Request{
		System:      "You are a strict blind evaluator. Do not guess which output used a skill.",
		User:        user,
		JSON:        true,
		Temperature: 0,
		MaxTokens:   1024,
	})
	if err != nil {
		return Pair{}, err
	}
	var parsed struct {
		Winner     string `json:"winner"`
		Reason     string `json:"reason"`
		Assertions []struct {
			Text     string `json:"text"`
			PassedA  bool   `json:"passed_a"`
			PassedB  bool   `json:"passed_b"`
			Evidence string `json:"evidence"`
		} `json:"assertions"`
	}
	if err := json.Unmarshal([]byte(llm.ExtractJSON(resp.Text)), &parsed); err != nil {
		return Pair{}, fmt.Errorf("decoding judge json: %w", err)
	}

	winner := "tie"
	switch strings.ToLower(parsed.Winner) {
	case "a":
		if aIsWith {
			winner = "with"
		} else {
			winner = "without"
		}
	case "b":
		if aIsWith {
			winner = "without"
		} else {
			winner = "with"
		}
	}

	var assertionsOut []AssertionResult
	for _, a := range parsed.Assertions {
		withPass, withoutPass := a.PassedB, a.PassedA
		if aIsWith {
			withPass, withoutPass = a.PassedA, a.PassedB
		}
		assertionsOut = append(assertionsOut, AssertionResult{
			Text:     a.Text,
			WithPass: withPass,
			Without:  withoutPass,
			Evidence: a.Evidence,
		})
	}

	return Pair{
		CaseID:           c.ID,
		WithText:         withResp.Text,
		WithoutText:      withoutResp.Text,
		Winner:           winner,
		Reason:           parsed.Reason,
		AssertionResults: assertionsOut,
		WithTokens:       withResp.InputTokens + withResp.OutputTokens,
		WithoutTokens:    withoutResp.InputTokens + withoutResp.OutputTokens,
		WithLatencyMS:    withResp.Latency.Milliseconds(),
		WithoutLatencyMS: withoutResp.Latency.Milliseconds(),
	}, nil
}

func summarize(pairs []Pair) Summary {
	var s Summary
	s.Cases = len(pairs)
	for _, p := range pairs {
		switch p.Winner {
		case "with":
			s.WithWins++
		case "without":
			s.WithoutWins++
		default:
			s.Ties++
		}
		s.TokenDelta += p.WithTokens - p.WithoutTokens
		s.LatencyDeltaMS += p.WithLatencyMS - p.WithoutLatencyMS
	}
	decided := s.WithWins + s.WithoutWins
	if decided > 0 {
		s.WinRate = float64(s.WithWins) / float64(decided)
	}
	return s
}

func coinFlip() bool {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UnixNano()%2 == 0
	}
	return b[0]%2 == 0
}
