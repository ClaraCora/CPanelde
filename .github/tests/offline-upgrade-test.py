"""Isolated Linux fixtures; no real service/config paths are used."""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

VERSION='v2.0.10+10feddbd7e66'
OLD='v2.0.9+79fb7be01ae5'
SOURCE='10feddbd7e66bde17aecea8e3589f9f80953ee39'
SCRIPT=(Path(__file__).resolve().parents[2]/'packaging/offline/upgrade.sh.in').read_text().replace('@VERSION@',VERSION).replace('@SOURCE@',SOURCE)

class OfflineTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory(prefix='corade-offline-test-')
        self.root=Path(self.temp.name)
        self.package=self.root/'package'; self.release=self.package/'release'
        self.bin=self.root/'installed'; self.conf=self.root/'config'; self.mock=self.root/'mock'
        for p in [self.release,self.bin,self.conf,self.mock,self.root/'systemd']:
            p.mkdir(parents=True)
        (self.conf/'config.yml').write_text('health_port: 65530\ncontrol:\n  mode: device-platform\n')
        (self.conf/'agent-v2-identity.json').write_text('{"fixture":"preserve"}\n')
        (self.root/'service').write_text('fixture service\n')
        (self.root/'openrc').write_text('fixture OpenRC service\n')
        (self.root/'state').write_text('active')
        self.write_bin(self.bin/'corade',OLD)
        self.write_bin(self.bin/'coradectl',OLD,cli=True)
        for arch in ['amd64','arm64']:
            self.write_bin(self.release/('corade-linux-'+arch),VERSION)
            self.write_bin(self.release/('coradectl-linux-'+arch),VERSION,cli=True)
        (self.release/'agent-version.txt').write_text(VERSION+'\n')
        (self.release/'source-sha.txt').write_text(SOURCE+'\n')
        manifest=''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(self.release.iterdir()))
        (self.release/'release-manifest.sha256').write_text(manifest)
        self.openssl('genpkey','-algorithm','ED25519','-out',str(self.root/'private.pem'))
        self.openssl('pkey','-in',str(self.root/'private.pem'),'-pubout','-out',str(self.package/'release-public-key.pem'))
        self.openssl('pkeyutl','-sign','-inkey',str(self.root/'private.pem'),'-rawin','-in',str(self.release/'release-manifest.sha256'),'-out',str(self.release/'release-manifest.sha256.sig'))
        script=SCRIPT
        replacements={"BIN_DIR='/usr/local/bin'":f"BIN_DIR='{self.bin}'",
            "CONFIG_DIR='/etc/corade'":f"CONFIG_DIR='{self.conf}'",
            "SYSTEMD_FILE='/etc/systemd/system/corade.service'":f"SYSTEMD_FILE='{self.root/'service'}'",
            "OPENRC_FILE='/etc/init.d/corade'":f"OPENRC_FILE='{self.root/'openrc'}'",
            "SYSTEMD_MARKER='/run/systemd/system'":f"SYSTEMD_MARKER='{self.root/'systemd'}'",
            "BACKUP_ROOT='/var/backups/corade'":f"BACKUP_ROOT='{self.root/'backups'}'",
            "LOCK_DIR='/run/lock/corade-offline-upgrade.lock'":f"LOCK_DIR='{self.root/'lock'}'",
            "KEY_SHA='@KEY_SHA@'":f"KEY_SHA='{hashlib.sha256((self.package/'release-public-key.pem').read_bytes()).hexdigest()}'"}
        for old,new in replacements.items():
            assert old in script
            script=script.replace(old,new)
        (self.package/'upgrade.sh').write_text(script)
        (self.package/'README.md').write_text('fixture instructions\n')
        sums=''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.relative_to(self.package).as_posix()+'\n' for p in sorted(self.package.rglob('*')) if p.is_file())
        (self.package/'SHA256SUMS').write_text(sums)
        self.openssl('pkeyutl','-sign','-inkey',str(self.root/'private.pem'),'-rawin','-in',str(self.package/'SHA256SUMS'),'-out',str(self.package/'SHA256SUMS.sig'))
        self.env=dict(os.environ,PATH=str(self.mock)+':'+os.environ['PATH'],FIXTURE=str(self.root))
        self.mock_bin('uname','[ "$1" = -s ] && { echo Linux; exit; }; echo "${TEST_ARCH:-x86_64}"')
        self.mock_bin('id','echo 0')
        self.mock_bin('df','printf "Filesystem 1024-blocks Used Available Capacity Mounted\nfixture 9999999 0 9999999 0%% /\n"')
        self.mock_bin('sleep','exit 0')
        self.mock_bin('curl','''
if [ "${FAIL_NEW_HEALTH:-0}" = 1 ] && grep -q 'v2.0.10' "$FIXTURE/installed/corade"; then exit 1; fi
[ "$(cat "$FIXTURE/state")" = active ]
''')
        manager='''
printf '%s\n' "$*" >>"$FIXTURE/commands"
case "$*" in
  *ExecStart*) printf 'path=%s/installed/corade ;\n' "$FIXTURE" ;;
  *is-active*|*status*) [ "$(cat "$FIXTURE/state")" = active ] ;;
  *stop*) echo inactive >"$FIXTURE/state" ;;
  *start*)
    if [ "${FAIL_NEW_START:-0}" = 1 ] && grep -q 'v2.0.10' "$FIXTURE/installed/corade"; then exit 1; fi
    echo active >"$FIXTURE/state" ;;
  *) exit 1 ;;
esac
'''
        self.mock_bin('systemctl',manager)
        self.mock_bin('rc-service',manager)
        self.mock_bin('mv','''
case "$*" in
  *"/coradectl "*)
    if [ "${FAIL_CLI_SWAP:-0}" = 1 ] && [ ! -e "$FIXTURE/swap-failed" ]; then touch "$FIXTURE/swap-failed"; exit 1; fi ;;
esac
exec /usr/bin/mv "$@"
''')

    def openssl(self,*args):
        subprocess.run(['openssl',*args],check=True,capture_output=True)

    def write_bin(self,path,version,cli=False):
        body='#!/bin/sh\n'
        if cli: body+='if [ "${1:-}" = config ]; then echo 65530; exit 0; fi\n'
        body+=f"echo '{version}'\n"
        path.write_text(body);path.chmod(0o755)

    def mock_bin(self,name,body):
        p=self.mock/name;p.write_text('#!/bin/sh\nset -eu\n'+body+'\n');p.chmod(0o755)

    def run_script(self,*args,ok=True):
        r=subprocess.run(['/bin/sh',str(self.package/'upgrade.sh'),*args],env=self.env,text=True,capture_output=True)
        self.assertEqual(r.returncode==0,ok,r.stdout+r.stderr)
        return r

    def assert_old(self):
        self.assertIn(OLD,(self.bin/'corade').read_text())
        self.assertIn(OLD,(self.bin/'coradectl').read_text())
        self.assertEqual((self.root/'state').read_text().strip(),'active')

    def tearDown(self): self.temp.cleanup()

    def test_check_does_not_modify_installation(self):
        self.run_script('--check'); self.assert_old()
        self.assertFalse((self.root/'backups').exists())

    def test_modified_binary_is_rejected(self):
        (self.release/'corade-linux-amd64').write_text('tampered')
        self.run_script('--apply',ok=False);self.assert_old()

    def test_modified_signature_is_rejected(self):
        (self.release/'release-manifest.sha256.sig').write_bytes(b'x'*64)
        self.run_script('--apply',ok=False);self.assert_old()

    def test_modified_instructions_are_rejected(self):
        (self.package/'README.md').write_text('tampered')
        self.run_script('--apply',ok=False);self.assert_old()

    def test_modified_package_signature_is_rejected(self):
        (self.package/'SHA256SUMS.sig').write_bytes(b'x'*64)
        self.run_script('--apply',ok=False);self.assert_old()

    def test_missing_configuration_is_rejected(self):
        (self.conf/'config.yml').unlink()
        self.run_script('--apply',ok=False);self.assert_old()

    def test_newer_version_is_not_downgraded(self):
        for version in ['v2.0.11+fixture','v2.1.0+fixture','v3.0.0+fixture']:
            with self.subTest(version=version):
                self.write_bin(self.bin/'corade',version)
                self.run_script('--apply',ok=False)
                self.assertIn(version,(self.bin/'corade').read_text())

    def test_systemd_success_and_manual_rollback(self):
        original={p.name:p.read_bytes() for p in self.conf.iterdir()}
        self.run_script('--apply')
        self.assertIn(VERSION,(self.bin/'corade').read_text())
        self.assertIn(VERSION,(self.bin/'coradectl').read_text())
        backup=next((self.root/'backups').glob('offline-*'))
        self.assertTrue((backup/'configuration.tar.gz').is_file())
        self.run_script('--rollback',str(backup)); self.assert_old()
        self.assertEqual(original,{p.name:p.read_bytes() for p in self.conf.iterdir()})

    def test_restart_failure_restores_both_binaries(self):
        self.env['FAIL_NEW_START']='1'
        self.run_script('--apply',ok=False); self.assert_old()

    def test_health_failure_restores_both_binaries(self):
        self.env['FAIL_NEW_HEALTH']='1'
        self.run_script('--apply',ok=False); self.assert_old()

    def test_existing_lock_does_not_change_running_agent(self):
        (self.root/'lock').mkdir()
        self.run_script('--apply',ok=False); self.assert_old()
        self.assertTrue((self.root/'lock').exists())

    def test_partial_swap_restores_both_binaries(self):
        self.env['FAIL_CLI_SWAP']='1'
        self.run_script('--apply',ok=False); self.assert_old()

    def test_same_exact_version_skips_restart(self):
        shutil.copy2(self.release/'corade-linux-amd64',self.bin/'corade')
        shutil.copy2(self.release/'coradectl-linux-amd64',self.bin/'coradectl')
        self.run_script('--apply')
        commands=(self.root/'commands').read_text()
        self.assertNotIn('start corade.service',commands)
        self.assertFalse((self.root/'backups').exists())

    def test_openrc_and_arm64_selection(self):
        (self.root/'systemd').rmdir();self.env['TEST_ARCH']='aarch64'
        self.run_script('--apply')
        self.assertIn('corade start',(self.root/'commands').read_text())
        self.assertIn(VERSION,(self.bin/'corade').read_text())

if __name__=='__main__': unittest.main(verbosity=2)
