#!/usr/bin/env python3
import contextlib
import argparse
import http.server
import io
import json
import os
from pathlib import Path
import runpy
import shlex
import subprocess
import threading
import unittest
import urllib.error
from unittest.mock import patch

TOOL = Path(__file__).resolve().parents[1] / 'tools/devlink'
NS = runpy.run_path(str(TOOL))
DEVICE = dict(id='0123456789abcdef', label='phone', connected=True, revoked=False,
              active_lanes=4, ssh_host='127.0.0.1', ssh_port=18622, ssh_user='root',
              adb_host='127.0.0.1', adb_port=18623, adb_forwarding=True)

class CLI(unittest.TestCase):
    def select(self, selector, records, **kwargs):
        with patch.dict(NS['select_device'].__globals__, discover=lambda _: records):
            return NS['select_device'](selector, **kwargs)

    def test_exact_prefix_and_label(self):
        for selector in ('0123456789abcdef', '0123', 'phone'):
            self.assertEqual(self.select(selector, [DEVICE]), DEVICE)

    def test_ambiguous_missing_revoked_offline_and_invalid_ports(self):
        other = dict(DEVICE, id='fedcba9876543210')
        for selector, records, expected in [
            ('phone', [DEVICE, other], 'ambiguous'),
            ('missing', [DEVICE], 'not found'),
            ('phone', [dict(DEVICE, revoked=True)], 'revoked'),
            ('phone', [dict(DEVICE, connected=False)], 'offline'),
            ('phone', [dict(DEVICE, ssh_port=0)], 'Invalid'),
            ('phone', [dict(DEVICE, ssh_host='-malicious')], 'Unexpected'),
        ]:
            with self.subTest(expected=expected), self.assertRaisesRegex(RuntimeError, expected):
                self.select(selector, records, require_online=True)
        # Exact ID wins even if a label elsewhere happens to equal that ID.
        self.assertEqual(self.select(DEVICE['id'], [DEVICE, dict(other, label=DEVICE['id'])]), DEVICE)

    def test_discovery_http_and_filter_json(self):
        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(200); self.end_headers()
                self.wfile.write(json.dumps([DEVICE, dict(DEVICE, id='offline', connected=False)]).encode())
            def log_message(self, *args): pass
        server = http.server.HTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever); thread.start()
        try:
            endpoint = f'http://127.0.0.1:{server.server_port}/devices'
            result = subprocess.run([str(TOOL), 'devices', '--json', '--connected', '--endpoint', endpoint], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout), [DEVICE])
        finally:
            server.shutdown(); thread.join(); server.server_close()

    def test_ssh_exec_quotes_and_does_not_replay(self):
        globals_ = NS['device_command'].__globals__
        with patch.dict(globals_, select_device=lambda *a, **k: DEVICE), patch('os.execvp') as execute:
            NS['device_command']('ssh', ['--identity', '/keys/a b', 'phone', '--', 'echo', 'a; b'])
        name, argv = execute.call_args.args
        self.assertEqual(name, 'ssh')
        self.assertIn('ControlMaster=no', argv)
        self.assertIn('/keys/a b', argv)
        self.assertEqual(shlex.split(argv[-1]), ['echo', 'a; b'])
        self.assertEqual(execute.call_count, 1)

    def test_adb_disabled_and_serial_selection(self):
        globals_ = NS['device_command'].__globals__
        with patch.dict(globals_, select_device=lambda *a, **k: dict(DEVICE, adb_forwarding=False)):
            with self.assertRaisesRegex(RuntimeError, 'forwarding is off'):
                NS['device_command']('adb', ['phone'])
        with patch.dict(globals_, select_device=lambda *a, **k: DEVICE), patch('subprocess.run', return_value=subprocess.CompletedProcess([], 0)) as run, patch('os.execvp') as execute:
            NS['device_command']('adb', ['phone', '--', 'shell', 'id'])
        self.assertEqual(run.call_args.args[0], ['adb', 'connect', '127.0.0.1:18623'])
        self.assertEqual(execute.call_args.args, ('adb', ['adb', '-s', '127.0.0.1:18623', 'shell', 'id']))

    def test_transfer_selects_device_without_a_manual_port(self):
        captured = []
        globals_ = NS['transfer_main'].__globals__
        with patch.dict(globals_, select_device=lambda *a, **k: DEVICE,
                        SSH=lambda args: None,
                        tunnel_lanes=lambda *a: contextlib.nullcontext(),
                        transfer=lambda args, ssh: captured.append(args)):
            result = NS['transfer_main'](['push', './file', '/sdcard/file', '--device', 'phone'])
        self.assertEqual(result, 0)
        self.assertEqual((captured[0].host, captured[0].port, captured[0].user), ('127.0.0.1', 18622, 'root'))

    def test_transfer_retries_discovery_at_fixed_delay_and_accepts_offline(self):
        waits = []
        class Stop:
            def is_set(self): return False
            def wait(self, seconds): waits.append(seconds); return False
        globals_ = NS['transfer_device'].__globals__
        args = argparse.Namespace(device='phone', endpoint='http://127.0.0.1:18792/devices', retries=0)
        with patch.dict(globals_, say=lambda _: None), patch.dict(globals_, select_device=unittest.mock.Mock(side_effect=[urllib.error.URLError('connection refused'), TimeoutError('timeout'), dict(DEVICE, connected=False)])):
            result = NS['transfer_device'](args, Stop())
            self.assertFalse(result['connected'])
            self.assertEqual(globals_['select_device'].call_args.kwargs, {'require_online': False})
        self.assertEqual(waits, [2, 2])
        with patch.dict(globals_, select_device=unittest.mock.Mock(side_effect=RuntimeError('revoked'))):
            with self.assertRaisesRegex(RuntimeError, 'revoked'):
                NS['transfer_device'](args, Stop())
        with patch.dict(globals_, select_device=unittest.mock.Mock(side_effect=urllib.error.HTTPError(args.endpoint, 404, 'not found', {}, None))):
            with self.assertRaises(urllib.error.HTTPError):
                NS['transfer_device'](args, Stop())

    def test_help_and_legacy_aliases(self):
        for argv in (['--help'], ['devices', '--help'], ['transfer', '--help'], ['ssh', '--help']):
            result = subprocess.run([str(TOOL), *argv], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
        for tool in ('devlink-devices', 'devlink-transfer'):
            result = subprocess.run([str(TOOL.with_name(tool)), '--help'], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)

if __name__ == '__main__': unittest.main()
