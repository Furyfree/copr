#!/usr/bin/env python3
"""Verify Voxtype's release, vendor locked crates, and prepare an offline SRPM."""

import argparse
import hashlib
import gzip
import importlib.util
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request


PACKAGE = Path(__file__).resolve().parent
FINGERPRINT = "9CCF7915B750CAE8B095ED1AA3FC9F33FD209279"
SOURCE_SHA256 = "a4d0a256167f58ce90153077da82620794422f5172c918625d480ff9ffca625e"


def verify(source, signature, key, home):
    if hashlib.sha256(source.read_bytes()).hexdigest() != SOURCE_SHA256:
        raise ValueError("upstream source checksum differs from the reviewed release")
    home.mkdir(mode=0o700)
    subprocess.run(["gpg", "--batch", "--homedir", str(home), "--import", str(key)],
                   check=True, capture_output=True)
    result = subprocess.run([
        "gpg", "--batch", "--homedir", str(home), "--status-fd", "1", "--verify",
        str(signature), str(source)], check=True, capture_output=True, text=True)
    valid = [line.split()[2:] for line in result.stdout.splitlines()
             if line.startswith("[GNUPG:] VALIDSIG ")]
    if len(valid) != 1 or valid[0][0] != FINGERPRINT:
        raise ValueError("source signature does not match the pinned upstream signer")


def prepare(outdir):
    spec = (PACKAGE / "voxtype.spec").read_text()
    version = re.search(r"(?m)^Version:\s+([0-9.]+)$", spec)[1]
    output = Path(outdir).absolute()
    if output.resolve().is_relative_to(PACKAGE):
        raise ValueError("outdir must be outside the package source")
    module = importlib.util.spec_from_file_location("srpm", PACKAGE.parents[1] / "scripts/srpm.py")
    srpm = importlib.util.module_from_spec(module)
    module.loader.exec_module(srpm)
    with tempfile.TemporaryDirectory(prefix="voxtype-source-") as tmp:
        root = Path(tmp)
        source = root / "upstream.tar.gz"
        signature = root / "upstream.tar.gz.asc"
        for target, url in (
            (source, f"https://github.com/peteonrails/voxtype/archive/refs/tags/v{version}.tar.gz"),
            (signature, f"https://github.com/peteonrails/voxtype/releases/download/v{version}/voxtype-{version}.tar.gz.asc"),
        ):
            with urllib.request.urlopen(url, timeout=120) as response, target.open("xb") as dest:
                shutil.copyfileobj(response, dest)
        verify(source, signature, PACKAGE / "signing.asc", root / "gnupg")
        with tarfile.open(source) as archive:
            archive.extractall(root, filter="data")
        tree = root / f"voxtype-{version}"
        lock = (tree / "Cargo.lock").read_bytes()
        env = os.environ | {"CARGO_HOME": str(root / "cargo-home")}
        config = subprocess.run(["cargo", "vendor", "--locked", "vendor"],
                                cwd=tree, env=env, check=True, text=True,
                                stdout=subprocess.PIPE).stdout
        if (tree / "Cargo.lock").read_bytes() != lock:
            raise ValueError("vendoring changed Cargo.lock")
        (tree / ".cargo").mkdir(exist_ok=True)
        (tree / ".cargo/config.toml").write_text(config)
        # The native upstream unit requests ydotool even with compositor
        # bindings. Keep its lifecycle, but do not start an unselected backend.
        unit = tree / "packaging/systemd/voxtype.service"
        unit.write_text(unit.read_text().replace("Wants=ydotool.service\n", ""))
        recipe_dir = root / "recipe"
        recipe_dir.mkdir()
        bundle = recipe_dir / f"voxtype-{version}-vendor.tar.gz"
        def normalize(info):
            info.uid = info.gid = 0
            info.uname = info.gname = "root"
            info.mtime = 0
            info.mode = 0o755 if info.isdir() or info.mode & 0o111 else 0o644
            return info
        with bundle.open("xb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as zipped:
            with tarfile.open(fileobj=zipped, mode="w|") as archive:
                archive.add(tree, arcname=tree.name, filter=normalize)
        digest = hashlib.sha256(bundle.read_bytes()).hexdigest()
        recipe = recipe_dir / "voxtype.spec"
        recipe.write_text(spec.replace("%global source_sha256 UNPREPARED",
                                       f"%global source_sha256 {digest}"))
        return srpm.build(recipe, output)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--outdir", required=True)
    args = parser.parse_args()
    try:
        print(prepare(args.outdir))
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"Voxtype source preparation failed: {error}\n")
