#!/usr/bin/env python3
"""Package an authenticated release using only explicit public assets."""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile

BASE = Path(__file__).resolve().parent
RAW_ASSETS = {
    f'{binary}-linux-{arch}{suffix}'
    for binary in ('corade', 'coradectl') for arch in ('amd64', 'arm64') for suffix in ('', '.sha256')
} | {'agent-version.txt', 'source-sha.txt', 'release-manifest.json', 'corade-install.sh'}
SIGNED_ASSETS = RAW_ASSETS | {'release-manifest.sha256', 'release-manifest.sha256.sig'}
ARCHIVE = 'corade-offline-linux.tar.gz'


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def openssl(executable, *args):
    return subprocess.run([executable, *map(str, args)], check=True, capture_output=True).stdout


def sign(executable, key, public, path):
    signature = path.with_name(path.name+'.sig')
    openssl(executable, 'pkeyutl', '-sign', '-inkey', key, '-rawin', '-in', path, '-out', signature)
    openssl(executable, 'pkeyutl', '-verify', '-pubin', '-inkey', public, '-rawin', '-in', path, '-sigfile', signature)


def build(release, output, key, public, expected_source, epoch=0, executable='openssl'):
    release, output, key, public = map(lambda p: Path(p).resolve(), (release, output, key, public))
    if key.is_relative_to(release) or key.is_relative_to(output):
        raise ValueError('Signing key must remain outside release and output directories')
    derived = openssl(executable, 'pkey', '-in', key, '-pubout').replace(b'\r\n', b'\n').strip()
    if derived != public.read_bytes().replace(b'\r\n', b'\n').strip():
        raise ValueError('Signing key does not match the release trust anchor')
    manifest = release/'release-manifest.sha256'
    openssl(executable, 'pkeyutl', '-verify', '-pubin', '-inkey', public, '-rawin',
            '-in', manifest, '-sigfile', release/'release-manifest.sha256.sig')
    entries = {}
    for line in manifest.read_text(encoding='ascii').splitlines():
        match = re.fullmatch(r'([0-9a-f]{64})  ([A-Za-z0-9][A-Za-z0-9._-]*)', line)
        if not match or match[2] in entries:
            raise ValueError('Invalid or duplicate signed release entry')
        entries[match[2]] = match[1]
    if set(entries) != RAW_ASSETS or {p.name for p in release.iterdir()} != SIGNED_ASSETS:
        raise ValueError('Unexpected release asset set; offline packages must not nest')
    for name in SIGNED_ASSETS:
        path = release/name
        if path.is_symlink() or not path.is_file():
            raise ValueError('Release assets must be regular files: '+name)
        if name in entries and digest(path) != entries[name]:
            raise ValueError('Release checksum mismatch: '+name)
    version = (release/'agent-version.txt').read_text().strip()
    source = (release/'source-sha.txt').read_text().strip()
    if not re.fullmatch(r'[0-9a-f]{40}', source) or source != expected_source:
        raise ValueError('Release source does not match the requested commit')
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+\+'+source[:12], version):
        raise ValueError('Release version does not match the source commit')
    metadata = json.loads((release/'release-manifest.json').read_text())
    if metadata != {'component': 'corade', 'version': version,
                    'source_repository': 'ClaraCora/CPanelde', 'source_commit': source}:
        raise ValueError('Unexpected signed release metadata')
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='corade-offline-') as temporary:
        root = Path(temporary)/'corade-offline'
        (root/'release').mkdir(parents=True)
        for name in sorted(SIGNED_ASSETS):
            shutil.copyfile(release/name, root/'release'/name)
        shutil.copyfile(public, root/'release-public-key.pem')
        values = {'@VERSION@': version, '@SOURCE@': source, '@KEY_SHA@': digest(public)}
        for template, target in [('upgrade.sh.in', 'upgrade.sh'), ('README.md.in', 'README.md'), ('rollback.sh', 'rollback.sh')]:
            content = (BASE/template).read_text(encoding='utf-8')
            for marker, value in values.items():
                content = content.replace(marker, value)
            if re.search(r'@[A-Z_]+@', content):
                raise ValueError('Unresolved package template')
            (root/target).write_text(content, encoding='utf-8', newline='\n')
        files = sorted(p for p in root.rglob('*') if p.is_file())
        sums = root/'SHA256SUMS'
        sums.write_text(''.join(digest(p)+'  '+p.relative_to(root).as_posix()+'\n' for p in files), encoding='ascii', newline='\n')
        sign(executable, key, public, sums)
        archive = output/ARCHIVE
        # Fixed ordering, timestamps, ownership and gzip header; no local paths or credentials.
        with archive.open('wb') as raw, gzip.GzipFile(fileobj=raw, filename='', mode='wb', mtime=epoch, compresslevel=6) as compressed:
            with tarfile.open(fileobj=compressed, mode='w', format=tarfile.PAX_FORMAT) as tar:
                for path in [root, *sorted(root.rglob('*'))]:
                    info = tar.gettarinfo(str(path), arcname=path.relative_to(root.parent).as_posix())
                    info.uid = info.gid = 0
                    info.uname = info.gname = 'root'
                    info.mtime = epoch
                    info.pax_headers = {}
                    binary = bool(re.fullmatch(r'corade(?:ctl)?-linux-(amd64|arm64)', path.name))
                    info.mode = 0o755 if path.is_dir() or binary or path.suffix == '.sh' else 0o644
                    if path.is_file():
                        with path.open('rb') as stream:
                            tar.addfile(info, stream)
                    else:
                        tar.addfile(info)
        checksum = output/(ARCHIVE+'.sha256')
        checksum.write_text(digest(archive)+'  '+ARCHIVE+'\n', encoding='ascii', newline='\n')
        sign(executable, key, public, checksum)
    return {'archive': ARCHIVE, 'sha256': digest(archive), 'version': version, 'source_commit': source, 'bytes': archive.stat().st_size}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--release-dir', required=True, type=Path)
    parser.add_argument('--output-dir', required=True, type=Path)
    parser.add_argument('--signing-key', required=True, type=Path)
    parser.add_argument('--public-key', type=Path, default=BASE.parents[1]/'internal/releaseverify/release-public-key.pem')
    parser.add_argument('--source', required=True)
    parser.add_argument('--epoch', type=int, default=0)
    args = parser.parse_args()
    print(json.dumps(build(args.release_dir, args.output_dir, args.signing_key, args.public_key, args.source, args.epoch), indent=2))
