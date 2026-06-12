# suluctl

**suluctl** is a single static binary that streams allure-results and JUnit XML reports into [Sulu TMS](https://aisulu.dev). No per-framework adapter is needed — the server detects and parses every file format automatically. Drop it into any CI pipeline and results appear in Sulu in seconds.

## Install

**a) One-liner (Linux / macOS):**

```sh
curl -fsSL https://raw.githubusercontent.com/ellyZz/suluctl/main/install.sh | sh
```

**b) Download a binary** from [GitHub Releases](https://github.com/ellyZz/suluctl/releases) — choose the archive for your OS and architecture, extract, and place `suluctl` on your `PATH`.

**c) Docker:**

```sh
docker run --rm -v "$PWD:/work" -w /work \
  -e SULU_URL -e SULU_TOKEN -e SULU_PROJECT_ID \
  ghcr.io/ellyzz/suluctl:latest upload --results ./allure-results
```

## Quickstart

```bash
export SULU_URL=https://sulu.example.com
export SULU_TOKEN=<api token>      # Profile → API keys
export SULU_PROJECT_ID=1

# one-shot upload after a run
suluctl upload --results ./allure-results --launch-name "nightly $(date +%F)"

# near-live streaming around the test command
suluctl watch --results ./allure-results -- mvn test
```

## Configuration

| Env var | Flag | Required | Meaning |
|---|---|---|---|
| `SULU_URL` | `--url` | yes | Base URL of your Sulu instance, e.g. `https://sulu.example.com` |
| `SULU_TOKEN` | `--token` | yes | User API token (Profile → API keys in Sulu) |
| `SULU_PROJECT_ID` | `--project` | yes | Numeric project ID to upload results into |
| `SULU_LAUNCH_NAME` | `--launch-name` | no | Launch name; server assigns a default when omitted |
| — | `--env` | no | Environment label attached to the launch |
| — | `--tag` | no | Tag name (repeatable, e.g. `--tag smoke --tag nightly`) |
| — | `--env-var K=V` | no | Custom environment variable recorded on the launch (repeatable) |
| — | `--insecure` | no | Skip TLS certificate verification — for on-prem self-signed certs only; prefer adding your CA to the trust store |
| — | `--clean` | no | `watch` only — empty the results directory before starting the test command |

## Exit codes

| Command | Code | Meaning |
|---|---|---|
| `upload` | `0` | Success — all files processed (individual `FAILED` rows in the summary are non-fatal) |
| `upload` | `1` | Total failure — config error, auth failure (401/403), quota exceeded (402), conflict (409), or exhausted retries. An oversize solo file (>50 MB) fails per-file, not at exit-code level. |
| `upload` | `2` | Usage error (bad flags / missing required config) |
| `watch` | the wrapped command's exit code | Always relays the test command's exit code: `127` if the command cannot be started, `130` if killed by a signal. If Sulu is unreachable, `watch` degrades to a transparent wrapper and still returns the test command's exit code. |

## CI examples

### GitHub Actions

```yaml
- name: Run tests and stream to Sulu
  env:
    SULU_URL: ${{ secrets.SULU_URL }}
    SULU_TOKEN: ${{ secrets.SULU_TOKEN }}
    SULU_PROJECT_ID: ${{ secrets.SULU_PROJECT_ID }}
  run: |
    curl -fsSL https://raw.githubusercontent.com/ellyZz/suluctl/main/install.sh | sh
    suluctl watch --results ./allure-results -- mvn test
```

### GitLab CI

```yaml
test:
  script:
    - curl -fsSL https://raw.githubusercontent.com/ellyZz/suluctl/main/install.sh | sh
    - suluctl watch --results ./allure-results -- mvn test
  variables:
    SULU_URL: $SULU_URL
    SULU_TOKEN: $SULU_TOKEN
    SULU_PROJECT_ID: $SULU_PROJECT_ID
```

## How it works

- **`upload`** creates an import session on the Sulu server, then uploads files in batches (up to 100 files / 190 MB per request; files larger than 50 MB ride alone). After all batches are sent, it calls finish and prints a ledger summary with a direct link to the new launch.
- **`watch`** polls the results directory every 2 seconds and uploads files once their size and modification time are stable across two consecutive scans. Changed files are re-uploaded — the server deduplicates identical files by checksum and collapses rewritten results by test identity (historyId). If Sulu is unreachable, `watch` runs the test command transparently and exits with its exit code.
- **The server handles format detection** — allure-results JSON, allure container JSON, JUnit XML, and ZIP archives are all parsed server-side. Unknown file types are silently ignored and never cause an error.

## License

Apache-2.0 — see [LICENSE](LICENSE).
