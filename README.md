<div align="center">

# tempo

A [Temporal](https://temporal.io) TUI that matches your rhythm

[![GitHub Release](https://img.shields.io/github/v/release/galaxy-io/tempo)](https://github.com/galaxy-io/tempo/releases)
[![License](https://img.shields.io/github/license/galaxy-io/tempo)](https://github.com/galaxy-io/tempo/blob/main/LICENSE)
![GitHub Downloads](https://img.shields.io/github/downloads/galaxy-io/tempo/total)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/galaxy-io/tempo)

</div>

<p align="center">
  <img src="./assets/tempo-demo.gif" alt="Tempo Demo" width="800">
</p>

## Features

**Workflow Management**

- Browse workflows across namespaces
- View workflow details, inputs, outputs, and metadata
- Inspect full event history with tree and timeline views
- Cancel, terminate, or signal running workflows
- Advanced search with visibility queries and saved filters

**Namespace Operations**

- List and browse all namespaces
- View namespace configuration and details
- Quick namespace switching

**Task Queues & Schedules**

- Monitor task queue activity
- View and manage schedules

**Connection Profiles**

- Save multiple Temporal server configurations
- TLS/mTLS support with certificate paths
- Quick profile switching with `P` key

**Customization**

- 26 built-in color themes (dark and light variants)
- Themes include: TokyoNight, Catppuccin, Dracula, Nord, Gruvbox, One Dark, Solarized, Rosé Pine, Kanagawa, Everforest, Monokai, GitHub
- Live theme preview while selecting

## Installation

### From Source

```bash
go install github.com/galaxy-io/tempo/cmd/tempo@latest
```

### Brew

Brew installed versions will not recieve auto-updates you must update with `brew update`

```bash
brew install galaxy-io/tap/tempo
```

### Build Locally

```bash
git clone https://github.com/galaxy-io/tempo.git
cd tempo
go build -o tempo ./cmd/tempo
```

## Usage

```bash
tempo --address localhost:7233 // default dev server address loads without flag
```

### Command Line Flags

| Flag                | Description                           |
| ------------------- | ------------------------------------- |
| `--address`         | Temporal server address (host:port)   |
| `--namespace`       | Default namespace                     |
| `--profile`         | Connection profile name (from config) |
| `--tls-cert`        | Path to TLS certificate               |
| `--tls-key`         | Path to TLS private key               |
| `--tls-ca`          | Path to CA certificate                |
| `--tls-server-name` | Server name for TLS verification      |
| `--tls-skip-verify` | Skip TLS verification (insecure)      |
| `--codec-endpoint`  | Codec server URL for decoding payloads |
| `--theme`           | Theme name                            |

### Keybindings

**Navigation**
| Key | Action |
|-----|--------|
| `j` / `k` | Navigate down / up |
| `Tab` | Switch pane (Primary / Secondary / Tertiary) |
| `Shift+Option+←↑↓→` | Resize the focused pane |
| `Enter` | Select / expand |
| `Esc` / `Backspace` | Go back |
| `q` | Quit (from root view) |

The **Primary** pane is Workflows, Task Queues, Schedules, or Workers. **Secondary** is the preview. **Tertiary** is Input/Output, Activity Details, or Event Details.

**Global**
| Key | Action |
|-----|--------|
| `?` | Show help |
| `T` | Theme selector |
| `P` | Profile selector |
| `:` | Command mode |
| `Ctrl+O` | Toggle mouse support (off allows terminal text selection) |
| `/` | Filter (in workflow list) |
| `f` | Add a temporary filter clause (in workflow list) |
| `F` | Manage saved filters (in workflow list) |
| `S` | Save temporary filter clauses into the active filter, or as a new one on All (in workflow list) |

**Workflow Actions**
| Key | Action |
|-----|--------|
| `c` | Cancel workflow |
| `t` | Terminate workflow |
| `s` | Signal workflow |
| `u` | Open workflow in the Temporal Web UI |

## Configuration

Configuration is stored in `~/.config/tempo/config.yaml` (or `$XDG_CONFIG_HOME/tempo/config.yaml`).

```yaml
theme: tokyonight-night
active_profile: local

profiles:
  local:
    address: localhost:7233
    namespace: default

  staging:
    address: temporal.staging.example.com:7233
    namespace: staging
    codec_endpoint: https://codec.example.com
    ui_url: https://temporal.staging.example.com
    tls:
      cert: /path/to/client.pem
      key: /path/to/client-key.pem
      ca: /path/to/ca.pem
```

Auto-refresh (`a` on workflows and namespaces) reloads every second by default.
Set `refresh_rate` to a Go duration (`500ms`, `2s`) or a bare number of seconds:

```yaml
refresh_rate: 1s
workflow_page_size: 100
show_scrollbars: true
preview_load_delay: 200ms
```

`workflow_page_size` is how many workflows each list page fetches (default 100, between 10 and 1000). Scrolling loads more pages.

`filter_roots_only: true` makes a filter in the tree view pick root workflows only, those no other workflow started, and shows every child of those roots under them whether it matches or not. Children are looked up by their parent's ID as the roots load. It is off by default, and the flat list and `/` searches always filter every workflow.

`show_scrollbars` draws a track and thumb on overflow tables, text views, and the event tree (default on). Set `show_scrollbars: false` to hide them.

`preview_load_delay` is how long preview waits after you highlight a workflow before fetching its history (default `200ms`). That keeps rapid movement from firing a request per row. Set `0` to load immediately. Values are Go durations, clamped to 2s.

The workers tab decides whether an instance is still alive from how recently it
was last seen, because Temporal keeps listing a worker for minutes after it
stops. Both allowances can be tuned:

```yaml
worker_poll_quiet_after: 90s      # task queue poll registry entry unchanged
worker_heartbeat_quiet_after: 3m  # worker heartbeat unchanged, when nothing is polling
```

Past those, the instance reads as `Stale` instead of `Running` or `Polling`.
Values are Go durations, clamped between 5s and 1h.

`codec_endpoint` is the Temporal codec server base URL (the same value as `temporal --codec-endpoint`). Tempo POSTs to `/encode` and `/decode`, sends `X-Namespace`, and substitutes `{namespace}` in the URL when present. Environment variables are expanded (`${TEMPORAL_CODEC_URL}`).

### Triggers

Triggers run a script or open a link when something happens in tempo. Both
events work on the input/output tree (`io_tree: true`), in the Input/Output
window (`i`) and the preview panes:

- `io_enter` (the default) fires when you press `Enter` on a value.
- `io_show` fires on its own, in the background, for every matching value when
  an input or output is shown, and adds what its command prints to the payload.

```yaml
io_tree: true

triggers:
  # Open any link inside the highlighted value.
  - name: open-link
    match: 'https?://\S+'
    url: '{match}'

  # Jump from an order ID to the admin page, only under an orderId key.
  - name: order-admin
    path: 'orderId$'
    match: '^ord_(\w+)$'
    url: 'https://admin.example.com/orders/{1}'

  # Hand the value to your own script.
  - name: inspect
    path: '^result\.debug$'
    cmd: ~/bin/tempo-inspect {value}
    output: log
    confirm: true
```

The first trigger whose conditions hold wins, so list the specific ones first.
Profile triggers (`profiles.<name>.triggers`) are tried before global ones.
`Enter` on an object or array, or on a value no trigger matches, still folds.

| Field | Meaning |
|-------|---------|
| `on` | `io_enter` (default) or `io_show`. |
| `match` | Regular expression the value must contain. Without it the whole value matches. |
| `path` | Regular expression for the value's dotted path, such as `items.0.url`. |
| `url` | Open this in the browser. It must expand to an `http` or `https` URL. |
| `cmd` | Run this through `sh`. |
| `output` | For `cmd`: `none` (default, runs in the background and reports failure), `log`, or `json`. |
| `confirm` | Ask before opening or running. |

Placeholders: `{value}` is the highlighted value, `{match}` what `match`
matched, `{1}`, `{2}`, … its capture groups, `{path}` and `{key}` where the
value sits, plus `{workflow_id}`, `{run_id}`, `{workflow_type}`,
`{namespace}`, and `{profile}`. A placeholder with nothing to fill it stops the
trigger with an error.

In `cmd`, every placeholder is substituted already shell-quoted, because
payloads can hold anything. Write `open {match}`, not `open "{match}"`. Scripts
can also read `TEMPO_VALUE`, `TEMPO_MATCH`, `TEMPO_PATH`, `TEMPO_KEY`,
`TEMPO_WORKFLOW_ID`, `TEMPO_RUN_ID`, `TEMPO_WORKFLOW_TYPE`, `TEMPO_NAMESPACE`,
`TEMPO_ADDRESS`, and `TEMPO_PROFILE` from the environment, which is easier for
multi-line JSON values.

#### Adding to a payload with `io_show`

An `io_show` trigger's command runs once per matching value. Its stdout is added
to the payload under the trigger's name: as JSON when it parses, so it can be
folded and browsed like the rest, otherwise as text. A payload that is one
object gets a new key; any other payload gets one more value after it. Two
matches from the same trigger are numbered (`s3-output 1`, `s3-output 2`).

```yaml
triggers:
  # Show the result a workflow wrote to S3 next to the key it returned.
  - name: s3-output
    on: io_show
    path: '(^|\.)output_s3_key$'
    cmd: aws s3 cp s3://my-bucket/{value} -
```

While the command runs the entry reads `fetching…`, and the tree redraws when it
finishes. Results are cached, so showing the payload again does not run the
command again. A command that fails shows the last line it wrote to stderr and
is tried again after 30 seconds; one that runs past 30 seconds is stopped.
Only stdout is added, so warnings on stderr cannot spoil the JSON. `url`,
`output` and `confirm` do not apply to `io_show`.

Only `{name}` and `{1}` count as placeholders, so a command can contain JSON or
a `jq` program: `cmd: aws s3 cp s3://my-bucket/{value} - | jq '{rows: .rows}'`.

## Themes

<p align="center">
  <img src="./assets/tempo-themeselect.gif" alt="Theme Selection" width="800">
</p>

26 themes are available, organized by color scheme family:

**Dark Themes**

- `tokyonight-night`, `tokyonight-storm`, `tokyonight-moon`
- `catppuccin-mocha`, `catppuccin-macchiato`, `catppuccin-frappe`
- `dracula`
- `nord`
- `gruvbox-dark`
- `onedark`
- `solarized-dark`
- `rosepine`, `rosepine-moon`
- `kanagawa`
- `everforest-dark`
- `monokai`
- `github-dark`

**Light Themes**

- `tokyonight-day`
- `catppuccin-latte`
- `dracula-light`
- `gruvbox-light`
- `onelight`
- `solarized-light`
- `rosepine-dawn`
- `everforest-light`
- `github-light`

Press `T` to open the theme selector with live preview.

## Requirements

- Go 1.21+
- A running Temporal server

MIT License - see [LICENSE](LICENSE) for details.

## Ways to Contribute

You can contribute by:

- reporting bugs
- Proposing features or design improvements
- Improving documentation
- Fixing issues labeled good first issue

To report a bug, make a feature request, and more, visit our issues page

## Acknowledgments

- [Temporal](https://temporal.io) - The workflow engine this client connects to
- [tview](https://github.com/rivo/tview) - Terminal UI library
- [jig](https://github.com/atterpac/jig) - UI component framework
