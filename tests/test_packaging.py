import hashlib
import importlib.util
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch


ROOT = Path(__file__).resolve().parents[1]


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / "scripts" / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


srpm = load("srpm")
publisher = load("publish")


class Sources(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.package = self.root / "package"
        self.package.mkdir()
        self.spec = self.package / "hello.spec"
        self.spec.write_text("fixture")

    def test_reject_sources_before_fetch_or_build(self):
        outside = self.root / "secret"
        outside.write_text("fixture")
        (self.package / "linked").symlink_to(outside)
        for source in ["http://example.invalid/a", "https://user:token@example.invalid/a",
                       "../secret", str(outside), "linked", "missing.tar.gz"]:
            with self.subTest(source=source), patch.object(srpm.subprocess, "run") as run:
                run.return_value.stdout = f"Source0: {source}\n"
                with self.assertRaises(ValueError):
                    srpm.build(self.spec, self.root / "out")
                self.assertEqual(run.call_count, 1)
                self.assertFalse((self.root / "out").exists())

    def test_output_must_be_outside_package(self):
        with patch.object(srpm.subprocess, "run") as run:
            with self.assertRaises(ValueError):
                srpm.build(self.spec, self.package / "out")
            run.assert_not_called()

    @unittest.skipUnless(all(shutil.which(tool) for tool in ["make", "rpmbuild", "spectool"]),
                         "requires RPM build tools; run the container gate")
    def test_native_make_contract_builds_srpm_without_changing_sources(self):
        source = self.root / "hello-1.0"
        source.mkdir()
        (source / "README").write_text("test fixture\n")
        with tarfile.open(self.package / "hello.tar.gz", "w:gz") as archive:
            archive.add(source, arcname="hello-1.0")
        self.spec.write_text("""Name: hello
Version: 1.0
Release: 1
Summary: Packaging fixture
License: MIT
Source0: hello.tar.gz
BuildArch: noarch
%description
An isolated packaging fixture.
%prep
%setup -q
%files
""")
        before = {p.name: p.read_bytes() for p in self.package.iterdir()}
        output = self.root / "output"
        subprocess.run(["make", "-f", str(ROOT / ".copr/Makefile"), "srpm",
                        f"spec={self.spec.name}", f"outdir={output}"],
                       cwd=self.package, check=True, capture_output=True, text=True)
        archives = list(output.glob("*.src.rpm"))
        self.assertEqual(len(archives), 1)
        name = subprocess.check_output(["rpm", "-qp", "--qf", "%{NAME}", str(archives[0])],
                                       text=True)
        self.assertEqual(name, "hello")
        self.assertEqual(before, {p.name: p.read_bytes() for p in self.package.iterdir()})


class NimbusSource(unittest.TestCase):
    @unittest.skipUnless(shutil.which("rpmbuild") and shutil.which("rpmspec"),
                         "requires RPM build tools; run the container gate")
    def test_checksum_precedes_native_source_extraction(self):
        recipe = ROOT / "packages/nimbus/nimbus.spec"
        version = subprocess.check_output(
            ["rpmspec", "-q", "--srpm", "--qf", "%{VERSION}", str(recipe)],
            text=True)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / f"nimbus-{version}"
            source.mkdir()
            marker = source / "extraction-marker"
            marker.write_text("approved source\n")
            archive = root / f"nimbus-{version}-vendor.tar.gz"
            with tarfile.open(archive, "w:gz") as output:
                output.add(source, arcname=source.name)
            digest = hashlib.sha256(archive.read_bytes()).hexdigest()
            spec = root / "nimbus.spec"
            spec.write_text(re.sub(r"(?m)^%global source_sha256 [0-9a-f]{64}$",
                                   f"%global source_sha256 {digest}",
                                   recipe.read_text()))
            for changed in [False, True]:
                with self.subTest(changed=changed):
                    if changed:
                        marker.write_text("changed source\n")
                        with tarfile.open(archive, "w:gz") as output:
                            output.add(source, arcname=source.name)
                    build = root / ("changed" if changed else "approved")
                    result = subprocess.run(
                        ["rpmbuild", "-bp", "--nodeps", str(spec),
                         "--define", f"_topdir {build}",
                         "--define", f"_sourcedir {root}"],
                        capture_output=True, text=True)
                    log = result.stdout + result.stderr
                    extracted = list(build.rglob("extraction-marker"))
                    if changed:
                        self.assertNotEqual(result.returncode, 0, log)
                        self.assertIn("FAILED", log)
                        self.assertEqual(extracted, [], log)
                    else:
                        self.assertEqual(result.returncode, 0, log)
                        self.assertEqual(len(extracted), 1, log)
                        self.assertEqual(extracted[0].read_text(),
                                         "approved source\n")


class Publishing(unittest.TestCase):
    def setUp(self):
        self.client = Mock()
        self.client.project_proxy.get.return_value = SimpleNamespace(
            chroot_repos={"fedora-44-x86_64": [], "fedora-rawhide-x86_64": []})
        self.config_paths = []

        def client_from_config(path):
            path = Path(path)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertIn("token = fixture-token", path.read_text())
            self.config_paths.append(path)
            return self.client

        module = SimpleNamespace(Client=SimpleNamespace(create_from_config_file=client_from_config))
        self.modules = patch.dict(sys.modules, {"copr": SimpleNamespace(), "copr.v3": module})
        self.modules.start()
        self.addCleanup(self.modules.stop)
        self.env = patch.dict(os.environ, {
            "COPR_OWNER": "owner",
            "COPR_CONFIG": "[copr-cli]\nusername=owner\nlogin=fixture-login\n"
                           "token=fixture-token\ncopr_url=https://copr.fedorainfracloud.org\n",
        })
        self.env.start()
        self.addCleanup(self.env.stop)

    def test_account_validation_precedes_project_mutation(self):
        with patch.dict(os.environ, {"COPR_OWNER": "someoneelse"}):
            with self.assertRaises(ValueError):
                publisher.publish("project")
        self.client.project_proxy.add.assert_not_called()
        self.assertEqual(self.config_paths, [])

    def test_project_creation_preserves_existing_settings_and_removes_credentials(self):
        publisher.publish("project")
        options = self.client.project_proxy.add.call_args.kwargs
        self.assertTrue(options["exist_ok"])
        self.assertFalse(options["enable_net"])
        self.assertEqual(options["chroots"], ["fedora-44-x86_64"])
        self.client.project_proxy.edit.assert_not_called()
        self.assertTrue(self.config_paths)
        self.assertFalse(self.config_paths[0].exists())

    def test_missing_chroot_blocks_build_and_removes_credentials(self):
        self.client.project_proxy.get.return_value = SimpleNamespace(chroot_repos={})
        with self.assertRaises(ValueError):
            publisher.publish("project")
        self.assertFalse(self.config_paths[0].exists())

    def test_native_build_waits_and_propagates_failure_without_secret_environment(self):
        with tempfile.TemporaryDirectory() as temporary:
            archive = Path(temporary) / "hello.src.rpm"
            archive.write_bytes(b"fixture")

            def fail(command, **kwargs):
                self.assertNotIn("--nowait", command)
                self.assertEqual(command[-1], str(archive))
                self.assertEqual(command[command.index("--enable-net") + 1], "off")
                self.assertNotIn("COPR_CONFIG", kwargs["env"])
                self.assertTrue(kwargs["check"])
                self.assertNotIn("fixture-token", " ".join(command))
                raise subprocess.CalledProcessError(1, command)

            with patch.object(publisher.subprocess, "run", side_effect=fail):
                with self.assertRaises(subprocess.CalledProcessError):
                    publisher.publish("build", archive)
            self.assertFalse(self.config_paths[0].exists())


if __name__ == "__main__":
    unittest.main()
