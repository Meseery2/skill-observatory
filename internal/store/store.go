package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meseery/skill-observatory/internal/skill"
	_ "modernc.org/sqlite"
)

// Store is a local SQLite inventory of skills, invocations, and eval runs.
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pragma: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	if err := s.migrateInvocationsUnique(); err != nil {
		return err
	}
	const schema = `
CREATE TABLE IF NOT EXISTS skills (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  path TEXT NOT NULL UNIQUE,
  source TEXT NOT NULL,
  disable_model_invocation INTEGER NOT NULL DEFAULT 0,
  description_chars INTEGER NOT NULL,
  body_lines INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  flags TEXT NOT NULL DEFAULT '[]',
  discovered_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS invocations (
  id INTEGER PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  project TEXT NOT NULL,
  transcript_path TEXT NOT NULL,
  turn_index INTEGER NOT NULL,
  prompt TEXT NOT NULL,
  prompt_truncated INTEGER NOT NULL,
  skill_name TEXT NOT NULL,
  skill_path TEXT NOT NULL,
  kind TEXT NOT NULL,
  invoked_at TEXT NOT NULL,
  UNIQUE(conversation_id, turn_index, skill_name, kind, skill_path, transcript_path)
);

CREATE INDEX IF NOT EXISTS invocations_skill ON invocations(skill_name);
CREATE INDEX IF NOT EXISTS invocations_project ON invocations(project);

CREATE TABLE IF NOT EXISTS eval_runs (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL,
  skill_name TEXT NOT NULL,
  catalog_mode TEXT,
  model TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NOT NULL,
  summary_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS eval_results (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
  case_id TEXT NOT NULL,
  repetition INTEGER NOT NULL,
  payload_json TEXT NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrating schema: %w", err)
	}
	return nil
}

func (s *Store) migrateInvocationsUnique() error {
	var ddl string
	err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='invocations'`).Scan(&ddl)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspecting invocations table: %w", err)
	}
	if strings.Contains(ddl, "transcript_path)") {
		return nil
	}
	if _, err := s.db.Exec(`DROP TABLE IF EXISTS invocations`); err != nil {
		return fmt.Errorf("dropping legacy invocations table: %w", err)
	}
	return nil
}

