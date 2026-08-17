# Skill Observatory

Local Go CLI that measures whether installed Cursor/Claude agent skills actually fire, and whether they improve output.

It answers:

1. Does the model pick the relevant skill, and does that help?
2. For one skill, does its description trigger on the right prompts, and does the body improve the answer?
3. Which skills ran, on which prompts?

## Install

```bash
go build -o bin/skill-observatory ./cmd/skill-observatory
```

Optional config: `~/.config/skill-observatory/config.yaml` (see `config.example.yaml`). Set `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` for evals.

## Commands

```bash
skill-observatory discover
skill-observatory inventory
skill-observatory log --skill resume-ats-optimizer
skill-observatory eval generate golang-error-handling
skill-observatory eval trigger golang-error-handling --catalog-mode full
skill-observatory eval quality golang-error-handling
skill-observatory report
```

`discover` and `log` need no API key. Trigger and quality evals call an LLM over HTTP.

Seed fixtures live in `evals/` for `golang-error-handling`, `resume-ats-optimizer`, and `canvas`.

## How measurement works

Cursor only injects skill **name + description**. The model then **reads `SKILL.md`** (or you attach the skill with `/`).

- **Transcript log** parses `~/.cursor/projects/*/agent-transcripts/**/*.jsonl` for manual attaches and `Read` of `SKILL.md`.
- **Trigger eval** sends the catalog of names+descriptions and asks which skills the model would read. Reports precision, recall, F1, and clashes.
- **Quality eval** runs the same prompt with and without the skill body, then a blind LLM judge.

Slash-only skills (`disable-model-invocation: true`) are skipped for auto-trigger scoring.
