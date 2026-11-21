// Детектор проектов 1С для Codex и Antigravity. Версия 1.1.0.
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');

const read = file => fs.readFileSync(file, 'utf8');
const norm = value => String(value || '').toLowerCase().replace(/-/g, '_');
const ONE_C_SERVER_RE = /(?:^|_)(?:bsl_context|code_index|one_c_form_viewer)$/;
const ONE_C_TOOL_RE = /^(?:execute_query|validate_query|get_structure|preview_posting|get_event_log|execute_code)$/;
const ONE_C_HTTP_MCP_RE = /\/hs\/[^\r\n]*mcp/i;

function codexConfigPath() {
  return path.join(process.env.CODEX_HOME || path.join(os.homedir(), '.codex'), 'config.toml');
}

function antigravityConfigDir() {
  return process.env.ANTIGRAVITY_CONFIG_DIR || path.join(os.homedir(), '.gemini', 'config');
}

function collectAntigravityOneCServers() {
  const servers = new Set();
  const baseDir = antigravityConfigDir();
  const configs = [{file: path.join(baseDir, 'mcp_config.json'), prefixes: ['']}];
  const pluginsDir = path.join(baseDir, 'plugins');
  try {
    for (const entry of fs.readdirSync(pluginsDir, {withFileTypes: true})) {
      if (!entry.isDirectory()) continue;
      const pluginDir = path.join(pluginsDir, entry.name);
      const prefixes = [entry.name];
      try {
        const manifest = JSON.parse(read(path.join(pluginDir, 'plugin.json')));
        if (manifest && typeof manifest.name === 'string' && manifest.name && !prefixes.includes(manifest.name)) {
          prefixes.push(manifest.name);
        }
      } catch {}
      configs.push({file: path.join(pluginDir, 'mcp_config.json'), prefixes});
    }
  } catch {}
  for (const {file, prefixes} of configs) {
    let parsed;
    try { parsed = JSON.parse(read(file)); } catch { continue; }
    const map = parsed && parsed.mcpServers && typeof parsed.mcpServers === 'object' ? parsed.mcpServers : null;
    if (!map) continue;
    for (const [serverName, cfg] of Object.entries(map)) {
      if (!cfg || typeof cfg !== 'object') continue;
      const url = typeof cfg.url === 'string' ? cfg.url : (typeof cfg.serverUrl === 'string' ? cfg.serverUrl : '');
      if (!ONE_C_HTTP_MCP_RE.test(url)) continue;
      servers.add(norm(serverName));
      for (const prefix of prefixes) {
        if (prefix) servers.add(norm(prefix + '_' + serverName));
      }
    }
  }
  return servers;
}

function strings(value, result = []) {
  if (typeof value === 'string') result.push(value);
  else if (Array.isArray(value)) for (const x of value) strings(x, result);
  else if (value && typeof value === 'object') for (const x of Object.values(value)) strings(x, result);
  return result;
}

function projectIsOneC(cwd) {
  if (!cwd || typeof cwd !== 'string') return false;
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

function isOneCMcpTool(input) {
  const codexName = input.tool_name || '';
  if (/mcp__(?:bsl_context|code_index|one_c_form_viewer)__/.test(codexName) || /mcp__.*__(?:execute_query|validate_query|get_structure|preview_posting|get_event_log|execute_code)$/.test(codexName)) return true;
  if (codexName.startsWith('mcp__')) {
    let config = ''; try { config = read(codexConfigPath()); } catch {}
    for (const section of config.matchAll(/^\[mcp_servers\.([^\]\r\n]+)\]\r?\n([\s\S]*?)(?=^\[|$(?![\s\S]))/gm)) {
      const server = section[1].replace(/^["']|["']$/g, '').replace(/-/g, '_');
      if (/^url\s*=\s*["'][^\r\n]*\/hs\/[^\r\n]*mcp/im.test(section[2]) && codexName.startsWith('mcp__' + server + '__')) return true;
    }
  }

  const agyCall = input.toolCall && typeof input.toolCall === 'object' ? input.toolCall : null;
  const agyName = (agyCall && typeof agyCall.name === 'string' ? agyCall.name : '') || '';
  const agyArgs = agyCall && agyCall.args && typeof agyCall.args === 'object' ? agyCall.args : null;
  if (agyName === 'call_mcp_tool' && agyArgs) {
    const serverNorm = norm(agyArgs.ServerName);
    const toolNorm = norm(agyArgs.ToolName);
    if (ONE_C_TOOL_RE.test(toolNorm) || ONE_C_SERVER_RE.test(serverNorm)) return true;
    if (serverNorm && collectAntigravityOneCServers().has(serverNorm)) return true;
  }
  const normalizedAgyName = norm(agyName);
  if (normalizedAgyName.startsWith('mcp_') && !normalizedAgyName.startsWith('mcp__')) {
    if (/(?:^|_)mcp_(?:.*_)?(?:bsl_context|code_index|one_c_form_viewer)_/.test(normalizedAgyName)) return true;
    if (/_(?:execute_query|validate_query|get_structure|preview_posting|get_event_log|execute_code)$/.test(normalizedAgyName)) return true;
    for (const server of collectAntigravityOneCServers()) {
      if (normalizedAgyName.startsWith('mcp_' + server + '_')) return true;
    }
  }
  return false;
}

function isOneC(input) {
  if (!input || typeof input !== 'object') return false;
  if (isOneCMcpTool(input)) return true;

  const agyArgs = input.toolCall && input.toolCall.args && typeof input.toolCall.args === 'object' ? input.toolCall.args : {};
  const values = [];
  strings(input.tool_input || {}, values);
  strings(agyArgs, values);
  if (typeof input.lastUserInput === 'string' && input.lastUserInput) values.push(input.lastUserInput);

  if (values.some(s => /\.(?:bsl|os|epf|erf|cfe|cf)(?=[\s"'`,;:)\]}]|$)|\.v8-project\.json|https?:\/\/v8\.1c\.ru\//i.test(s))) return true;

  const dirs = [
    input.cwd,
    ...(Array.isArray(input.workspacePaths) ? input.workspacePaths : []),
    input.tool_input?.workdir,
    input.tool_input?.cwd,
    agyArgs.Cwd,
    agyArgs.cwd,
  ];
  if (dirs.some(projectIsOneC)) return true;

  for (const value of values) {
    // Пути из структурированных аргументов и строк оболочки проверяются без исполнения кода.
    const candidates = [
      value,
      ...(value.match(/[A-Za-z]:[\\/][^"'\r\n;]+/g) || []),
      ...(value.match(/(?:^|[\s"'=])(\/[^\s"'\r\n;]+)/g) || []).map(m => m.replace(/^[\s"'=]+/, '')),
    ];
    for (const candidate of candidates) {
      if (candidate.length > 1024) continue;
      if (projectIsOneC(candidate)) return true;
      if (/\.xml$/i.test(candidate) && xmlIsOneC(candidate)) return true;
    }
  }
  return false;
}

module.exports = {isOneC, VERSION: '1.1.0'};
