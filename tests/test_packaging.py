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
import tomllib
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


@unittest.skipUnless(all(shutil.which(tool) for tool in
                         ["rpmbuild", "rpmspec", "spectool", "jq", "cpio"]),
                     "requires Fedora RPM tools and jq; run the container gate")
class CopilotInstaller(unittest.TestCase):
    def test_local_sources_build_and_ship_only_the_helper(self):
        recipe = (ROOT / "packages/github-copilot-installer/"
                  "github-copilot-installer.spec")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source_rpm = srpm.build(recipe, root / "sources")
            identity = subprocess.check_output(
                ["rpm", "-qp", "--qf", "%{NAME}\n%{SOURCEPACKAGE}\n", str(source_rpm)],
                text=True)
            self.assertEqual(identity, "github-copilot-installer\n1\n")
            contents = subprocess.check_output(
                ["rpm", "-qpl", str(source_rpm)], text=True).splitlines()
            self.assertEqual(set(contents), {
                "github-copilot-installer.spec", "github-copilot-installer",
                "github-copilot-installer.1", "LICENSE", "README.md",
                "test-installer.sh",
            })
            result = subprocess.run(
                ["rpmbuild", "--rebuild", str(source_rpm),
                 "--define", f"_topdir {root / 'build'}"],
                capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("installer tests", result.stdout)
            packages = list((root / "build/RPMS/noarch").glob("*.rpm"))
            self.assertEqual(len(packages), 1)
            package = str(packages[0])
            files = subprocess.check_output(
                ["rpm", "-qpl", package], text=True).splitlines()
            self.assertEqual(set(files), {
                "/usr/bin/github-copilot-installer",
                "/usr/share/doc/github-copilot-installer",
                "/usr/share/doc/github-copilot-installer/README.md",
                "/usr/share/licenses/github-copilot-installer",
                "/usr/share/licenses/github-copilot-installer/LICENSE",
                "/usr/share/man/man1/github-copilot-installer.1.gz",
            })
            for option in ["--scripts", "--triggers"]:
                self.assertEqual(subprocess.check_output(
                    ["rpm", "-qp", option, package], text=True), "")
            payload = subprocess.check_output(["rpm2cpio", package])
            extracted = root / "extracted"
            extracted.mkdir()
            subprocess.run(["cpio", "-idm", "--quiet"], input=payload,
                           cwd=extracted, check=True)
            notice = (extracted / "usr/share/licenses/"
                      "github-copilot-installer/LICENSE").read_text()
            self.assertIn("Copyright (c) 2026 Chris Titus Tech", notice)
            version = subprocess.check_output(
                ["rpm", "-qp", "--qf", "%{VERSION}", package], text=True)
            helper = extracted / "usr/bin/github-copilot-installer"
            self.assertEqual(subprocess.check_output([str(helper), "version"],
                                                     text=True).strip(),
                             f"github-copilot-installer {version}")


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
                publisher.publish("project", "nimbus")
        self.client.project_proxy.add.assert_not_called()
        self.assertEqual(self.config_paths, [])

    def test_project_creation_preserves_existing_settings_and_removes_credentials(self):
        publisher.publish("project", "nimbus")
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
            publisher.publish("project", "nimbus")
        self.assertFalse(self.config_paths[0].exists())

    def test_native_build_waits_and_propagates_failure_without_secret_environment(self):
        with tempfile.TemporaryDirectory() as temporary:
            archive = Path(temporary) / "hello.src.rpm"
            archive.write_bytes(b"fixture")

            def fail(command, **kwargs):
                self.assertNotIn("COPR_CONFIG", kwargs["env"])
                if command[0] == "rpm":
                    return subprocess.CompletedProcess(command, 0, "nimbus\n1\n")
                self.assertNotIn("--nowait", command)
                self.assertEqual(command[-1], str(archive))
                self.assertEqual(command[command.index("--enable-net") + 1], "off")
                self.assertNotIn("COPR_CONFIG", kwargs["env"])
                self.assertTrue(kwargs["check"])
                self.assertNotIn("fixture-token", " ".join(command))
                raise subprocess.CalledProcessError(1, command)

            with patch.object(publisher.subprocess, "run", side_effect=fail):
                with self.assertRaises(subprocess.CalledProcessError):
                    publisher.publish("build", "nimbus", archive)
            self.assertFalse(self.config_paths[0].exists())

    def test_each_package_targets_only_its_own_project(self):
        with (ROOT / ".copr/projects.toml").open("rb") as stream:
            projects = tomllib.load(stream)["projects"]
        with tempfile.TemporaryDirectory() as temporary:
            archive = Path(temporary) / "candidate.src.rpm"
            archive.write_bytes(b"fixture")
            for package, project in projects.items():
                with self.subTest(package=package):
                    self.client.reset_mock()
                    with patch.object(publisher.subprocess, "run") as run:
                        run.return_value.stdout = f"{package}\n1\n"
                        publisher.publish("build", package, archive)
                    self.assertEqual(run.call_count, 2)
                    command = run.call_args.args[0]
                    self.assertEqual(command[-2:],
                                     [f"owner/{project['name']}", str(archive)])
                    self.assertEqual(command[command.index("--chroot") + 1],
                                     "fedora-44-x86_64")
                    self.assertEqual(command[command.index("--enable-net") + 1], "off")
                    self.client.project_proxy.add.assert_called_once()
                    self.assertEqual(self.client.project_proxy.add.call_args.kwargs[
                        "projectname"], project["name"])
                    self.client.project_proxy.get.assert_called_once_with(
                        "owner", project["name"])
                    self.client.project_proxy.edit.assert_not_called()

    def test_invalid_selection_and_wrong_srpm_never_mutate_a_project(self):
        for action, package, archive in [("delete", "nimbus", None),
                                         ("project", "../nimbus", None),
                                         ("project", "nimbus", "unexpected.src.rpm")]:
            with self.subTest(action=action, package=package):
                with self.assertRaises(ValueError):
                    publisher.publish(action, package, archive)
        with tempfile.TemporaryDirectory() as temporary:
            archive = Path(temporary) / "candidate.src.rpm"
            archive.write_bytes(b"fixture")
            for identity in ["nimbus\n1\n", "voxtype\n0\n", "garbage\n"]:
                with self.subTest(identity=identity):
                    with patch.object(publisher.subprocess, "run") as run:
                        run.return_value.stdout = identity
                        with self.assertRaisesRegex(ValueError, "identity"):
                            publisher.publish("build", "voxtype", archive)
                        run.assert_called_once()
            with patch.object(publisher.subprocess, "run",
                              side_effect=subprocess.CalledProcessError(1, "rpm")):
                with self.assertRaises(subprocess.CalledProcessError):
                    publisher.publish("build", "voxtype", archive)
        self.client.project_proxy.add.assert_not_called()
        self.assertEqual(self.config_paths, [])


if __name__ == "__main__":
    unittest.main()
