// Детектор проектов 1С, выделенный из локального harness. Версия 1.0.0.
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const read = file => fs.readFileSync(file, 'utf8');
const configPath = path.join(process.env.CODEX_HOME || path.join(os.homedir(), '.codex'), 'config.toml');

function strings(value, result = []) {
  if (typeof value === 'string') result.push(value);
  else if (Array.isArray(value)) for (const x of value) strings(x, result);
  else if (value && typeof value === 'object') for (const x of Object.values(value)) strings(x, result);
  return result;
}
function projectIsOneC(cwd) {
  if (!cwd) return false;
  let dir = path.resolve(cwd);
  try { if (fs.statSync(dir).isFile()) dir = path.dirname(dir); } catch {}
  const initialDir = dir;
  while (true) {
    if (fs.existsSync(path.join(dir, '.v8-project.json')) || xmlIsOneC(path.join(dir, 'src', 'cf', 'Configuration.xml')) || xmlIsOneC(path.join(dir, 'Configuration.xml'))) return true;
    for (const candidateDir of dir === initialDir || fs.existsSync(path.join(dir, '.git')) ? [dir, path.join(dir, 'src')] : []) {
      try { if (fs.readdirSync(candidateDir, {withFileTypes: true}).some(e => e.isFile() && (/\.(?:bsl|os|epf|erf|cf|cfe)$/i.test(e.name) || (/\.xml$/i.test(e.name) && xmlIsOneC(path.join(candidateDir, e.name)))))) return true; } catch {}
    }
    for (const sub of ['src/cfe', 'cfe']) {
      try { if (fs.readdirSync(path.join(dir, sub)).some(n => xmlIsOneC(path.join(dir, sub, n, 'Configuration.xml')))) return true; } catch {}
    }
    if (fs.existsSync(path.join(dir, '.git'))) return false;
    const parent = path.dirname(dir);
    if (parent === dir) return false;
    dir = parent;
  }
}
function xmlIsOneC(file) {
  try {
    const fd = fs.openSync(file, 'r'); const buf = Buffer.alloc(4096); let n;
    try { n = fs.readSync(fd, buf); } finally { fs.closeSync(fd); }
    return /https?:\/\/v8\.1c\.ru\//.test(buf.subarray(0, n).toString('utf8'));
  } catch { return false; }
}
function isOneC(input) {
  const name = input.tool_name || '';
  if (/mcp__(?:bsl_context|code_index|one_c_form_viewer)__/.test(name) || /mcp__.*__(?:execute_query|validate_query|get_structure|preview_posting|get_event_log|execute_code)$/.test(name)) return true;
  if (name.startsWith('mcp__')) {
    let config = ''; try { config = read(configPath); } catch {}
    for (const section of config.matchAll(/^\[mcp_servers\.([^\]\r\n]+)\]\r?\n([\s\S]*?)(?=^\[|$(?![\s\S]))/gm)) {
      const server = section[1].replace(/^["']|["']$/g, '').replace(/-/g, '_');
      if (/^url\s*=\s*["'][^\r\n]*\/hs\/[^\r\n]*mcp/im.test(section[2]) && name.startsWith('mcp__' + server + '__')) return true;
    }
  }
  const values = strings(input.tool_input || {});
  if (values.some(s => /\.(?:bsl|os|epf|erf|cfe|cf)(?=[\s"'`,;:)\]}]|$)|\.v8-project\.json|https?:\/\/v8\.1c\.ru\//i.test(s))) return true;
  if ([input.cwd, input.tool_input?.workdir, input.tool_input?.cwd].some(projectIsOneC)) return true;
  for (const value of values) {
    // Пути из структурированных аргументов и строк оболочки проверяются без исполнения кода.
    const candidates = [value, ...(value.match(/[A-Za-z]:[\\/][^"'\r\n;]+/g) || [])];
    for (const candidate of candidates) {
      if (candidate.length > 1024) continue;
      if (projectIsOneC(candidate)) return true;
      if (/\.xml$/i.test(candidate) && xmlIsOneC(candidate)) return true;
    }
  }
  return false;
}
module.exports = {isOneC};
