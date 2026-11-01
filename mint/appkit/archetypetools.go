package appkit

// archetypeTools are programs whose archetype was decided by a person, each
// looked at on its own (ps lists processes, pkill acts on them). mint-gen
// checks this list before it reads any help.
//
// Names are lower case: programs are looked up by their lower-case base name.
var archetypeTools = map[string]ArchetypeID{
	// Settings
	"sysctl": "settings", "timedatectl": "settings", "hostnamectl": "settings", "localectl": "settings",
	"resolvectl": "settings", "git-config": "settings", "npm-config": "settings", "pip-config": "settings",
	"gh-config": "settings", "gsettings": "settings", "dconf": "settings", "xfconf-query": "settings",
	// Device
	"xrandr": "device", "amixer": "device", "pactl": "device", "wpctl": "device", "brightnessctl": "device",
	"bluetoothctl": "device", "nmcli": "device", "iwctl": "device", "rfkill": "device", "hdparm": "device",
	// Rules
	"ufw": "rules", "iptables": "rules", "ip6tables": "rules", "nft": "rules", "firewall-cmd": "rules",
	"ip-route": "rules", "ip-rule": "rules", "route": "rules",
	// Edit files
	"shred": "edit-files", "cp": "edit-files", "mv": "edit-files", "install": "edit-files",
	// Mirror
	"rsync": "mirror",
	// Convert
	"dd": "convert", "ffmpeg": "convert", "magick": "convert", "convert": "convert", "mogrify": "convert",
	"pandoc": "convert", "sox": "convert", "gs": "convert", "dot": "convert", "gnuplot": "convert",
	"age": "convert", "base64": "convert",
	// Generate
	"qrencode": "generate",
	// Transfer
	"scp": "transfer", "curl": "transfer", "wget": "transfer", "yt-dlp": "transfer", "aria2c": "transfer",
	"axel": "transfer", "http": "transfer", "xh": "transfer",
	// Pack
	"tar": "pack", "7z": "pack", "7za": "pack", "zip": "pack", "unzip": "pack", "gzip": "pack", "gunzip": "pack",
	"bzip2": "pack", "xz": "pack", "zstd": "pack", "lz4": "pack", "cpio": "pack", "ar": "pack",
	// History
	"borg": "history", "restic": "history", "timeshift": "history", "snapper": "history", "duplicity": "history",
	"kopia": "history", "git": "history", "hg": "history", "svn": "history", "fossil": "history",
	// Build
	"go": "build", "make": "build", "cargo": "build", "ninja": "build", "cmake": "build", "pytest": "build",
	"gcc": "build", "g++": "build", "clang": "build", "javac": "build", "tsc": "build",
	// Audit
	"eslint": "audit", "shellcheck": "audit", "nikto": "audit", "lynis": "audit", "clamscan": "audit",
	"rkhunter": "audit", "chkrootkit": "audit", "trivy": "audit", "grype": "audit", "checksec": "audit",
	"sqlmap": "audit", "nuclei": "audit", "wpscan": "audit", "john": "audit", "hashcat": "audit",
	// Measure
	"hyperfine": "measure", "ab": "measure", "wrk": "measure", "hey": "measure", "vegeta": "measure",
	"sysbench": "measure", "fio": "measure", "iperf3": "measure", "stress-ng": "measure",
	// Probe
	"nmap": "probe", "masscan": "probe", "rustscan": "probe", "zmap": "probe", "arp-scan": "probe",
	"fping": "probe", "netdiscover": "probe", "nbtscan": "probe", "onesixtyone": "probe",
	"traceroute": "probe", "tracepath": "probe", "mtr": "probe", "gobuster": "probe", "ffuf": "probe",
	// Record
	"tshark": "record", "tcpdump": "record", "dumpcap": "record",
	// Follow
	"ngrep": "follow", "journalctl": "follow", "dmesg": "follow", "tail": "follow",
	// List
	"ss": "list", "netstat": "list", "conntrack": "list", "df": "list", "w": "list", "who": "list",
	"ps": "list", "lsof": "list", "pstree": "list", "lsns": "list", "ipcs": "list", "last": "list",
	"lastlog": "list", "faillock": "list", "lsblk": "list", "findmnt": "list", "du": "list",
	"lvs": "list", "vgs": "list", "pvs": "list",
	// Look up
	"whois": "look-up", "dig": "look-up", "host": "look-up", "nslookup": "look-up", "drill": "look-up",
	"subfinder": "look-up", "amass": "look-up", "theharvester": "look-up", "dnsrecon": "look-up",
	// Inspect
	"sha256sum": "inspect", "sha512sum": "inspect", "sha1sum": "inspect", "md5sum": "inspect",
	"b2sum": "inspect", "cksum": "inspect", "acpi": "inspect", "fuser": "inspect", "blkid": "inspect",
	"smartctl": "inspect", "exiftool": "inspect", "mediainfo": "inspect", "ffprobe": "inspect",
	"file": "inspect", "stat": "inspect", "strings": "inspect", "readelf": "inspect", "objdump": "inspect",
	"nm": "inspect", "ldd": "inspect", "wc": "inspect",
	"fastfetch": "inspect", "neofetch": "inspect", // system information, printed once
	// Watch
	"free": "watch", "vmstat": "watch", "iostat": "watch", "mpstat": "watch", "pidstat": "watch",
	"sensors": "watch", "nvidia-smi": "watch",
	// Ask
	"uptime": "ask", "whatis": "ask",
	// Control
	"loginctl": "control", "systemctl": "control", "supervisorctl": "control", "rc-service": "control",
	"service": "control", "sv": "control", "s6-svc": "control", "pkill": "signal", "killall": "signal",
	"virsh": "control", "qm": "control", "pct": "control", "lxc": "control", "incus": "control",
	"vagrant": "control",
	// Search
	"pgrep": "search", "ausearch": "search", "apt-cache": "search", "rg": "search", "grep": "search",
	"egrep": "search", "fgrep": "search", "find": "search", "fd": "search", "locate": "search",
	"plocate": "search", "ack": "search", "ag": "search", "apropos": "search",
	// Query
	"jq": "query", "yq": "query", "mlr": "query", "awk": "query", "gawk": "query",
	// Filter
	"sed": "filter", "cut": "filter", "sort": "filter", "uniq": "filter", "tr": "filter", "column": "filter",
	// Session
	"sftp": "session", "sqlite3": "session", "psql": "session", "mysql": "session",
	// Read
	"man": "read", "tldr": "read", "info": "read", "help": "read", "perldoc": "read", "pydoc3": "read",
	"go-doc": "read",
	// Records
	"helm": "records", "pacman": "records", "yay": "records", "paru": "records", "apt": "records",
	"apt-get": "records", "dnf": "records", "yum": "records", "zypper": "records", "apk": "records",
	"brew": "records", "flatpak": "records", "snap": "records", "pip": "records", "pipx": "records",
	"npm": "records", "pnpm": "records", "gem": "records",
	// Toolkit
	"ip": "toolkit", "rclone": "toolkit", "openssl": "toolkit", "gpg": "toolkit", "gh": "toolkit",
	"glab": "toolkit", "docker": "toolkit", "podman": "toolkit", "nerdctl": "toolkit", "kubectl": "toolkit",
	"nvme": "toolkit", "btrfs": "toolkit", "zfs": "toolkit", "zpool": "toolkit", "mdadm": "toolkit",
	"csvtk": "toolkit", "xsv": "toolkit",
	// Schedule
	"crontab": "schedule", "at": "schedule", "atq": "schedule", "atrm": "schedule", "anacron": "schedule",
	"batch": "schedule",
}
