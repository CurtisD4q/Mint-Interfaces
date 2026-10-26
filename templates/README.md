# Mint Templates — 6 Base Archetypes, 8 Mixins & 28 Template Types (`T01`–`T28`)

Every compatible Linux CLI program can be wrapped in Mint using **6 Base Archetypes**, **8 Composable Mixins**, and **28 Template Types (`T01`–`T28`)** provided by `mint/appkit` and automatically detected by `tools/mint-gen`.

---

## 1. Starter Directories in `templates/`

| Directory | Archetype | Purpose | Covered Templates |
|---|---|---|---|
| `templates/config/` | `ArchetypeConfig` (`config`) | **State Editor**: reads live system/app state, renders controls or rule tables, and applies changes | `T01`, `T02`, `T03`, `T04` |
| `templates/runner/` | `ArchetypeRunner` (`runner`) | **Task & Pipeline**: target + flags + presets $\rightarrow$ background `runner.Job` + `Progress` bar $\rightarrow$ output + history | `T05`, `T06`, `T07`, `T10`, `T11`, `T15`, `T16` |
| `templates/scanner/` | `ArchetypeScanner` (`scanner`) | **Target $\rightarrow$ Live Table + Log**: target/interface + preset $\rightarrow$ live parsed `mint.Table` + collapsible logs | `T12`, `T13`, `T14` |
| `templates/monitor/` | `ArchetypeMonitor` (`monitor`) | **Live Poller & Controller**: periodic `Poll` command $\rightarrow$ gauges & live `mint.Table` $\rightarrow$ per-row actions | `T17`, `T18`, `T19`, `T20` |
| `templates/explorer/` | `ArchetypeExplorer` (`explorer`) | **Query $\rightarrow$ Results $\rightarrow$ Detail**: query/search bar $\rightarrow$ parsed `mint.Table` or `mint.Collapsible` sections | `T24`, `T25`, `T26`, `T27` |
| `templates/suite/` | `ArchetypeSuite` (`suite`) | **Multi-Subcommand Manager**: subcommand mode selector (`SubcommandSpec`) with per-mode flags, tables, and actions | `T08`, `T09`, `T21`, `T22`, `T23`, `T28` |
| `templates/app/` | Low-level `mint` | **Raw `mint` Starter**: direct `mint.App` + `mint.Screen` + `cli.go` helpers for custom multi-screen apps | Custom / hand-crafted |

---

## 2. The 8 Composable Mixins (`mint/appkit`)

| Mixin | `ToolSpec` Fields | Behaviour Added |
|---|---|---|
| `WithFileOutput` | `OutputsFile`, `OutputFlag`, `DefaultExt` | Adds the **Output File** section (`Save directory`, editable `File name`, timestamped filename generator, post-run verification, and the **Output Directory Browser** screen) |
| `WithTableOutput` | `TableOutput`, `TableColumns`, `RowActions` | Automatically parses tabular stdout (`mint.ParseTable`) into an interactive `mint.Table` with per-row detail or action modals |
| `WithCollapsibleOutput` | `CollapsibleOutput`, `ChunkSize` | Automatically splits section-headed or long output into foldable `mint.Collapsible` blocks |
| `WithProfiles` | `Profiles []ProfileSpec` | Adds a preset selector (`Default`, `Quick`, `Intense`, etc.) that populates flags and extra args |
| `WithDynamicChoices` | `FlagSpec.ChoicesCmd []string` | Populates `FlagChoice` options at runtime by running a command (e.g., `ip -o link`, `lsblk -dn -o NAME`) |
| `WithRootAuth` | `NeedsRoot` | Adds a `Run as root (sudo -S)` toggle and a `mint.Password` prompt; the password is sent to `sudo -S` on standard input |
| `WithConfirm` | `ConfirmRun`, `ConfirmPrompt` | Asks before running (`mint.Confirm`). Tools marked `Irreversible` (`dd`, `shred`, `rm`, `kill`, `pkill`…) need the target typed instead (`mint.TypeToConfirm`) |
| `WithHistory` | `KeepHistory` | Saves past runs, command lines, durations, and output paths to `~/.local/share/mint-<tool>/runs.json` |

---

## 3. All 28 Template Types (`T01`–`T28`)

