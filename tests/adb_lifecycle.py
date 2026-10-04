#!/usr/bin/env python3
"""Run the real ADB helper with mocked Android commands; never touches adbd."""
import os, pathlib, subprocess, tempfile, unittest
HELPER = pathlib.Path(__file__).resolve().parents[1] / 'module/scripts/adb.sh'
MOCK = '''#!/usr/bin/env python3
import os,sys,pathlib
r=pathlib.Path(os.environ['MOCK_ROOT']);name=pathlib.Path(sys.argv[0]).name;a=sys.argv[1:]
with (r/'calls').open('a') as f:f.write(name+' '+ ' '.join(a)+'\\n')
if name=='getprop':
 p=r/('prop-'+a[0]);print(p.read_text().strip() if p.exists() else '')
elif name=='setprop':(r/('prop-'+a[0])).write_text(a[1] if len(a)>1 else '')
elif name=='ss':
 if (r/'borrow').exists(): print('LISTEN 0 128 127.0.0.1:5555 0.0.0.0:* users:(("adbd",pid=10,fd=1))')
 elif (r/'prop-service.adb.listen_addrs').exists() and '15556' in (r/'prop-service.adb.listen_addrs').read_text():print('LISTEN 0 128 127.0.0.1:15556 0.0.0.0:* users:(("adbd",pid=10,fd=1))')
elif name in ('iptables','ip6tables') and '-C' in a:sys.exit(1)
'''
class Adb(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.root=pathlib.Path(self.tmp.name);self.bin=self.root/'bin';self.bin.mkdir();self.state=self.root/'state';self.state.mkdir()
  for name in ['getprop','setprop','ss','iptables','ip6tables','stop','start']:
   p=self.bin/name;p.write_text(MOCK);p.chmod(0o755)
  self.env=dict(os.environ,DEVLINK_STATE=str(self.state),MOCK_ROOT=str(self.root),PATH=str(self.bin)+':'+os.environ['PATH'])
 def tearDown(self):self.tmp.cleanup()
 def run_helper(self,verb):return subprocess.run(['sh',str(HELPER),verb],env=self.env,text=True,capture_output=True,check=True).stdout.strip()
 def calls(self):return (self.root/'calls').read_text()
 def prop(self,v):(self.root/'prop-service.adb.listen_addrs').write_text(v)
 def test_borrow_box_untouched(self):
  (self.root/'borrow').touch();self.assertEqual(self.run_helper('on'),'5555');self.run_helper('off');self.assertNotIn('setprop',self.calls());self.assertNotIn('stop adbd',self.calls());self.assertNotIn('-I INPUT',self.calls())
 def test_owned_preserves_and_restores_listeners(self):
  self.prop('tcp:localhost:7777');self.assertEqual(self.run_helper('on'),'15556');self.assertEqual((self.root/'prop-service.adb.listen_addrs').read_text(),'tcp:localhost:7777,tcp:localhost:15556');self.run_helper('off');self.assertEqual((self.root/'prop-service.adb.listen_addrs').read_text(),'tcp:localhost:7777');self.assertIn('iptables -I INPUT',self.calls());self.assertIn('ip6tables -I INPUT',self.calls());self.assertNotIn('persist.',self.calls());self.assertNotIn('setprop service.adb.tcp.port',self.calls())
 def test_other_owner_change_not_overwritten(self):
  self.prop('');self.run_helper('on');self.prop('tcp:localhost:5555');before=self.calls().count('stop adbd');self.run_helper('off');self.assertEqual((self.root/'prop-service.adb.listen_addrs').read_text(),'tcp:localhost:5555');self.assertEqual(self.calls().count('stop adbd'),before)
 def test_stale_boot_does_not_restore_properties(self):
  self.prop('');self.run_helper('on');(self.state/'adb/boot').write_text('previous-boot');self.prop('tcp:localhost:7777');self.run_helper('off');self.assertEqual((self.root/'prop-service.adb.listen_addrs').read_text(),'tcp:localhost:7777')
if __name__=='__main__':unittest.main()
