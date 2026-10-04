#!/usr/bin/env python3
"""Exercise the real promotion script against an isolated fake Android layout."""
import os, pathlib, subprocess, tempfile, unittest
SCRIPT = pathlib.Path(__file__).resolve().parents[1] / 'module/scripts/hotreload.sh'
class HotReload(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.root=pathlib.Path(self.tmp.name);self.active=self.root/'modules/devlink';self.source=self.root/'modules_update/devlink';self.state=self.root/'devlink';self.state.mkdir();self.statefile=self.state/'state.json';self.statefile.write_text('{"enabled":true,"until":1234567,"secret":"preserve"}')
  self.bin=self.root/'mock-bin';self.bin.mkdir();self.executable(self.bin/'id','#!/bin/sh\necho 0\n')
  for folder,version in [(self.active,'old'),(self.source,'new')]:
   (folder/'bin').mkdir(parents=True);(folder/'module.prop').write_text('id=devlink\nversion='+version+'\n')
   self.executable(folder/'bin/devlink','#!/bin/sh\n[ "$1" = stop-runtime ] || exit 2\necho stopped > "$DEVLINK_STATE/stopped"\n')
   self.executable(folder/'service.sh',f'#!/bin/sh\necho {version} > "$DEVLINK_STATE/activated"\n')
  (self.source/'update').touch();other=self.root/'modules/box_for_root';other.mkdir();(other/'sentinel').write_text('untouched')
  self.env=dict(os.environ,DEVLINK_ADB=str(self.root),DEVLINK_STATE=str(self.state),DEVLINK_SHELL='/bin/sh',PATH=str(self.bin)+':'+os.environ['PATH'])
 def tearDown(self):self.tmp.cleanup()
 def executable(self,path,body):path.write_text(body);path.chmod(0o755)
 def run_script(self,src=None):return subprocess.run(['sh',str(SCRIPT),str(src or self.source),'0'],env=self.env,text=True,capture_output=True)
 def test_promotes_and_preserves_state(self):
  before=self.statefile.read_text();r=self.run_script();self.assertEqual(r.returncode,0,r.stderr);self.assertIn('version=new',(self.active/'module.prop').read_text());self.assertFalse(self.source.exists());self.assertFalse((self.active/'update').exists());self.assertEqual(self.statefile.read_text(),before);self.assertEqual((self.state/'activated').read_text().strip(),'new');self.assertEqual((self.root/'modules/box_for_root/sentinel').read_text(),'untouched');self.assertFalse((self.state/'hotreload.lock').exists())
 def test_disabled_stays_disabled(self):
  (self.active/'disable').touch();r=self.run_script();self.assertEqual(r.returncode,0,r.stderr);self.assertTrue((self.active/'disable').exists());self.assertFalse((self.state/'activated').exists())
 def test_failed_activation_rolls_back(self):
  self.executable(self.source/'service.sh','#!/bin/sh\nexit 1\n');r=self.run_script();self.assertEqual(r.returncode,1);self.assertIn('version=old',(self.active/'module.prop').read_text());self.assertEqual((self.state/'activated').read_text().strip(),'old')
 def test_refuses_system_mount_and_wrong_module(self):
  (self.source/'system').mkdir();r=self.run_script();self.assertEqual(r.returncode,1);self.assertTrue(self.source.exists());self.assertFalse((self.state/'stopped').exists());(self.source/'system').rmdir();(self.source/'module.prop').write_text('id=unrelated\n');r=self.run_script();self.assertEqual(r.returncode,1)
 def test_reloads_active_module(self):
  r=self.run_script(self.active);self.assertEqual(r.returncode,0,r.stderr);self.assertIn('version=old',(self.active/'module.prop').read_text());self.assertEqual((self.state/'activated').read_text().strip(),'old')
if __name__=='__main__':unittest.main()
