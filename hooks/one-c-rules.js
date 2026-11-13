// Автоподключение правил 1С 1.0.2. Наблюдатель и контроль session.md не запускаются.
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const os = require('node:os');
const home = os.homedir();
const {isOneC} = require('./one-c-detector.js');
const hash = value => crypto.createHash('sha256').update(value).digest('hex');

function run(input, event, runtime = path.join(home, '.agents/hooks/one-c-runtime')) {
  if (process.env.EVENTLOG_AUTORUN || !input.session_id) return {};
  const stateFile = path.join(runtime, hash(input.session_id) + '.json');
  let state = {};
  try { state = JSON.parse(fs.readFileSync(stateFile, 'utf8')); } catch {}
  if (event === 'startup') state.emitted = false;
  // Детектор работает независимо от других навыков и не запускает наблюдателей.
  const active = Boolean(state.active || isOneC(input));
  let result = {};
  if (active) {
    const skill = path.join(home, '.agents/skills/1c-development-rules/SKILL.md');
    const text = fs.readFileSync(skill, 'utf8') + '\n' + fs.readFileSync(path.join(path.dirname(skill), 'references/rules.md'), 'utf8');
    const digest = hash(text);
    if (!state.emitted || state.hash !== digest) {
      result = {hookSpecificOutput: {
        hookEventName: event === 'startup' ? 'SessionStart' : 'PreToolUse',
        additionalContext: `Автоматически подключены правила проекта 1С. Источник: ${skill}. Если сообщение обрезано, прочитай references/rules.md рядом с этим файлом целиком.\n${text}`
      }};
      state = {active: true, emitted: true, hash: digest};
    }
  }
  if (active) {
    fs.mkdirSync(runtime, {recursive: true});
    const temporary = stateFile + '.' + crypto.randomUUID() + '.tmp';
    fs.writeFileSync(temporary, JSON.stringify(state));
    fs.renameSync(temporary, stateFile);
  }
  return result;
}

if (require.main === module) {
  const event = process.argv[2];
  try {
    const result = run(JSON.parse(fs.readFileSync(0, 'utf8') || '{}'), event);
    if (Object.keys(result).length) process.stdout.write(JSON.stringify(result));
  } catch (error) {
    const message = 'Ошибка подключения правил 1С: ' + error.message;
    if (event === 'pre') { process.stderr.write(message); process.exitCode = 2; }
    else process.stdout.write(JSON.stringify({systemMessage: message}));
  }
}
module.exports = {run, VERSION: '1.0.2'};
