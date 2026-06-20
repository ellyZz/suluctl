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

### Console logs (launch-scoped)

`suluctl watch` ships the wrapped command's stdout/stderr to Sulu as
**launch-scoped logs** — they appear in the launch's Logs panel. This is
**on by default** and works for any language/framework, since suluctl just
tees the console of whatever you run.

- Disable with `--ship-console=false` or `SULU_SHIP_CONSOLE=false`.
- stdout lines are recorded at `INFO`, stderr at `ERROR`.
- Capture is best-effort: if Sulu is unreachable, your test command's exit
  code is never affected.
- Capped at ~50 MB / 200k lines per run (beyond that, logs are truncated with
  a warning).
- **JUnit XML uploaders:** if the uploaded XML contains `<system-out>`/`<system-err>`,
  the server ships those suite-level lines a second time (as `junit-import-suite` source)
  — console output can appear twice in the Logs panel for JUnit XML runs.

> ⚠️ **Security:** console output is sent to Sulu as-is. Do not print secrets,
> tokens, or PII in tests — or set `SULU_SHIP_CONSOLE=false`.

> ℹ️ This is **launch-scoped** (the whole run's console). For **per-test** logs
> in each result's Logs panel, use your framework's Allure log integration
> (see `suluctl init`).

### `suluctl init`

Scaffold the Sulu allure-glue into an existing test project, then print the exact
`suluctl watch` command to run.

```
suluctl init [--framework testng|junit5|playwright|pytest|xunit] [--package P] [--dry-run] [--force]
```

- Autodetects the framework from your build files; `--framework` overrides.
- Drops the glue (idempotent — re-run is safe; `--force` overwrites managed files).
- Auto-patches `package.json` / `*.csproj`; prints the snippet for Gradle / Maven / `pyproject.toml` / `playwright.config.ts`.
- `--package` (Java only) sets the glue package; defaults to your tests' base package, else `sulu`.

Caveats: **Playwright** specs must import `test` from `./support/sulu`; **xUnit** has no
auto-binding — apply `[SuluTest("<id>")]` to tests or no results are produced.

### `suluctl sync-ids`

After a first real run, auto-created test cases get an ugly id equal to the test's structural
fullName (e.g. `petstore.PetTests.create`). `sync-ids` pulls the pretty, stable per-project id
(`<KEY>-<N>`, e.g. `PET-37`) from Sulu and writes it back into your source as the framework's
`sulu_id` token, so it round-trips cleanly and survives renames.

```
suluctl sync-ids [--framework testng|junit5|pytest|playwright|xunit] [--package P] [--dir D] [--dry-run] [--force]
```

The flow composes with `init` + `watch`:

```
suluctl init                                            # wire the glue (once)
suluctl watch --results <dir> -- <test cmd>             # run → cases auto-created
suluctl sync-ids                                        # write @SuluTest(id="PET-37") into source
suluctl watch --results <dir> -- <test cmd>             # subsequent runs link to the same cases
```

- Needs `SULU_URL` / `SULU_TOKEN` / `SULU_PROJECT_ID` (a `MEMBER+` API token). Read-only on the
  server (it only resolves ids); it edits your local source. `--dry-run` previews, `--force`
  overwrites an existing id, and re-running is a no-op (idempotent).
- A test that doesn't resolve (404 — never run, or its method/class was renamed so its fullName
  changed) is reported as **not found** and left unannotated; it is never guessed. No fuzzy matcher
  in v1.

| Framework | token written | resolve key (fullName) |
|---|---|---|
| testng / junit5 | `@SuluTest(id="PET-37")` (+ import) | `<FQCN>.<method>` |
| pytest | `@sulu_test(id="PET-37")` (+ import) | `tests.<module>#<func>` |
| playwright | `{ annotation: { type: 'sulu', description: 'PET-37' } }` in the `test(...)` 2nd arg | `<fileRelToTestDir>:<line>:<col>` |
| xunit | fills an empty `[Fact, SuluTest("")]` → `[Fact, SuluTest("PET-37")]` | `<Namespace>.<Class>.<Method>` |

Per-framework caveats:

- **playwright** — the resolve key is `file:line:col`, so it shifts if a test's line moves. Run
  `sync-ids` right after `watch`, before further edits; the written `sulu_id` is stable thereafter.
- **xunit** — a `[Fact]` without `[SuluTest]` emits no result at all, so there's nothing to resolve.
  Write `[Fact, SuluTest("")]` (empty) to opt a test into auto-creation; `sync-ids` then fills the
  empty id. Non-empty (your own) ids are never touched.

Requires the Sulu backend resolve endpoint (`GET /api/projects/{id}/test-cases/resolve`), shipped in
the `sulu` repo Layer-2 backend PR.

## License

Apache-2.0 — see [LICENSE](LICENSE).
