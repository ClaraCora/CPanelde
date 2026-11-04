"""Exercise actual archive building and signatures using disposable fixture keys."""
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('offline_builder', ROOT/'packaging/offline/build.py')
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)
SOURCE = '123456789abc'+'0'*28
VERSION = 'v9.8.7+'+SOURCE[:12]


class PackageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='offline-package-test-')
        self.root = Path(self.temp.name)
        self.release, self.output = self.root/'release', self.root/'output'
        self.release.mkdir()
        self.key, self.public = self.root/'fixture-private.pem', self.root/'fixture-public.pem'
        builder.openssl('openssl', 'genpkey', '-algorithm', 'ED25519', '-out', self.key)
        builder.openssl('openssl', 'pkey', '-in', self.key, '-pubout', '-out', self.public)
        for name in builder.RAW_ASSETS:
            content = 'fixture '+name+'\n'
            if name == 'agent-version.txt':
                content = VERSION+'\n'
            elif name == 'source-sha.txt':
                content = SOURCE+'\n'
            elif name == 'release-manifest.json':
                content = json.dumps({'component': 'corade', 'version': VERSION,
                                      'source_repository': 'ClaraCora/CPanelde', 'source_commit': SOURCE})
            (self.release/name).write_text(content)
        self.sign_release()

    def sign_release(self):
        manifest = self.release/'release-manifest.sha256'
        manifest.write_text(''.join(builder.digest(self.release/name)+'  '+name+'\n' for name in sorted(builder.RAW_ASSETS)))
        builder.sign('openssl', self.key, self.public, manifest)

    def build(self):
        return builder.build(self.release, self.output, self.key, self.public, SOURCE, epoch=1700000000)

    def tearDown(self):
        self.temp.cleanup()

    def test_complete_signed_archive_is_reproducible_and_contains_no_private_material(self):
        first = self.build()
        second = self.build()
        self.assertEqual(first, second)
        archive = self.output/builder.ARCHIVE
        checksum = self.output/(builder.ARCHIVE+'.sha256')
        self.assertEqual(checksum.read_text(), hashlib.sha256(archive.read_bytes()).hexdigest()+'  '+archive.name+'\n')
        builder.openssl('openssl', 'pkeyutl', '-verify', '-pubin', '-inkey', self.public, '-rawin',
                        '-in', checksum, '-sigfile', str(checksum)+'.sig')
        with tarfile.open(archive) as tar:
            files = {m.name:m for m in tar.getmembers() if m.isfile()}
            expected = {'corade-offline/release/'+name for name in builder.SIGNED_ASSETS}
            expected |= {'corade-offline/'+name for name in ['upgrade.sh','rollback.sh','README.md','release-public-key.pem','SHA256SUMS','SHA256SUMS.sig']}
            self.assertEqual(set(files), expected)
            self.assertTrue(all(not m.issym() and not m.islnk() and m.uid == 0 for m in tar.getmembers()))
            for name in expected:
                self.assertNotIn('private', name)
            script = tar.extractfile(files['corade-offline/upgrade.sh']).read().decode()
            self.assertIn("VERSION='"+VERSION+"'", script)
            self.assertNotIn('@KEY_SHA@', script)
            # Verify the package's own signed inventory and every file without executing code.
            sums = tar.extractfile(files['corade-offline/SHA256SUMS']).read()
            signature = tar.extractfile(files['corade-offline/SHA256SUMS.sig']).read()
            (self.root/'sums').write_bytes(sums)
            (self.root/'signature').write_bytes(signature)
            builder.openssl('openssl', 'pkeyutl', '-verify', '-pubin', '-inkey', self.public, '-rawin',
                            '-in', self.root/'sums', '-sigfile', self.root/'signature')
            for line in sums.decode().splitlines():
                digest, name = line.split()
                self.assertEqual(hashlib.sha256(tar.extractfile(files['corade-offline/'+name]).read()).hexdigest(), digest)
            self.assertEqual(files['corade-offline/release/corade-linux-arm64'].mode, 0o755)
            self.assertEqual(files['corade-offline/upgrade.sh'].mode, 0o755)

    def test_tampered_binary_is_rejected(self):
        (self.release/'corade-linux-arm64').write_text('tampered')
        with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
            self.build()

    def test_unsigned_extra_file_is_rejected(self):
        (self.release/'production.env').write_text('fixture')
        with self.assertRaisesRegex(ValueError, 'Unexpected release asset set'):
            self.build()

    def test_wrong_source_is_rejected(self):
        with self.assertRaisesRegex(ValueError, 'source does not match'):
            builder.build(self.release, self.output, self.key, self.public, '0'*40)

    def test_wrong_public_key_is_rejected(self):
        other = self.root/'other.pem'
        builder.openssl('openssl', 'genpkey', '-algorithm', 'ED25519', '-out', other)
        with self.assertRaisesRegex(ValueError, 'trust anchor'):
            builder.build(self.release, self.output, other, self.public, SOURCE)

    def test_tampered_signature_is_rejected(self):
        (self.release/'release-manifest.sha256.sig').write_bytes(b'x'*64)
        with self.assertRaises(subprocess.CalledProcessError):
            self.build()


if __name__ == '__main__':
    unittest.main(verbosity=2)
