import type { EngineEvent, Handoff } from './types';
import { autheliaURL } from './steps';

// The engine may deliver machine-readable values in `event.data` (the contract's
// optional object) or only in the human `msg`; mergeHandoff accepts both.

const OTPAUTH_RE = /otpauth:\/\/[^\s'"]+/;
const ZIP_RE = /[^\s'"]+\.zip/;
const URL_RE = /https?:\/\/[^\s'"]+/;

export function mergeHandoff(previous: Handoff, event: EngineEvent): Handoff {
  const next: Handoff = { ...previous };
  const data = event.data ?? {};

  if (data.otpauth) next.otpauthUri = data.otpauth;
  if (data.secret) next.secret = data.secret;
  if (data.path) next.zipPath = data.path;
  if (data.loginUrl) next.loginUrl = data.loginUrl;

  const msg = event.msg ?? '';
  if (!next.otpauthUri) {
    const match = msg.match(OTPAUTH_RE);
    if (match) next.otpauthUri = match[0];
  }
  if (!next.zipPath) {
    const match = msg.match(ZIP_RE);
    if (match) next.zipPath = match[0];
  }
  if (!next.loginUrl && event.step === 'done') {
    const match = msg.match(URL_RE);
    if (match) next.loginUrl = match[0];
  }
  return next;
}

export function secretFromURI(uri: string | undefined): string | undefined {
  if (!uri) return undefined;
  try {
    return new URL(uri).searchParams.get('secret') ?? undefined;
  } catch {
    return undefined;
  }
}

export function resolveLoginUrl(domain: string, handoff: Handoff): string {
  // The value can come from engine output; only hand an https URL to the
  // browser. Anything else (http:, javascript:, data:) falls back to the
  // locally derived address.
  if (handoff.loginUrl) {
    try {
      if (new URL(handoff.loginUrl).protocol === 'https:') return handoff.loginUrl;
    } catch {
      // fall through
    }
  }
  return autheliaURL(domain);
}
