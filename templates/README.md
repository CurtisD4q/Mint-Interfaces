# Mint Starters — 35 Archetypes, 6 Base Layouts & Mixins

Every compatible Linux CLI program can be wrapped in Mint using **35 archetypes** (what kind of program it is), **6 base layouts** and **composable mixins** (add-ons), provided by `mint/appkit`. `tools/mint-gen` detects a program's archetype; the archetype chooses the layout and add-ons.

---

## 1. Starter Directories in `templates/`

| Directory | Base layout | Purpose | Archetypes on this layout |
|---|---|---|---|
| `templates/config/` | `ArchetypeConfig` (`config`) | **State Editor**: reads live system/app state, renders controls or rule tables, and applies changes | Settings, Device, Rules |
| `templates/runner/` | `ArchetypeRunner` (`runner`) | **Task & Pipeline**: target + flags + presets $\rightarrow$ background `runner.Job` + `Progress` bar $\rightarrow$ output + history | Audit, Build, Convert, Edit files, Filter, Generate, Measure, Mirror, Present, Serve, Session, Transfer, Wrap, General |
| `templates/scanner/` | `ArchetypeScanner` (`scanner`) | **Target $\rightarrow$ Live Table + Log**: target/interface + preset $\rightarrow$ live parsed `mint.Table` + collapsible logs | Probe, Record |
| `templates/monitor/` | `ArchetypeMonitor` (`monitor`) | **Live Poller & Controller**: periodic `Poll` command $\rightarrow$ gauges & live `mint.Table` $\rightarrow$ per-row actions | Watch, Follow, Control |
| `templates/explorer/` | `ArchetypeExplorer` (`explorer`) | **Query $\rightarrow$ Results $\rightarrow$ Detail**: query/search bar $\rightarrow$ parsed `mint.Table` or `mint.Collapsible` sections | Ask, Look up, Inspect, List, Search, Query, Compare, Read |
| `templates/suite/` | `ArchetypeSuite` (`suite`) | **Multi-Subcommand Manager**: subcommand mode selector (`SubcommandSpec`) with per-mode flags, tables, and actions | Pack, History, Records, Schedule, Toolkit |
| `templates/app/` | Low-level `mint` | **Raw `mint` Starter**: direct `mint.App` + `mint.Screen` + `cli.go` helpers for custom multi-screen apps | Custom / hand-crafted |

---

## 2. The Composable Mixins (`mint/appkit`)

| Mixin | `ToolSpec` Fields | Behaviour Added |
|---|---|---|
| `WithFileOutput` | `OutputsFile`, `OutputFlag`, `DefaultExt` | Adds the **Output File** section (`Save directory`, editable `File name`, timestamped filename generator, post-run verification, and the **Output Directory Browser** screen) |
| `WithTableOutput` | `TableOutput`, `TableColumns`, `RowActions` | Automatically parses tabular stdout (`mint.ParseTable`) into an interactive `mint.Table` with per-row detail or action modals |
| `WithCollapsibleOutput` | `CollapsibleOutput`, `ChunkSize` | Automatically splits section-headed or long output into foldable `mint.Collapsible` blocks |
| `WithProfiles` | `Profiles []ProfileSpec` | Adds a preset selector (`Default`, `Quick`, `Intense`, etc.) that populates flags and extra args |
| `WithDynamicChoices` | `FlagSpec.ChoicesCmd []string` | Populates `FlagChoice` options at runtime by running a command (e.g., `ip -o link`, `lsblk -dn -o NAME`) |
| `WithRootAuth` | `NeedsRoot` | Adds a "Root access" section: it shows whether the app runs as root and, if not, the `sudo` command that restarts it with root rights. Mint never asks for or handles the sudo password; `mint-gen --run` offers to start these apps with sudo |
| `WithConfirm` | `ConfirmRun`, `ConfirmPrompt` | Asks before running (`mint.Confirm`). Tools marked `Irreversible` (`dd`, `shred`, `rm`, `kill`, `pkill`…) need the target typed instead (`mint.TypeToConfirm`) |
| `WithHistory` | `KeepHistory` | Saves past runs, command lines, durations, and output paths to `~/.local/share/mint-<tool>/runs.json` |

---

## 3. The 35 Archetypes

Each archetype says what a program takes, how it runs and what it shows; its blueprint picks the base layout and the add-ons it wants most. `mint-gen --list-archetypes` lists them, and `mint-gen --archetype <id> <program>` chooses one yourself. Programs named below are on the archetype lists (`mint/appkit/archetypetools.go`); any other program gets the archetype its help points to.

