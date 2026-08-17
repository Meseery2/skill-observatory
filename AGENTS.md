# Skill Observatory

Before any Go coding, review, debugging, troubleshooting, or setup task, load the `samber/cc-skills-golang@golang-how-to` skill first — it routes to whichever other Go skills the task needs.

## Go development

This is a local CLI. Keep `cmd/skill-observatory` thin: parse flags, open the store, call `internal/` packages. Logs go to stderr; pipeable results go to stdout.
