#!/usr/bin/env python3
import hashlib, json, os, pathlib, subprocess, tempfile, unittest
TOOL = pathlib.Path(__file__).resolve().parents[1] / 'tools/devlink-transfer'
MOCK = '''#!/usr/bin/env python3
import os,sys,subprocess,pathlib,time
r=pathlib.Path(os.environ['MOCK_ROOT']);cmd=sys.argv[-1]
if 'cat >' in cmd and (r/'fail').exists():
 if '000000000001.chunk' in cmd:
  (r/'fail').unlink();sys.stderr.write('Connection reset by peer');sys.exit(255)
if os.environ.get('MOCK_SLOW_STDIN') and 'cat >' in cmd:
 data=b''
 while True:
  b=sys.stdin.buffer.read(4096)
  if not b:break
  data+=b;time.sleep(0.05)
 p=subprocess.run(['/bin/sh','-c',cmd],input=data)
else:p=subprocess.run(['/bin/sh','-c',cmd])
sys.exit(p.returncode)
'''
class Transfer(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.root=pathlib.Path(self.tmp.name);self.bin=self.root/'bin';self.bin.mkdir();p=self.bin/'ssh';p.write_text(MOCK);p.chmod(0o755)
  self.env=dict(os.environ,MOCK_ROOT=str(self.root),PATH=str(self.bin)+':'+os.environ['PATH'])
  self.src=self.root/"source ' quoted.bin";self.remote=self.root/"remote ' quoted.bin";self.dest=self.root/'result.bin';self.src.write_bytes(os.urandom(12001))
 def tearDown(self):self.tmp.cleanup()
 def run_tool(self,direction,src,dst,*opts):
  return subprocess.run([str(TOOL),direction,str(src),str(dst),'--port','18622','--tunnels','1','--jobs','2','--chunk-size','4096','--state-dir',str(self.root/'state'),'--remote-state',str(self.root/'remote-state'),*opts],env=self.env,text=True,capture_output=True)
 def test_push_pull_and_idempotence(self):
  r=self.run_tool('push',self.src,self.remote);self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(self.src.read_bytes(),self.remote.read_bytes())
  r=self.run_tool('push',self.src,self.remote);self.assertIn('already matches',r.stderr)
  r=self.run_tool('pull',self.remote,self.dest);self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(self.src.read_bytes(),self.dest.read_bytes())
 def test_disconnect_resume(self):
  (self.root/'fail').touch();r=self.run_tool('push',self.src,self.remote,'--retries','1');self.assertEqual(r.returncode,1,r.stderr);self.assertFalse(self.remote.exists())
  r=self.run_tool('push',self.src,self.remote);self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(self.src.read_bytes(),self.remote.read_bytes())
 def test_auto_retry(self):
  (self.root/'fail').touch();r=self.run_tool('push',self.src,self.remote,'--retries','5');self.assertEqual(r.returncode,0,r.stderr);self.assertIn('pausing',r.stderr)
 def test_corrupt_resume_chunk_is_not_trusted(self):
  self.remote.write_bytes(self.src.read_bytes());r=self.run_tool('pull',self.remote,self.dest,'--keep-chunks');self.assertEqual(r.returncode,0,r.stderr);self.dest.unlink()
  chunk=next((self.root/'state').glob('*/*.chunk'));chunk.write_bytes(b'bad')
  r=self.run_tool('pull',self.remote,self.dest);self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(self.src.read_bytes(),self.dest.read_bytes())
 def test_slow_stdin_past_communicate_timeout(self):
  self.env['MOCK_SLOW_STDIN']='1';self.src.write_bytes(os.urandom(131073))
  r=self.run_tool('push',self.src,self.remote,'--chunk-size','131072','--timeout','10');self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(self.src.read_bytes(),self.remote.read_bytes())
 def test_empty_and_no_clobber(self):
  self.src.write_bytes(b'');r=self.run_tool('push',self.src,self.remote);self.assertEqual(r.returncode,0,r.stderr);r=self.run_tool('pull',self.remote,self.dest);self.assertEqual(r.returncode,0,r.stderr)
  self.src.write_bytes(b'new');r=self.run_tool('push',self.src,self.remote);self.assertEqual(r.returncode,1);self.assertEqual(self.remote.read_bytes(),b'')
  r=self.run_tool('push',self.src,self.remote,'--overwrite');self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(self.remote.read_bytes(),b'new')
if __name__=='__main__':unittest.main()
