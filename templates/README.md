# Mint Starters — 36 Archetypes & Mixins

Every compatible Linux CLI program can be wrapped in Mint using **36 archetypes** (what kind of program it is) and **composable mixins** (add-ons), provided by `mint/appkit`. `tools/mint-gen` detects a program's archetype. `MainScreen` is one screen assembled from that archetype: what it takes, how it runs, and what it shows.

---

## 1. Starter Directories in `templates/`

These folders are small example programs. They are not separate screen types: each sets an `ArchetypeID`, and `MainScreen` builds the interface from that archetype.

| Directory | Example | How it runs |
|---|---|---|
| `templates/config/` | Settings (`sysctl`) | Read state, edit, apply |
| `templates/runner/` | Transfer (`curl`) | One shot: input, a job, then the output |
| `templates/scanner/` | Probe (`nmap`) | One shot, with a results table on the page |
| `templates/monitor/` | Control (`systemctl`) | Live refresh, with an action on each row |
| `templates/explorer/` | Search (`rg`) | One shot: a query, then the matches below |
| `templates/suite/` | History (`git`) | Subcommand tabs when the spec lists commands |
| `templates/app/` | Hand-built `mint` | A raw `mint.App` for a custom multi-screen program |

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

## 3. The 36 Archetypes

Each archetype says what a program takes, how it runs and what it shows. Its blueprint is every add-on wanted at weight 0.5 or more. `MainScreen` uses that blueprint: a target, two inputs, or subcommand tabs; a one-shot run, a live poll, or a state editor; and an inline result when the archetype shows one. `mint-gen --list-archetypes` lists them, and `mint-gen --archetype <id> <program>` chooses one yourself. Programs named below are on the archetype lists (`mint/appkit/archetypetools.go`); any other program gets the archetype its help points to.

