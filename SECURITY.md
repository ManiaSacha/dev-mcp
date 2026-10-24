# Security policy

## Reporting a vulnerability

Please report security problems privately. Do not open a public issue.

Use GitHub's private vulnerability reporting: open the **Security** tab of this repository and choose **Report a vulnerability**. Include the version (`dev-mcp --version`), your operating system, the tool involved, and the steps to reproduce.

This is a volunteer project. I will read every report and reply as soon as I can, but there is no guaranteed response time.

## Supported versions

Only the latest release and the `main` branch receive fixes.

## What counts as a vulnerability

`dev-mcp` runs on your machine with your user's permissions and is driven by an MCP client, which is usually an AI assistant. These are in scope:

- A way to make a tool run a shell command or an unintended program.
- A way to read or write outside the paths a tool is meant to reach, including escaping `DEV_MCP_ALLOWED_ROOTS` with `..`, symlinks or similar tricks.
- Secrets that should be masked but appear in the output of `logs_tail`, `logs_search`, `logs_redact`, `docker_logs` or `docker_health`.
- A way to write to a database or to run SQL through `db_schema` or `db_migrations`.
- A way to write to stdout that corrupts the JSON-RPC stream and could be used to inject protocol messages.

## Known limits (not vulnerabilities)

- **Redaction is best-effort.** It matches common credential shapes. A secret in an unusual format may pass through. A report of one missed pattern is welcome as a normal issue or pull request.
- **Tools act with your permissions.** Without `DEV_MCP_ALLOWED_ROOTS`, the file-reading tools can read any file your user can read. Set it to limit them. See the README.
- **`git_stage_and_commit` runs the repository's git hooks**, as `git commit` normally does. Do not point it at a repository you do not trust.
- The Docker tools talk to whichever Docker daemon the `docker` command reaches.
