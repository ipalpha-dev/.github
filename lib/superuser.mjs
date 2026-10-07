import fs from 'node:fs';
import { pathToFileURL } from 'node:url';

// Mirrors shared-js/br-phone: setup can run before that package is installed.
export function normalizePhone(raw) {
  let digits = String(raw ?? '').replace(/\D/g, '');
  if (digits.length > 11 && digits.startsWith('55')) digits = digits.slice(2);
  if (digits.length === 12 && digits.startsWith('0')) digits = digits.slice(1);
  if (!/^[1-9]{2}9\d{8}$/.test(digits)) throw new Error('invalidPhone');
  return `+55${digits}`;
}

export function normalizeName(raw) {
  const name = String(raw ?? '').trim().replace(/[ \t]+/g, ' ');
  if (!name || name.length > 200 || /[\x00-\x1f\x7f]/.test(name)) throw new Error('invalidName');
  return name;
}

export function writeSuperuser(file, rawName, rawPhone) {
  const name = normalizeName(rawName);
  const phone = normalizePhone(rawPhone);
  let text = fs.readFileSync(file, 'utf8');
  for (const [key, value] of [['SUPERUSER_NAME', name], ['SUPERUSER_PHONE', phone]]) {
    // The generated launcher reads .env as data, never as shell code. Retain the
    // literal name (including apostrophes/$), without evaluation or interpolation.
    const line = `${key}=${value}`;
    const pattern = new RegExp(`^${key}=.*$`, 'm');
    text = pattern.test(text) ? text.replace(pattern, () => line) : text.replace(/\n?$/, '\n') + line + '\n';
  }
  const temporary = `${file}.superuser-${process.pid}`;
  try {
    fs.writeFileSync(temporary, text, { mode: 0o600, flag: 'wx' });
    fs.renameSync(temporary, file);
  } finally {
    if (fs.existsSync(temporary)) fs.unlinkSync(temporary);
  }
}

const cli = process.argv[1] && fs.existsSync(process.argv[1]) && import.meta.url === pathToFileURL(fs.realpathSync(process.argv[1])).href;
if (cli && process.argv[2] === 'normalize') {
  let input = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { input += chunk; });
  process.stdin.on('end', () => {
    try { process.stdout.write(process.argv[3] === 'phone' ? normalizePhone(input) : normalizeName(input)); }
    catch { process.exitCode = 1; }
  });
} else if (cli && process.argv[2] === 'write') {
  try { writeSuperuser(process.argv[3], process.env.IPALPHA_SEED_NAME, process.env.IPALPHA_SEED_PHONE); }
  catch { process.exitCode = 1; }
}
