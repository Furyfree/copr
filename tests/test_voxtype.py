import hashlib
import importlib.util
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
path = ROOT / "packages/voxtype/prepare.py"
spec = importlib.util.spec_from_file_location("voxtype_prepare", path)
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


@unittest.skipUnless(shutil.which("rpmspec") and shutil.which("systemctl"),
                     "requires Fedora systemd/RPM tools; run the container gate")
class VoxtypeService(unittest.TestCase):
    def test_native_lifecycle_scriptlets(self):
        expanded = subprocess.check_output(
            ["rpmspec", "-P", str(ROOT / "packages/voxtype/voxtype.spec")],
            text=True)
        scripts = dict(re.findall(
            r"^%(post|preun|postun)\s*\n(.*?)(?=^%|\Z)",
            expanded, flags=re.M | re.S))
        self.assertEqual(set(scripts), {"post", "preun", "postun"})
        self.assertNotIn("%systemd_", "".join(scripts.values()),
                         "systemd-rpm-macros must be installed")
        cases = [
            ("post", "1", "install-user-units voxtype.service\n"),
            ("post", "2", ""),
            ("preun", "1", ""),
            ("preun", "0", "remove-user-units voxtype.service\n"),
            ("postun", "1", "mark-restart-user-units voxtype.service\n"),
            ("postun", "0", ""),
        ]
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            helper = root / "systemd-update-helper"
            log = root / "calls"
            helper.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$CALL_LOG"\n'
                              'exit "$HELPER_RESULT"\n')
            helper.chmod(0o755)
            for outcome in ["0", "1", "missing"]:
                if outcome == "missing":
                    helper.unlink()
                for phase, count, expected in cases:
                    with self.subTest(outcome=outcome, phase=phase, count=count):
                        log.write_text("")
                        # Redirect native macros to an isolated recorder; never
                        # execute the workstation's systemd helper in tests.
                        script = scripts[phase].replace(
                            "/usr/lib/systemd/systemd-update-helper", str(helper))
                        subprocess.run(
                            ["sh", "-c", script, "scriptlet", count], check=True,
                            env=os.environ | {"CALL_LOG": str(log),
                                              "HELPER_RESULT": outcome})
                        self.assertEqual(log.read_text(),
                                         "" if outcome == "missing" else expected)

    def test_fedora_preset_keeps_fresh_install_disabled(self):
        presets = list(Path("/usr/lib/systemd/user-preset").glob("*.preset"))
        self.assertTrue(presets, "Fedora user presets are required")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            units = root / "usr/lib/systemd/user"
            units.mkdir(parents=True)
            (units / "voxtype.service").write_text(
                "[Service]\nExecStart=/usr/bin/true\n"
                "[Install]\nWantedBy=default.target\n")
            policy = root / "usr/lib/systemd/user-preset"
            policy.mkdir()
            for preset in presets:
                shutil.copyfile(preset, policy / preset.name)
            command = ["systemctl", "--root", str(root), "--global"]
            subprocess.run(command + ["preset", "voxtype.service"], check=True,
                           capture_output=True, text=True)
            result = subprocess.run(command + ["is-enabled", "voxtype.service"],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 1, result.stderr)
            self.assertEqual(result.stdout.strip(), "disabled")


class VoxtypeSource(unittest.TestCase):
    def test_checksum_blocks_gpg_and_extraction(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.write_bytes(b"corrupted upstream source")
            with patch.object(prepare.subprocess, "run") as run:
                with self.assertRaisesRegex(ValueError, "checksum"):
                    prepare.verify(source, root / "sig", root / "key", root / "gpg")
                run.assert_not_called()
                self.assertFalse((root / "gpg").exists())

    def test_signer_and_native_verification_are_required(self):
        for signer in [prepare.FINGERPRINT, "A" * 40, ""]:
            with self.subTest(signer=signer), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                source = root / "source"
                source.write_bytes(b"reviewed source")
                with patch.object(prepare, "SOURCE_SHA256", hashlib.sha256(source.read_bytes()).hexdigest()), patch.object(prepare.subprocess, "run") as run:
                    run.return_value.stdout = "[GNUPG:] VALIDSIG " + signer + " 2026-09-09\n"
                    if signer == prepare.FINGERPRINT:
                        prepare.verify(source, root / "sig", root / "key", root / "gpg")
                    else:
                        with self.assertRaises(ValueError):
                            prepare.verify(source, root / "sig", root / "key", root / "gpg")
                    self.assertTrue(all(call.kwargs["check"] for call in run.call_args_list))

    def test_bad_signature_stops_preparation(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.write_bytes(b"reviewed source")
            with patch.object(prepare, "SOURCE_SHA256", hashlib.sha256(source.read_bytes()).hexdigest()), patch.object(prepare.subprocess, "run", side_effect=[subprocess.CompletedProcess([], 0), subprocess.CalledProcessError(1, "gpg")]):
                with self.assertRaises(subprocess.CalledProcessError):
                    prepare.verify(source, root / "sig", root / "key", root / "gpg")
