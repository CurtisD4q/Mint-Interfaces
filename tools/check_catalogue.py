#!/usr/bin/env python3
"""Compile every Go example in CATALOGUE.md.

Each ```go block is built as its own package together with the starter
app's helpers (templates/app/cli.go), so examples may use tool, query,
queryJSON, apply and applyShown. Imports are added automatically.

usage: python3 tools/check_catalogue.py      (run from the repository root)
"""
import os
import re
import shutil
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WORK = os.path.join(ROOT, "zz_catalogue_check")

IMPORTS = {
    "fmt": "fmt.Sprint",
    "strconv": "strconv.Itoa",
    "strings": "strings.Fields",
    "time": "time.Second",
    "encoding/json": "json.Marshal",
    "os/exec": "exec.Command",
    "syscall": "syscall.SIGINT",
    "github.com/CurtisD4q/Mint-Interfaces/mint": "mint.Note",
    "github.com/CurtisD4q/Mint-Interfaces/mint/runner": "runner.Start",
    "github.com/CurtisD4q/Mint-Interfaces/mint/util": "util.Run",
    "github.com/CurtisD4q/Mint-Interfaces/mint/eventcache": "eventcache.New",
    "github.com/CurtisD4q/Mint-Interfaces/mint/appkit": "appkit.FlagBool",
}


def blocks(text):
    for m in re.finditer(r"^```go\n(.*?)^```", text, re.S | re.M):
        line = text[: m.start()].count("\n") + 1
        yield line, m.group(1)


def main():
    with open(os.path.join(ROOT, "CATALOGUE.md")) as f:
        text = f.read()
    with open(os.path.join(ROOT, "templates", "app", "cli.go")) as f:
        helpers = f.read()
    shutil.rmtree(WORK, ignore_errors=True)
    found = list(blocks(text))
    for i, (line, code) in enumerate(found):
        d = os.path.join(WORK, f"ex{i:02d}")
        os.makedirs(d)
        with open(os.path.join(d, "cli.go"), "w") as f:
            f.write(helpers)
        uses = "\n".join(f"var _ = {u}" for u in IMPORTS.values())
        imports = "\n".join(f'\t"{p}"' for p in IMPORTS)
        with open(os.path.join(d, "example.go"), "w") as f:
            f.write(f"package main\n\n// CATALOGUE.md line {line}\n\nimport (\n{imports}\n)\n\n{uses}\n\nfunc main() {{}}\n\n{code}")
    try:
        r = subprocess.run(["go", "vet", "./zz_catalogue_check/..."], cwd=ROOT, capture_output=True, text=True)
        out = r.stdout + r.stderr
        failed = sorted(set(re.findall(r"zz_catalogue_check/(ex\d+)", out)))
        for name in failed:
            line = found[int(name[2:])][0]
            print(f"FAIL  example at CATALOGUE.md line {line}")
        if r.returncode != 0:
            print(out)
            sys.exit(1)
        print(f"all {len(found)} catalogue examples compile")
    finally:
        shutil.rmtree(WORK, ignore_errors=True)


if __name__ == "__main__":
    main()
