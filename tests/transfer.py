#!/usr/bin/env python3
import argparse, hashlib, json, os, pathlib, runpy, shlex, subprocess, tempfile, unittest
from unittest.mock import patch
TOOL = pathlib.Path(__file__).resolve().parents[1] / 'tools/devlink'
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
  return subprocess.run([str(TOOL),'transfer',direction,str(src),str(dst),'--port','18622','--tunnels','1','--jobs','2','--chunk-size','4096','--state-dir',str(self.root/'state'),'--remote-state',str(self.root/'remote-state'),*opts],env=self.env,text=True,capture_output=True)
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
class Lanes(unittest.TestCase):
 def test_transport_retry_delay_never_grows(self):
  ns=runpy.run_path(str(TOOL));waits=[]
  class Stop:
   def is_set(self):return False
   def wait(self,seconds):waits.append(seconds);return False
  class Process:
   def __init__(self,code):self.returncode=code
   def __enter__(self):return self
   def __exit__(self,*args):return False
   def communicate(self,timeout=None):return (b'result',b'connection lost')
  args=argparse.Namespace(host='127.0.0.1',port=18622,user='root',identity=None,timeout=30,retries=0)
  with patch('subprocess.Popen',side_effect=[Process(255),Process(255),Process(255),Process(0)]):
   self.assertEqual(ns['SSH'](args,stop=Stop()).run('command'),b'result')
  self.assertEqual(waits,[2,2,2])
 def test_same_frontend_port_no_default_expiry_and_restore(self):
  ns=runpy.run_path(str(TOOL));calls=[]
  previous={'ssh_port':18622,'lanes_available':4,'lanes':1}
  class FakeSSH:
   def __init__(self,*a,**kw):self.ports=[]
   def run(self,command):
    calls.append(command)
    return json.dumps(previous) if command.endswith('status --json') else ''
  args=argparse.Namespace(tunnels=4,client='/devlink',lane_lease='',timeout=30,retries=0)
  ssh=FakeSSH()
  with patch.dict(ns['tunnel_lanes'].__wrapped__.__globals__,SSH=FakeSSH):
   with ns['tunnel_lanes'](args,ssh):self.assertEqual(ssh.ports,[18622]*4)
  self.assertEqual(calls,['/devlink status --json','/devlink lanes 4','/devlink lanes 1'])
 def test_shrink_eof_is_confirmed_before_warning(self):
  ns=runpy.run_path(str(TOOL));calls=[];messages=[]
  class FakeSSH:
   def __init__(self,*a,**kw):self.ports=[]
   def run(self,command):
    calls.append(command)
    if command.endswith('lanes 1'):raise RuntimeError('closed its own lane')
    return json.dumps({'ssh_port':18622,'lanes_available':4,'lanes':1}) if command.endswith('status --json') else ''
  args=argparse.Namespace(tunnels=4,client='/devlink',lane_lease='',timeout=30,retries=0)
  with patch.dict(ns['tunnel_lanes'].__wrapped__.__globals__,SSH=FakeSSH,say=messages.append):
   with ns['tunnel_lanes'](args,FakeSSH()):pass
  self.assertEqual(calls[-2:],['/devlink lanes 1','/devlink status --json']);self.assertEqual(messages,[])
 def test_explicit_lease_is_one_shell_argument(self):
  ns=runpy.run_path(str(TOOL));calls=[]
  class FakeSSH:
   def __init__(self,*a,**kw):self.ports=[]
   def run(self,command):
    calls.append(command)
    return json.dumps({'ssh_port':18622,'lanes_available':4,'lanes':1}) if command.endswith('status --json') else ''
  args=argparse.Namespace(tunnels=4,client='/devlink',lane_lease='15m',timeout=30,retries=0)
  with patch.dict(ns['tunnel_lanes'].__wrapped__.__globals__,SSH=FakeSSH):
   with ns['tunnel_lanes'](args,FakeSSH()):pass
  self.assertEqual(shlex.split(calls[1]),['/devlink','lanes','4','15m'])
 def test_short_explicit_lease_renews_before_expiry(self):
  ns=runpy.run_path(str(TOOL))
  self.assertEqual(ns['lease_interval']('45s'),15)
  self.assertEqual(ns['lease_interval']('1h30m'),120)
  self.assertAlmostEqual(ns['lease_interval']('.5m'),10)
  for value in ('0s', '-1m', '15m; evil', '1m garbage', ''):
   if value:
    with self.assertRaises(ValueError):ns['lease_interval'](value)
   else:self.assertEqual(ns['lease_interval'](value),120)
if __name__=='__main__':unittest.main()