| ID | Slug | Name | Base Archetype | Enabled Mixins | Representative CLI Programs |
|---|---|---|---|---|---|
| `T01` | `system-config` | System & Daemon Configurator | `config` | `WithRootAuth`, `WithTableOutput` | `sysctl`, `timedatectl`, `hostnamectl`, `localectl`, `loginctl`, `resolvectl` |
| `T02` | `dotfile-config` | Dotfile / App Config Editor | `config` | `WithCollapsibleOutput`, `WithTableOutput` | `git config`, `npm config`, `pip config`, `gh config`, `gsettings`, `dconf` |
| `T03` | `hardware-controller` | Hardware & Device Controller | `config` | `WithDynamicChoices`, `WithTableOutput` | `xrandr`, `amixer`, `pactl`, `wpctl`, `brightnessctl`, `bluetoothctl`, `nmcli`, `rfkill` |
| `T04` | `rule-table-editor` | Firewall & Rule-Table Editor | `config` | `WithTableOutput`, `WithRootAuth`, `WithConfirm` | `ufw`, `iptables`, `nft`, `firewall-cmd`, `ip route`, `ip rule` |
| `T05` | `batch-runner` | Single-Target Batch Runner | `runner` | `WithConfirm`, `WithHistory`, `WithProfiles` | `rsync`, `scp`, `dd`, `shred`, `cp`, `mv` |
| `T06` | `media-transcoder` | Media & Document Transcoder | `runner` | `WithFileOutput`, `WithProfiles`, `WithHistory` | `ffmpeg`, `magick`, `convert`, `pandoc`, `sox`, `gs`, `qrencode`, `dot` |
| `T07` | `downloader` | Downloader & Stream Fetcher | `runner` | `WithFileOutput`, `WithProfiles`, `WithHistory` | `curl`, `wget`, `yt-dlp`, `aria2c`, `axel` |
| `T08` | `archive-manager` | Archive & Compression Tool | `suite` | `WithFileOutput`, `WithTableOutput`, `WithHistory` | `tar`, `7z`, `zip`, `unzip`, `gzip`, `xz`, `zstd`, `cpio` |
| `T09` | `backup-manager` | Backup & Snapshot Manager | `suite` | `WithTableOutput`, `WithConfirm`, `WithHistory` | `borg`, `restic`, `rclone`, `timeshift`, `snapper` |
| `T10` | `build-test-runner` | Build, Test & Compiler Runner | `runner` | `WithCollapsibleOutput`, `WithProfiles`, `WithHistory` | `go`, `make`, `cargo`, `ninja`, `cmake`, `pytest`, `gcc`, `clang` |
| `T11` | `benchmark-tester` | Benchmark & Stress Tester | `runner` | `WithTableOutput`, `WithProfiles`, `WithHistory`, `WithFileOutput` | `hyperfine`, `ab`, `wrk`, `sysbench`, `fio`, `iperf3`, `stress-ng` |
| `T12` | `network-scanner` | Network & Port Scanner | `scanner` | `WithTableOutput`, `WithCollapsibleOutput`, `WithFileOutput`, `WithProfiles`, `WithRootAuth`, `WithHistory` | `nmap`, `masscan`, `rustscan`, `arp-scan`, `fping`, `netdiscover` |
| `T13` | `packet-analyser` | Packet & Traffic Analyser | `scanner` | `WithDynamicChoices`, `WithTableOutput`, `WithFileOutput`, `WithRootAuth`, `WithHistory` | `tshark`, `tcpdump`, `ngrep`, `ss`, `netstat` |
| `T14` | `osint-recon` | OSINT & Recon Gatherer | `scanner` | `WithTableOutput`, `WithCollapsibleOutput`, `WithFileOutput`, `WithHistory`, `WithProfiles` | `whois`, `dig`, `host`, `nslookup`, `traceroute`, `mtr`, `subfinder`, `theHarvester` |
| `T15` | `security-auditor` | Security & Vulnerability Auditor | `runner` | `WithCollapsibleOutput`, `WithFileOutput`, `WithProfiles`, `WithRootAuth`, `WithHistory` | `nikto`, `lynis`, `clamscan`, `rkhunter`, `trivy`, `checksec`, `sqlmap` |
| `T16` | `crypto-hash-tool` | Password, Hash & Crypto Utility | `runner` | `WithFileOutput`, `WithProfiles`, `WithHistory` | `openssl`, `gpg`, `age`, `sha256sum`, `md5sum`, `b2sum`, `john`, `hashcat` |
| `T17` | `resource-monitor` | Live System Resource Monitor | `monitor` | `WithTableOutput` | `free`, `vmstat`, `iostat`, `mpstat`, `sensors`, `acpi`, `uptime`, `df` |
| `T18` | `service-controller` | Service & Daemon Controller | `monitor` | `WithTableOutput`, `WithRootAuth`, `WithConfirm` | `systemctl`, `supervisorctl`, `rc-service`, `service` |
| `T19` | `process-inspector` | Process & Open-File Inspector | `monitor` | `WithTableOutput`, `WithConfirm` | `ps`, `pgrep`, `pkill`, `lsof`, `fuser`, `pstree` |
| `T20` | `log-analyser` | Log Streamer & Journal Analyser | `monitor` | `WithCollapsibleOutput`, `WithFileOutput` | `journalctl`, `dmesg`, `tail`, `ausearch`, `last`, `lastlog` |
| `T21` | `vcs-suite` | Version Control & Repo Suite | `suite` | `WithTableOutput`, `WithCollapsibleOutput` | `git`, `gh`, `glab`, `hg`, `svn` |
| `T22` | `container-orchestrator` | Container & VM Orchestrator | `suite` | `WithTableOutput`, `WithConfirm` | `docker`, `podman`, `kubectl`, `virsh`, `lxc`, `vagrant` |
| `T23` | `package-manager` | Package & Plugin Manager | `suite` | `WithTableOutput`, `WithRootAuth`, `WithConfirm` | `pacman`, `yay`, `paru`, `apt`, `dnf`, `flatpak`, `snap`, `pip`, `npm` |
| `T24` | `disk-inspector` | Disk, Partition & Filesystem Inspector | `explorer` | `WithDynamicChoices`, `WithTableOutput`, `WithRootAuth` | `lsblk`, `blkid`, `findmnt`, `du`, `smartctl`, `hdparm`, `btrfs` |
| `T25` | `content-searcher` | File & Content Searcher | `explorer` | `WithTableOutput`, `WithFileOutput` | `rg`, `grep`, `find`, `fd`, `locate`, `ack`, `exiftool`, `file`, `strings` |
| `T26` | `data-query-explorer` | Structured Data & Query Explorer | `explorer` | `WithTableOutput`, `WithCollapsibleOutput`, `WithFileOutput` | `jq`, `yq`, `sqlite3`, `csvtk`, `mlr`, `awk`, `sed`, `column`, `sort`, `uniq` |
| `T27` | `doc-browser` | Reference & Documentation Browser | `explorer` | `WithCollapsibleOutput` | `man`, `tldr`, `apropos`, `whatis`, `info` |
| `T28` | `schedule-manager` | Schedule, Cron & Timer Manager | `suite` | `WithTableOutput`, `WithConfirm` | `crontab`, `at`, `anacron` |
