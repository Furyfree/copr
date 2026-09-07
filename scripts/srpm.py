#!/usr/bin/env python3
"""Build one source RPM in temporary storage using native RPM tools."""

import argparse
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
from urllib.parse import urlsplit


def build(spec, outdir):
    spec = Path(spec).absolute()
    if not spec.is_file() or spec.is_symlink() or spec.suffix != ".spec":
        raise ValueError("spec must be a regular .spec file")
    if not outdir:
        raise ValueError("outdir is required")
    output = Path(outdir).resolve()
    if output.is_relative_to(spec.parent.resolve()):
        raise ValueError("outdir must be outside the package directory")
    expanded = subprocess.run(
        ["rpmspec", "-P", str(spec)], check=True, text=True, capture_output=True
    ).stdout
    local_sources = set()
    for line in expanded.splitlines():
        match = re.match(r"^(?:Source|Patch)\d*:\s*(\S+)\s*$", line, re.I)
        if not match:
            continue
        source = match[1]
        url = urlsplit(source)
        if url.scheme or url.netloc:
            if url.scheme != "https" or not url.hostname or url.username or url.password:
                raise ValueError("remote sources require credential-free HTTPS")
        else:
            path = spec.parent / source
            if (Path(source).name != source or path.is_symlink()
                    or not path.is_file()):
                raise ValueError("local sources must be regular files beside the spec")
            local_sources.add(path)
    with tempfile.TemporaryDirectory(prefix="copr-srpm-") as temporary:
        root = Path(temporary)
        sources = root / "SOURCES"
        sources.mkdir()
        recipe = root / spec.name
        shutil.copyfile(spec, recipe)
        for path in local_sources:
            shutil.copyfile(path, sources / path.name)
        subprocess.run(["spectool", "-g", "-C", str(sources), str(recipe)], check=True)
        subprocess.run([
            "rpmbuild", "-bs", str(recipe),
            "--define", f"_topdir {root}",
            "--define", f"_sourcedir {sources}",
        ], check=True)
        archives = list((root / "SRPMS").glob("*.src.rpm"))
        if len(archives) != 1:
            raise ValueError("expected exactly one source RPM")
        output.mkdir(parents=True, exist_ok=True)
        target = output / archives[0].name
        # Never overwrite an earlier build or follow an existing output link.
        with target.open("xb") as destination, archives[0].open("rb") as source:
            shutil.copyfileobj(source, destination)
        return target


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--spec", required=True)
    parser.add_argument("--outdir", required=True)
    args = parser.parse_args()
    try:
        print(build(args.spec, args.outdir))
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"SRPM build failed: {error}\n")