| ID | Name | Group | Layout | What it is | Add-ons | Listed programs (examples) |
|---|---|---|---|---|---|---|
| `ask` | Ask | Answer once | `explorer` | Answers a question or works out an expression. | history, target | `uptime`, `whatis` |
| `look-up` | Look up | Answer once | `explorer` | Looks a name up somewhere else and shows its records. | table, history, target | `amass`, `dig`, `dnsrecon`, `drill`, `host` |
| `inspect` | Inspect | Answer once | `explorer` | Describes one item in detail. | collapsible, target | `acpi`, `b2sum`, `blkid`, `cksum`, `exiftool` |
| `list` | List | Answer once | `explorer` | Lists the items of a collection. | table, dynamic-choices | `conntrack`, `df`, `du`, `faillock`, `findmnt` |
| `search` | Search | Answer once | `explorer` | Finds what matches a pattern. | table, collapsible, history, target | `ack`, `ag`, `apropos`, `apt-cache`, `ausearch` |
| `query` | Query | Answer once | `explorer` | Runs a query language over data. | table, file-output, profiles, history, target | `awk`, `gawk`, `jq`, `mlr`, `yq` |
| `compare` | Compare | Answer once | `explorer` | Shows the differences between inputs. | diff, two-inputs |  |
| `read` | Read | Answer once | `explorer` | Browses documents or manuals. | collapsible, history, target | `go-doc`, `help`, `info`, `man`, `perldoc` |
| `watch` | Watch | Observe over time | `monitor` | Samples values again and again. | table, live-refresh | `free`, `iostat`, `mpstat`, `nvidia-smi`, `pidstat` |
| `follow` | Follow | Observe over time | `monitor` | Shows events as they happen. | collapsible, follow | `dmesg`, `journalctl`, `ngrep`, `tail` |
| `record` | Record | Observe over time | `scanner` | Records a live source to a file. | file-output, dynamic-choices, root, progress | `dumpcap`, `tcpdump`, `tshark` |
| `probe` | Probe | Act on targets | `scanner` | Checks targets and gives one result for each. | table, file-output, profiles, root, history, progress, target | `arp-scan`, `ffuf`, `fping`, `gobuster`, `masscan` |
| `audit` | Audit | Act on targets | `runner` | Runs checks and lists findings. | table, collapsible, file-output, history, findings, target | `checksec`, `chkrootkit`, `clamscan`, `eslint`, `grype` |
| `measure` | Measure | Act on targets | `runner` | Runs a workload and reports numbers. | table, profiles, history, progress | `ab`, `fio`, `hey`, `hyperfine`, `iperf3` |
| `convert` | Convert | Make something | `runner` | Turns an input into an output of another form. | file-output, profiles, progress | `age`, `base64`, `convert`, `dd`, `dot` |
| `filter` | Filter | Make something | `runner` | Transforms text: input in, output out. | collapsible, file-output | `column`, `cut`, `sed`, `sort`, `tr` |
| `pack` | Pack | Make something | `suite` | Packs many files into one, or unpacks them. | table, file-output, confirm, progress | `7z`, `7za`, `ar`, `bzip2`, `cpio` |
| `generate` | Generate | Make something | `runner` | Makes something new from settings alone. | file-output, history | `qrencode` |
| `build` | Build | Make something | `runner` | Builds or tests a project. | collapsible, profiles, history, progress | `cargo`, `clang`, `cmake`, `g++`, `gcc` |
| `transfer` | Transfer | Make something | `runner` | Fetches or sends something over the network. | file-output, profiles, progress, target | `aria2c`, `axel`, `curl`, `http`, `scp` |
| `present` | Present | Make something | `runner` | Shows, plays, prints or announces something; that is its effect. | dynamic-choices, target |  |
| `edit-files` | Edit files | Change things | `runner` | Changes many files in place: copies, moves, renames, removes. | confirm, preview | `cp`, `install`, `mv`, `shred` |
| `mirror` | Mirror | Change things | `runner` | Makes a destination match a source. | profiles, confirm, preview, progress | `rsync` |
| `settings` | Settings | Change things | `config` | Reads and sets named values. | table, row-actions | `dconf`, `gh-config`, `git-config`, `gsettings`, `hostnamectl` |
| `device` | Device | Change things | `config` | Adjusts a device or session live. | dynamic-choices | `amixer`, `bluetoothctl`, `brightnessctl`, `hdparm`, `iwctl` |
| `rules` | Rules | Change things | `config` | Edits an ordered list of rules or entries. | table, root, confirm, reorder, row-actions | `firewall-cmd`, `ip-route`, `ip-rule`, `ip6tables`, `iptables` |
| `control` | Control | Change things | `monitor` | Lists running things and acts on each. | table, root, confirm, live-refresh, row-actions | `incus`, `killall`, `loginctl`, `lxc`, `pct` |
| `records` | Records | Manage a collection | `suite` | Adds, lists, changes and removes items in a store. | table, confirm, row-actions | `apk`, `apt`, `apt-get`, `brew`, `dnf` |
| `history` | History | Manage a collection | `suite` | Keeps versions or snapshots of something. | table, confirm, history, row-actions | `borg`, `duplicity`, `fossil`, `git`, `hg` |
| `toolkit` | Toolkit | Manage a collection | `suite` | Many unrelated commands under one name. | collapsible | `btrfs`, `csvtk`, `docker`, `gh`, `glab` |
| `wrap` | Wrap | Run other things | `runner` | Runs another command with changes: a limit, a priority, a timer. | collapsible, target |  |
| `schedule` | Schedule | Run other things | `suite` | Runs something later or on a timetable. | table, confirm, row-actions | `anacron`, `at`, `atq`, `atrm`, `batch` |
| `serve` | Serve | Run other things | `runner` | Runs as a service until stopped. | collapsible, service |  |
| `session` | Session | Run other things | `runner` | An interactive prompt, driven one command at a time. | history, transcript | `mysql`, `psql`, `sftp`, `sqlite3` |
| `general` | General | Anything else | `runner` | Anything else: a command and its output. | collapsible |  |

The old 28 templates (`T01`–`T28`) are gone. A spec made before archetypes that still sets `Template:` no longer builds; give it an `ArchetypeID` instead.
