import { exec } from 'kernelsu';
const cli = '/data/adb/modules/devlink/bin/devlink';
const $ = id => document.getElementById(id);
// All user-provided values are single-quoted before crossing the shell bridge.
const quote = value => "'" + String(value).replaceAll("'", "'\\''") + "'";
let busy = false;
async function run(args) {
  const r = await exec(cli + ' ' + args.map(quote).join(' '));
  if (r.errno !== 0) throw new Error(r.stderr || r.stdout || `Command failed (${r.errno})`);
  return r.stdout;
}
async function refresh() {
  try {
    const s = JSON.parse(await run(['status', '--json']));
    $('state').textContent = !s.enabled ? 'Off · no background process' : s.connected ? 'Connected' : s.running ? 'Connecting / reconnecting' : 'Enabled · stopped (tap Enable to retry)';
    $('adb-state').textContent = s.adb ? 'ADB requested' : 'ADB off';
    $('details').textContent = [s.id && `Device: ${s.id}`, s.ssh_port && `SSH: root@127.0.0.1:${s.ssh_port} (on origin)`, s.adb_port && `ADB: 127.0.0.1:${s.adb_port} (on origin)`, s.until && `Turns off: ${new Date(s.until * 1000).toLocaleString()}`, s.ssh_ports && `SSH lane ports: ${s.ssh_ports.join(", ")} · active ${s.lanes}`, s.error && `Last error: ${s.error}`].filter(Boolean).join('\n');
    if (document.activeElement !== $('endpoint')) $('endpoint').value = s.endpoint;
    if (document.activeElement !== $('tls-name')) $('tls-name').value = s.tls_name || '';
    $('http').checked = !!s.allow_http;
  } catch (e) { $('message').textContent = e.message; }
}
async function action(args) {
  if (busy) return;
  busy = true; document.querySelectorAll('button').forEach(b => b.disabled = true);
  $('message').textContent = 'Working…';
  try { await run(args); $('message').textContent = 'Saved'; }
  catch (e) { $('message').textContent = e.message; }
  finally { busy = false; document.querySelectorAll('button').forEach(b => b.disabled = false); await refresh(); }
}
$('on').onclick = () => action(['on']);
$('timed').onclick = () => action(['on', $('duration').value]);
$('off').onclick = () => action(['off']);
$('reload').onclick = () => action(['reload']);
$('adb-on').onclick = () => action(['adb', 'on']);
$('adb-off').onclick = () => action(['adb', 'off']);
$('lanes-on').onclick = () => action(['lanes', '4', '15m']);
$('lanes-off').onclick = () => action(['lanes', '1']);
$('save-endpoint').onclick = () => {
  const args = ['endpoint', $('endpoint').value];
  if ($('tls-name').value.trim()) args.push('--tls-name', $('tls-name').value.trim());
  if ($('http').checked) {
    if (!confirm('Unencrypted HTTP exposes enrollment credentials. Only use on a trusted network. Continue?')) return;
    args.push('--allow-http');
  }
  action(args);
};
$('refresh').onclick = refresh;
refresh();
// Only poll while this page is visible. No device-side monitoring service.
setInterval(() => { if (!document.hidden && !busy) refresh(); }, 5000);
