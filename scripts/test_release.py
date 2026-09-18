"""Exercise release.sh with temporary builds and injected tool failures."""

import hashlib
import os
from pathlib import Path
import shlex
import subprocess
import sys
import tempfile
import unittest
import zipfile


SCRIPT = Path(__file__).with_name("release.sh").resolve()
BASH = os.environ.get("IPGW_TEST_BASH", "bash")
PLATFORMS = ["linux-amd64", "windows-amd64"]


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ipgw-release-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.build = self.root / "build with spaces"
        self.output = self.root / "release with spaces"
        self.shims = self.root / "tools"
        self.shims.mkdir()
        for platform in PLATFORMS:
            binary = self.binary(platform)
            binary.parent.mkdir(parents=True)
            binary.write_bytes(b"test executable " + platform.encode())
            # A release must contain only the expected executable.
            binary.with_name("unrelated.txt").write_text("not a release asset")
        self.wrapper("zip", '''
if [[ "$IPGW_TEST_FAILURE" == zip && "$*" == *windows-amd64* ]]; then exit 41; fi
if [[ "$IPGW_TEST_FAILURE" == corrupt_zip ]]; then printf broken > "$2"; exit 0; fi
if command -v /usr/bin/zip >/dev/null && [[ "$IPGW_TEST_FAILURE" != wrong_entry ]]; then
    exec /usr/bin/zip "$@"
fi
exec "$IPGW_TEST_PYTHON" "$IPGW_TEST_HELPER" --zip "$@"
''')
        self.wrapper("sha256sum", '''
if [[ "$IPGW_TEST_FAILURE" == checksum_write && "$1" != --check ]]; then exit 42; fi
if [[ "$IPGW_TEST_FAILURE" == checksum_verify && "$1" == --check ]]; then exit 43; fi
if [[ "$IPGW_TEST_FAILURE" == checksum_mismatch && "$1" != --check ]]; then
    shift
    for archive in "$@"; do printf '%064d  %s\\n' 0 "$archive"; done
    exit 0
fi
exec /usr/bin/sha256sum "$@"
''')
        self.wrapper("mv", '''
if [[ "$IPGW_TEST_FAILURE" == move ]]; then exit 44; fi
exec /usr/bin/mv "$@"
''')

    def binary(self, platform):
        return self.build / platform / ("ipgw.exe" if platform.startswith("windows-") else "ipgw")

    def wrapper(self, name, body):
        path = self.shims / name
        path.write_text("#!/usr/bin/env bash\nset -eu\n" + body, newline="\n")
        path.chmod(0o755)

    def run_release(self, failure="", platforms=None, relative=False):
        env = dict(os.environ, IPGW_TEST_FAILURE=failure,
                   IPGW_TEST_PYTHON=Path(sys.executable).as_posix(),
                   IPGW_TEST_HELPER=Path(__file__).resolve().as_posix())
        # Git Bash needs POSIX paths in PATH; command arguments may use drive paths.
        shim_path = self.shims.as_posix()
        if os.name == "nt":
            converted = subprocess.run([BASH, "--noprofile", "--norc", "-c",
                                        'PATH="/usr/bin:$PATH" cygpath -u "$1"', "test", shim_path],
                                       check=True, capture_output=True, text=True, encoding="utf-8", errors="replace")
            shim_path = converted.stdout.strip()
        command = 'export PATH=' + shlex.quote(shim_path) + ':/usr/bin:$PATH; exec bash "$@"'
        args = [SCRIPT.as_posix(), "ipgw",
                self.build.name if relative else self.build.as_posix(),
                self.output.name if relative else self.output.as_posix(),
                *(PLATFORMS if platforms is None else platforms)]
        return subprocess.run([BASH, "--noprofile", "--norc", "-c", command, "test", *args],
                              cwd=self.root, env=env, capture_output=True, text=True,
                              encoding="utf-8", errors="replace", timeout=30)

    def assert_failed(self, result):
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("Release ready:", result.stdout)
        self.assertFalse(self.output.exists(), "partial release was exposed")
        self.assertEqual(list(self.root.glob(self.output.name + ".tmp.*")), [])
        for platform in PLATFORMS:
            self.assertTrue(self.binary(platform).exists(), "build input was removed")

    def test_complete_release(self):
        for relative in (False, True):
            with self.subTest(relative=relative):
                # Both an absent output directory and an existing empty one work.
                if relative:
                    for asset in self.output.iterdir():
                        asset.unlink()
                result = self.run_release(relative=relative)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                expected = {f"ipgw-{p}.zip" for p in PLATFORMS}
                self.assertEqual({p.name for p in self.output.iterdir()}, expected | {"checksums.txt"})
                checksums = {}
                for line in (self.output / "checksums.txt").read_text().splitlines():
                    digest, name = line.split()
                    checksums[name.lstrip("*")] = digest
                self.assertEqual(set(checksums), expected)
                for platform in PLATFORMS:
                    archive = self.output / f"ipgw-{platform}.zip"
                    self.assertEqual(hashlib.sha256(archive.read_bytes()).hexdigest(), checksums[archive.name])
                    with zipfile.ZipFile(archive) as zipped:
                        self.assertEqual(zipped.namelist(), [self.binary(platform).name])
                        self.assertEqual(zipped.read(self.binary(platform).name), self.binary(platform).read_bytes())

    def test_tool_failures_stop_release(self):
        for failure in ("zip", "corrupt_zip", "wrong_entry", "checksum_write",
                        "checksum_verify", "checksum_mismatch", "move"):
            with self.subTest(failure=failure):
                result = self.run_release(failure=failure)
                self.assert_failed(result)
                expected_code = {"zip": 41, "checksum_write": 42, "checksum_verify": 43, "move": 44}.get(failure)
                if expected_code is not None:
                    self.assertEqual(result.returncode, expected_code, result.stdout + result.stderr)
                if failure == "wrong_entry":
                    self.assertIn("Unexpected archive contents", result.stderr)
                if failure == "checksum_mismatch":
                    self.assertIn("FAILED", result.stdout)

    def test_missing_or_empty_binary(self):
        binary = self.binary("windows-amd64")
        binary.unlink()
        result = self.run_release()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Missing or invalid executable", result.stderr)
        self.assertFalse(self.output.exists())
        binary.touch()
        self.assert_failed(self.run_release())

    def test_invalid_platform_list(self):
        for platforms in ([], ["../escape"], ["linux-amd64", "linux-amd64"]):
            with self.subTest(platforms=platforms):
                self.assert_failed(self.run_release(platforms=platforms))

    def test_existing_release_is_preserved(self):
        self.output.mkdir()
        old = self.output / "old.zip"
        old.write_bytes(b"previous release")
        result = self.run_release()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("not empty", result.stderr)
        self.assertEqual(old.read_bytes(), b"previous release")
        self.assertEqual(list(self.output.iterdir()), [old])


if __name__ == "__main__":
    if sys.argv[1:2] == ["--zip"]:
        # Git Bash has unzip/sha256sum but no zip. CI uses the real /usr/bin/zip;
        # this fallback produces real ZIPs so local integrity checks still run.
        _, archive, binary = sys.argv[2:]
        with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as zipped:
            if os.environ.get("IPGW_TEST_FAILURE") == "wrong_entry":
                zipped.writestr("unexpected.txt", b"wrong asset")
            else:
                zipped.write(binary, arcname=Path(binary).name)
    else:
        unittest.main(verbosity=2)
