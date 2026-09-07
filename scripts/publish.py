#!/usr/bin/env python3
"""Create the configured COPR project or submit one prepared SRPM."""

import argparse
import configparser
import os
from pathlib import Path
import re
import subprocess
import tempfile
import tomllib


ROOT = Path(__file__).resolve().parents[1]


def publish(action, srpm=None):
    with (ROOT / ".copr/project.toml").open("rb") as stream:
        project = tomllib.load(stream)["project"]
    owner = os.environ.get("COPR_OWNER", "")
    if not re.fullmatch(r"[a-z][a-z0-9_-]*", owner):
        raise ValueError("COPR_OWNER must be the Fedora account name")
    config = configparser.ConfigParser(interpolation=None)
    config.read_string(os.environ.get("COPR_CONFIG", ""))
    if not config.has_section("copr-cli"):
        raise ValueError("COPR_CONFIG must contain the generated [copr-cli] configuration")
    account = config["copr-cli"]
    if account.get("username") != owner:
        raise ValueError("COPR_CONFIG username must match COPR_OWNER")
    if account.get("copr_url", "").rstrip("/") != "https://copr.fedorainfracloud.org":
        raise ValueError("COPR_CONFIG must select the official Fedora COPR service")
    if not account.get("login") or not account.get("token"):
        raise ValueError("COPR_CONFIG requires login and token")
    if action == "build" and (not srpm or not Path(srpm).is_file()
                              or not str(srpm).endswith(".src.rpm")):
        raise ValueError("build requires an existing .src.rpm file")
    from copr.v3 import Client

    # Keep credentials out of argv, logs, and the checked-out source tree.
    with tempfile.TemporaryDirectory(prefix="copr-publish-") as temporary:
        path = Path(temporary) / "config"
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as stream:
            config.write(stream)
        client = Client.create_from_config_file(str(path))
        client.project_proxy.add(
            ownername=owner, projectname=project["name"],
            chroots=project["chroots"], description=project["description"],
            homepage=project["homepage"],
            instructions=f"sudo dnf copr enable {owner}/{project['name']}",
            enable_net=False, devel_mode=False, unlisted_on_hp=False,
            exist_ok=True,
        )
        existing = client.project_proxy.get(owner, project["name"])
        chroots = set(existing.chroot_repos)
        if not set(project["chroots"]).issubset(chroots):
            raise ValueError("existing project is missing the configured chroots")
        print(f"COPR project ready: {owner}/{project['name']}", flush=True)
        if action == "build":
            command = ["copr-cli", "--config", str(path), "build", "--enable-net", "off"]
            for chroot in project["chroots"]:
                command += ["--chroot", chroot]
            command += [f"{owner}/{project['name']}", str(Path(srpm).resolve())]
            environment = os.environ.copy()
            environment.pop("COPR_CONFIG", None)
            subprocess.run(command, check=True, env=environment)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["project", "build"])
    parser.add_argument("--srpm")
    args = parser.parse_args()
    try:
        publish(args.action, args.srpm)
    except Exception:
        # Client exception text may contain authentication details.
        parser.exit(1, "COPR operation failed; check account settings and the build output.\n")