func (s *Store) ReplaceSkills(ctx context.Context, skills []skill.Skill) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM skills`); err != nil {
		return fmt.Errorf("clearing skills: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO skills (
  name, description, path, source, disable_model_invocation,
  description_chars, body_lines, content_hash, flags, discovered_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert skill: %w", err)
	}
	defer stmt.Close()

	for _, sk := range skills {
		flags, err := json.Marshal(sk.Flags)
		if err != nil {
			return fmt.Errorf("encoding flags for %s: %w", sk.Name, err)
		}
		disable := 0
		if sk.DisableModelInvocation {
			disable = 1
		}
		if _, err := stmt.ExecContext(
			ctx,
			sk.Name,
			sk.Description,
			sk.Path,
			string(sk.Source),
			disable,
			sk.DescriptionChars,
			sk.BodyLines,
			sk.ContentHash,
			string(flags),
			now,
		); err != nil {
			return fmt.Errorf("inserting skill %s: %w", sk.Path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit skills: %w", err)
	}
	return nil
}

func (s *Store) ListSkills(ctx context.Context) ([]skill.Skill, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT name, description, path, source, disable_model_invocation,
       description_chars, body_lines, content_hash, flags
FROM skills ORDER BY name, path`)
	if err != nil {
		return nil, fmt.Errorf("listing skills: %w", err)
	}
	defer rows.Close()

	var out []skill.Skill
	for rows.Next() {
		var sk skill.Skill
		var source string
		var disable int
		var flags string
		if err := rows.Scan(
			&sk.Name,
			&sk.Description,
			&sk.Path,
			&source,
			&disable,
			&sk.DescriptionChars,
			&sk.BodyLines,
			&sk.ContentHash,
			&flags,
		); err != nil {
			return nil, fmt.Errorf("scanning skill: %w", err)
		}
		sk.Source = skill.Source(source)
		sk.Dir = filepath.Dir(sk.Path)
		sk.DisableModelInvocation = disable == 1
		sk.DescriptionTokensApprox = (sk.DescriptionChars + 3) / 4
		if flags != "" {
			_ = json.Unmarshal([]byte(flags), &sk.Flags)
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// Invocation is one skill activation on one user turn.
type Invocation struct {
	ConversationID  string `json:"conversation_id"`
	Project         string `json:"project"`
	TranscriptPath  string `json:"transcript_path"`
	TurnIndex       int    `json:"turn_index"`
	Prompt          string `json:"prompt"`
	PromptTruncated bool   `json:"prompt_truncated"`
	SkillName       string `json:"skill_name"`
	SkillPath       string `json:"skill_path"`
	Kind            string `json:"kind"`
	InvokedAt       string `json:"invoked_at"`
}

func (s *Store) ReplaceInvocations(ctx context.Context, events []Invocation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM invocations`); err != nil {
		return fmt.Errorf("clearing invocations: %w", err)
	}
	events = dedupeInvocations(events)
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO invocations (
  conversation_id, project, transcript_path, turn_index, prompt, prompt_truncated,
  skill_name, skill_path, kind, invoked_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert invocation: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		trunc := 0
		if e.PromptTruncated {
			trunc = 1
		}
		if _, err := stmt.ExecContext(
			ctx,
			e.ConversationID,
			e.Project,
			e.TranscriptPath,
			e.TurnIndex,
			e.Prompt,
			trunc,
			e.SkillName,
			e.SkillPath,
			e.Kind,
			e.InvokedAt,
		); err != nil {
			return fmt.Errorf("inserting invocation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit invocations: %w", err)
	}
	return nil
}

func dedupeInvocations(events []Invocation) []Invocation {
	type key struct {
		conv, tpath, name, kind, spath string
		turn                           int
	}
	seen := make(map[key]struct{}, len(events))
	out := make([]Invocation, 0, len(events))
	for _, e := range events {
		k := key{e.ConversationID, e.TranscriptPath, e.SkillName, e.Kind, e.SkillPath, e.TurnIndex}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, e)
	}
	return out
}

type InvocationFilter struct {
	Skill   string
	Project string
	Since   time.Time
}

func (s *Store) ListInvocations(ctx context.Context, f InvocationFilter) ([]Invocation, error) {
	q := `
SELECT conversation_id, project, transcript_path, turn_index, prompt, prompt_truncated,
       skill_name, skill_path, kind, invoked_at
FROM invocations WHERE 1=1`
	var args []any
	if f.Skill != "" {
		q += ` AND skill_name = ?`
		args = append(args, f.Skill)
	}
	if f.Project != "" {
		q += ` AND project = ?`
		args = append(args, f.Project)
	}
	if !f.Since.IsZero() {
		q += ` AND invoked_at >= ?`
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	q += ` ORDER BY invoked_at DESC, conversation_id, turn_index`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing invocations: %w", err)
	}
	defer rows.Close()

	var out []Invocation
	for rows.Next() {
		var e Invocation
		var trunc int
		if err := rows.Scan(
			&e.ConversationID,
			&e.Project,
			&e.TranscriptPath,
			&e.TurnIndex,
			&e.Prompt,
			&trunc,
			&e.SkillName,
			&e.SkillPath,
			&e.Kind,
			&e.InvokedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning invocation: %w", err)
		}
		e.PromptTruncated = trunc == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

type EvalRun struct {
	ID          int64
	Kind        string
	SkillName   string
	CatalogMode string
	Model       string
	StartedAt   string
	FinishedAt  string
	SummaryJSON string
}

func (s *Store) InsertEvalRun(ctx context.Context, run EvalRun, results []EvalResult) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
INSERT INTO eval_runs (kind, skill_name, catalog_mode, model, started_at, finished_at, summary_json)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.Kind, run.SkillName, run.CatalogMode, run.Model, run.StartedAt, run.FinishedAt, run.SummaryJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting eval run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("eval run id: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO eval_results (run_id, case_id, repetition, payload_json) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare eval result: %w", err)
	}
	defer stmt.Close()
	for _, r := range results {
		if _, err := stmt.ExecContext(ctx, id, r.CaseID, r.Repetition, r.PayloadJSON); err != nil {
			return 0, fmt.Errorf("inserting eval result: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit eval run: %w", err)
	}
	return id, nil
}

type EvalResult struct {
	CaseID      string
	Repetition  int
	PayloadJSON string
}

func (s *Store) LatestEvalRuns(ctx context.Context) ([]EvalRun, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, kind, skill_name, catalog_mode, model, started_at, finished_at, summary_json
FROM eval_runs e
WHERE id = (
  SELECT id FROM eval_runs
  WHERE kind = e.kind AND skill_name = e.skill_name
    AND IFNULL(catalog_mode,'') = IFNULL(e.catalog_mode,'')
  ORDER BY finished_at DESC, id DESC
  LIMIT 1
)
ORDER BY skill_name, kind`)
	if err != nil {
		return nil, fmt.Errorf("listing eval runs: %w", err)
	}
	defer rows.Close()

	var out []EvalRun
	for rows.Next() {
		var r EvalRun
		var mode sql.NullString
		if err := rows.Scan(
			&r.ID,
			&r.Kind,
			&r.SkillName,
			&mode,
			&r.Model,
			&r.StartedAt,
			&r.FinishedAt,
			&r.SummaryJSON,
		); err != nil {
			return nil, fmt.Errorf("scanning eval run: %w", err)
		}
		r.CatalogMode = mode.String
		out = append(out, r)
	}
	return out, rows.Err()
}
