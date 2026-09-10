import os
from pathlib import Path
import subprocess
import tempfile
import tomllib
import unittest

import yaml


ROOT = Path(__file__).resolve().parents[1]


class Workflow(unittest.TestCase):
    def test_package_selection_routes_source_preparation(self):
        # BaseLoader preserves GitHub's "on" key rather than treating it as
        # the YAML 1.1 boolean True.
        workflow = yaml.load((ROOT / ".github/workflows/copr.yml").read_text(),
                             Loader=yaml.BaseLoader)
        options = workflow["on"]["workflow_dispatch"]["inputs"]["package"]["options"]
        with (ROOT / ".copr/projects.toml").open("rb") as stream:
            projects = tomllib.load(stream)["projects"]
        self.assertEqual(set(options), set(projects))
        steps = workflow["jobs"]["publish"]["steps"]
        prepare = next(step["run"] for step in steps
                       if step.get("name") == "Prepare selected source RPM")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            log = root / "arguments"
            python = root / "python3"
            python.write_text('#!/bin/sh\nprintf "%s\\n" "$@" >> "$CALL_LOG"\n')
            python.chmod(0o755)
            for package in options + ["unknown"]:
                with self.subTest(package=package):
                    log.write_text("")
                    result = subprocess.run(
                        ["bash", "-c", prepare], capture_output=True, text=True,
                        env=os.environ | {"PACKAGE": package, "CALL_LOG": str(log),
                                          "PATH": f"{root}:{os.environ['PATH']}"})
                    if package == "unknown":
                        self.assertNotEqual(result.returncode, 0)
                        self.assertEqual(log.read_text(), "")
                        continue
                    self.assertEqual(result.returncode, 0, result.stderr)
                    if package == "voxtype":
                        expected = ["packages/voxtype/prepare.py",
                                    "--outdir", "/tmp/copr-srpm"]
                    else:
                        expected = ["scripts/srpm.py", "--spec",
                                    f"packages/{package}/{package}.spec",
                                    "--outdir", "/tmp/copr-srpm"]
                    self.assertEqual(log.read_text().splitlines(), expected)
