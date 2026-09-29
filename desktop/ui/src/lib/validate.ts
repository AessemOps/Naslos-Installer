import type { InstallInput } from './types';
import { shortName } from './steps';

// validate mirrors internal/config.validation so the wizard rejects bad input
// before the engine is spawned (the engine validates again, authoritatively).

const LABEL_RE = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;
const UID_RE = /^[a-zA-Z][a-zA-Z0-9._-]{0,31}$/;
const NAME_RE = /^[a-zA-Z0-9][a-zA-Z0-9-]{0,14}$/;

export function isIPv4(value: string): boolean {
  const parts = value.trim().split('.');
  if (parts.length !== 4) return false;
  return parts.every((p) => /^\d{1,3}$/.test(p) && Number(p) <= 255);
}

export function validatePassword(pw: string): string | null {
  if (pw.length < 8) return 'Password must be at least 8 characters.';
  if (pw.length > 72) return 'Password must be at most 72 characters.';
  if (!/[A-Z]/.test(pw) || !/[a-z]/.test(pw) || !/[0-9]/.test(pw)) {
    return 'Password must contain an uppercase letter, a lowercase letter and a digit.';
  }
  return null;
}

// validate returns a map of field -> message; an empty object means valid.
export function validate(input: InstallInput): Partial<Record<keyof InstallInput, string>> {
  const errors: Partial<Record<keyof InstallInput, string>> = {};

  if (!isIPv4(input.nodeIp)) {
    errors.nodeIp = 'Enter the IPv4 address of the freshly booted node.';
  }

  const domain = input.domain.trim();
  if (!domain) {
    errors.domain = 'A local domain is required.';
  } else if (/[ \t/:]/.test(domain)) {
    errors.domain = 'Use a bare hostname, e.g. naslos.local.';
  } else if (!domain.toLowerCase().split('.').every((l) => LABEL_RE.test(l))) {
    errors.domain = 'Each domain label must be lowercase letters, digits or hyphens.';
  }

  if (!UID_RE.test(input.adminUser)) {
    errors.adminUser = 'Must start with a letter; letters, digits, . _ - only.';
  }

  const pwError = validatePassword(input.adminPassword);
  if (pwError) errors.adminPassword = pwError;

  const name = shortName(input.name, domain);
  if (!name || !NAME_RE.test(name)) {
    errors.name = 'Advertised name must be 1–15 letters, digits or hyphens.';
  }

  return errors;
}

export function isValid(input: InstallInput): boolean {
  return Object.keys(validate(input)).length === 0;
}