| ID | Name | Group | Runs | What it is | Add-ons | Listed programs (examples) |
|---|---|---|---|---|---|---|
| `ask` | Ask | Answer once | once | Answers a question or works out an expression. | history, target | `uptime`, `whatis` |
| `look-up` | Look up | Answer once | once | Looks a name up somewhere else and shows its records. | table, history, target | `amass`, `dig`, `dnsrecon`, `drill`, `host` |
| `inspect` | Inspect | Answer once | once | Describes one item in detail. | collapsible, target | `acpi`, `b2sum`, `blkid`, `cksum`, `exiftool` |
| `list` | List | Answer once | once | Lists the items of a collection. | table, dynamic-choices | `conntrack`, `df`, `du`, `faillock`, `findmnt` |
| `search` | Search | Answer once | once | Finds what matches a pattern. | table, collapsible, history, target | `ack`, `ag`, `apropos`, `apt-cache`, `ausearch` |
| `query` | Query | Answer once | once | Runs a query language over data. | table, file-output, profiles, history, target | `awk`, `gawk`, `jq`, `mlr`, `yq` |
| `compare` | Compare | Answer once | once | Shows the differences between inputs. | diff, two-inputs |  |
| `read` | Read | Answer once | once | Browses documents or manuals. | collapsible, history, target | `go-doc`, `help`, `info`, `man`, `perldoc` |
| `watch` | Watch | Observe over time | live | Samples values again and again. | table, live-refresh | `free`, `iostat`, `mpstat`, `nvidia-smi`, `pidstat` |
| `follow` | Follow | Observe over time | once | Shows events as they happen. | collapsible, follow | `dmesg`, `journalctl`, `ngrep`, `tail` |
| `record` | Record | Observe over time | once | Records a live source to a file. | file-output, dynamic-choices, root, progress | `dumpcap`, `tcpdump`, `tshark` |
| `probe` | Probe | Act on targets | once | Checks targets and gives one result for each. | table, file-output, profiles, root, history, progress, target | `arp-scan`, `ffuf`, `fping`, `gobuster`, `masscan` |
| `audit` | Audit | Act on targets | once | Runs checks and lists findings. | table, collapsible, file-output, history, findings, target | `checksec`, `chkrootkit`, `clamscan`, `eslint`, `grype` |
| `measure` | Measure | Act on targets | once | Runs a workload and reports numbers. | table, profiles, history, progress | `ab`, `fio`, `hey`, `hyperfine`, `iperf3` |
| `convert` | Convert | Make something | once | Turns an input into an output of another form. | file-output, profiles, progress | `age`, `base64`, `convert`, `dd`, `dot` |
| `filter` | Filter | Make something | once | Transforms text: input in, output out. | collapsible, file-output | `column`, `cut`, `sed`, `sort`, `tr` |
| `pack` | Pack | Make something | commands | Packs many files into one, or unpacks them. | table, file-output, confirm, progress | `7z`, `7za`, `ar`, `bzip2`, `cpio` |
| `generate` | Generate | Make something | once | Makes something new from settings alone. | file-output, history | `qrencode` |
| `build` | Build | Make something | once | Builds or tests a project. | collapsible, profiles, history, progress | `cargo`, `clang`, `cmake`, `g++`, `gcc` |
| `transfer` | Transfer | Make something | once | Fetches or sends something over the network. | file-output, profiles, progress, target | `aria2c`, `axel`, `curl`, `http`, `scp` |
| `present` | Present | Make something | once | Shows, plays, prints or announces something; that is its effect. | dynamic-choices, target |  |
| `edit-files` | Edit files | Change things | once | Changes many files in place: copies, moves, renames, removes. | confirm, preview | `cp`, `install`, `mv`, `shred` |
| `mirror` | Mirror | Change things | once | Makes a destination match a source. | profiles, confirm, preview, progress | `rsync` |
| `settings` | Settings | Change things | state | Reads and sets named values. | table, row-actions | `dconf`, `gh-config`, `git-config`, `gsettings`, `hostnamectl` |
| `device` | Device | Change things | state | Adjusts a device or session live. | dynamic-choices | `amixer`, `bluetoothctl`, `brightnessctl`, `hdparm`, `iwctl` |
| `rules` | Rules | Change things | state | Edits an ordered list of rules or entries. | table, root, confirm, reorder, row-actions | `firewall-cmd`, `ip-route`, `ip-rule`, `ip6tables`, `iptables` |
| `control` | Control | Change things | live | Lists running things and acts on each. | table, root, confirm, live-refresh, row-actions | `incus`, `loginctl`, `lxc`, `pct` |
| `signal` | Signal | Change things | once | Sends a signal to running processes, or changes their priority, picking them by name or pattern. | root, confirm, history, target | `killall`, `pkill` |
| `records` | Records | Manage a collection | commands | Adds, lists, changes and removes items in a store. | table, confirm, row-actions | `apk`, `apt`, `apt-get`, `brew`, `dnf` |
| `history` | History | Manage a collection | commands | Keeps versions or snapshots of something. | table, confirm, history, row-actions | `borg`, `duplicity`, `fossil`, `git`, `hg` |
| `toolkit` | Toolkit | Manage a collection | commands | Many unrelated commands under one name. | collapsible | `btrfs`, `csvtk`, `docker`, `gh`, `glab` |
| `wrap` | Wrap | Run other things | once | Runs another command with changes: a limit, a priority, a timer. | collapsible, target |  |
| `schedule` | Schedule | Run other things | commands | Runs something later or on a timetable. | table, confirm, row-actions | `anacron`, `at`, `atq`, `atrm`, `batch` |
| `serve` | Serve | Run other things | once | Runs as a service until stopped. | collapsible, service |  |
| `session` | Session | Run other things | once | An interactive prompt, driven one command at a time. | history, transcript | `mysql`, `psql`, `sftp`, `sqlite3` |
| `general` | General | Anything else | once | Anything else: a command and its output. | collapsible |  |

The old 28 templates (`T01`–`T28`) are gone. A spec made before archetypes that still sets `Template:` no longer builds; give it an `ArchetypeID` instead.
