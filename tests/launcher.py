#!/usr/bin/env python3
"""Launcher regression checks against isolated fake cgroups, never host groups."""
import pathlib,subprocess,tempfile,unittest
ROOT=pathlib.Path(__file__).resolve().parents[1]
class Launcher(unittest.TestCase):
 def test_moves_own_pid_before_exec_and_preserves_arguments(self):
  with tempfile.TemporaryDirectory(prefix='devlink launcher ') as d:
   root=pathlib.Path(d);script=(ROOT/'module/scripts/devlink-launcher.sh').read_text();groups=[]
   for i,path in enumerate(['/sys/fs/cgroup/cgroup.procs','/sys/fs/cgroup/freezer/cgroup.procs','/dev/freezer/cgroup.procs']):
    target=root/f'group-{i}';target.write_text('');groups.append(target);script=script.replace(path,str(target))
   launcher=root/'devlink';launcher.write_text(script);native=root/'devlink.native';native.write_text('#!/bin/sh\nprintf "%s\\n" "$$" "$@"\nexit 7\n');native.chmod(0o700)
   p=subprocess.Popen(['sh',str(launcher),'endpoint','https://host/custom path',"quoted ' arg"],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True);out,err=p.communicate(timeout=5)
   self.assertEqual(p.returncode,7,err);self.assertEqual(out.splitlines(),[str(p.pid),'endpoint','https://host/custom path',"quoted ' arg"])
   for group in groups:self.assertEqual(group.read_text().strip(),str(p.pid))
 def test_absent_groups_are_safe(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);script=(ROOT/'module/scripts/devlink-launcher.sh').read_text().replace('/sys/fs/cgroup/',str(root/'missing')+'/').replace('/dev/freezer/',str(root/'missing2')+'/');launcher=root/'devlink';launcher.write_text(script);native=root/'devlink.native';native.write_text('#!/bin/sh\necho success\n');native.chmod(0o700)
   r=subprocess.run(['sh',str(launcher),'status'],capture_output=True,text=True);self.assertEqual(r.returncode,0,r.stderr);self.assertEqual(r.stdout.strip(),'success')
if __name__=='__main__':unittest.main()
