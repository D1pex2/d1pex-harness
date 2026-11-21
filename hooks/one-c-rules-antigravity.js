// Автоподключение правил 1С для Antigravity 1.0.0 (SessionStart, PreInvocation, PostToolUse).
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const os = require('node:os');
const {isOneC} = require('./one-c-detector.js');

const hash = value => crypto.createHash('sha256').update(value).digest('hex');

function resolveSkillPath() {
  const home = os.homedir();
  const geminiConfig = process.env.ANTIGRAVITY_CONFIG_DIR || path.join(home, '.gemini', 'config');
  const candidates = [
    process.env.ONE_C_RULES_SKILL,
    process.env.PLUGIN_ROOT ? path.join(process.env.PLUGIN_ROOT, 'skills', '1c-development-rules', 'SKILL.md') : '',
    path.resolve(__dirname, '..', 'skills', '1c-development-rules', 'SKILL.md'),
    path.join(geminiConfig, 'skills', '1c-development-rules', 'SKILL.md'),
    path.join(home, '.agents', 'skills', '1c-development-rules', 'SKILL.md'),
  ].filter(Boolean);

  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) return candidate;
  }

  const pluginsDir = path.join(geminiConfig, 'plugins');
  try {
    for (const entry of fs.readdirSync(pluginsDir, {withFileTypes: true})) {
      if (!entry.isDirectory()) continue;
      const candidate = path.join(pluginsDir, entry.name, 'skills', '1c-development-rules', 'SKILL.md');
      if (fs.existsSync(candidate)) return candidate;
    }
  } catch {}

  throw new Error('Не найден навык 1c-development-rules/SKILL.md');
}

function defaultRuntimeDir() {
  if (process.env.PLUGIN_DATA) return path.join(process.env.PLUGIN_DATA, 'one-c-runtime');
  return path.join(__dirname, 'one-c-runtime-antigravity');
}

function writeState(runtime, stateFile, state) {
  fs.mkdirSync(runtime, {recursive: true});
  const temporary = stateFile + '.' + crypto.randomUUID() + '.tmp';
  try {
    fs.writeFileSync(temporary, JSON.stringify(state));
    fs.renameSync(temporary, stateFile);
  } catch (error) {
    try { fs.unlinkSync(temporary); } catch {}
    throw error;
  }
}

function run(input, event, runtime = defaultRuntimeDir()) {
  if (!input || typeof input !== 'object') return {};
  const convId = input.conversationId || input.session_id;
  if (process.env.EVENTLOG_AUTORUN || !convId) return {};

  const stateFile = path.join(runtime, hash(convId) + '.json');
  let state = {};
  try { state = JSON.parse(fs.readFileSync(stateFile, 'utf8')); } catch {}

  if (event === 'post') {
    // PostToolUse в Antigravity всегда возвращает {}: только фиксируем признак 1С для следующего PreInvocation.
    if (!state.active && isOneC(input)) {
      writeState(runtime, stateFile, {active: true, emitted: false, hash: state.hash || ''});
    }
    return {};
  }

  if (event === 'startup') state.emitted = false;
  const active = Boolean(state.active || isOneC(input));
  let result = {};
  if (active) {
    const skill = resolveSkillPath();
    const text = fs.readFileSync(skill, 'utf8') + '\n' + fs.readFileSync(path.join(path.dirname(skill), 'references', 'rules.md'), 'utf8');
    const digest = hash(text);
    if (!state.emitted || state.hash !== digest) {
      result = {
        injectSteps: [
          {
            systemMessage: {
              systemMessage: `Автоматически подключены правила проекта 1С. Источник: ${skill}. Если сообщение обрезано, прочитай references/rules.md рядом с этим файлом целиком.\n${text}`,
            },
          },
        ],
      };
      state = {active: true, emitted: true, hash: digest};
      writeState(runtime, stateFile, state);
    }
  }
  return result;
}

if (require.main === module) {
  const event = process.argv[2];
  try {
    const result = run(JSON.parse(fs.readFileSync(0, 'utf8') || '{}'), event);
    process.stdout.write(JSON.stringify(result));
  } catch (error) {
    if (event === 'post') {
      process.stdout.write('{}');
    } else {
      const message = 'Ошибка подключения правил 1С: ' + error.message;
      process.stdout.write(JSON.stringify({
        injectSteps: [{systemMessage: {systemMessage: message}}],
      }));
    }
  }
}

module.exports = {run, resolveSkillPath, VERSION: '1.0.0'};
