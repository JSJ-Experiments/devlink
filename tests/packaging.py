#!/usr/bin/env python3
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]

class Packaging(unittest.TestCase):
    def test_self_contained_origin_bundles_without_private_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            tmp = Path(temporary); binary = tmp/'bin'; binary.mkdir()
            go = binary/'go'
            go.write_text('''#!/bin/sh
while [ "$#" -gt 0 ]; do
 if [ "$1" = -o ]; then shift; printf '#!/bin/sh\\necho fake-test-binary\\n' > "$1"; chmod 755 "$1"; exit 0; fi
 shift
done
exit 1
'''); go.chmod(0o755)
            out = tmp/'output'
            env = dict(os.environ, OUT=str(out), PATH=str(binary)+':'+os.environ['PATH'])
            subprocess.run([str(ROOT/'build/package-origin.sh')], env=env, check=True, capture_output=True)
            for arch in ('amd64', 'arm64'):
                with tarfile.open(out/f'devlink-origin-linux-{arch}.tar.gz') as bundle:
                    prefix = f'devlink-origin-linux-{arch}/'
                    files = {m.name.removeprefix(prefix) for m in bundle.getmembers() if m.isfile()}
                    self.assertEqual(files, {'devlink', 'devlink-server', 'README.md', 'LICENSE',
                                            'deploy/install-server.sh', 'deploy/devlink-server.service',
                                            'deploy/Caddyfile.fragment', 'deploy/public.json'})
                    for executable in ('devlink', 'devlink-server', 'deploy/install-server.sh'):
                        self.assertTrue(bundle.getmember(prefix+executable).mode & 0o111)
            self.assertEqual(len(list(out.iterdir())), 2)

    def test_release_publishes_only_simple_asset_directory(self):
        workflow = (ROOT/'.github/workflows/build.yml').read_text()
        self.assertIn('dist/release/* --clobber', workflow)
        self.assertNotIn('gh release upload latest dist/*', workflow)
        self.assertIn('gh release delete-asset latest "$asset" --yes', workflow)
        self.assertIn('devlink.zip|devlink-origin-linux-amd64.tar.gz|devlink-origin-linux-arm64.tar.gz|checksums.txt|update.json', workflow)

if __name__ == '__main__': unittest.main()
