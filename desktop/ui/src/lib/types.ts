// Types shared with the engine's NDJSON progress protocol (docs/installer-contract.md §5).

export interface InstallInput {
  nodeIp: string;
  domain: string;
  name: string;
  adminUser: string;
  adminPassword: string;
  addResolver: boolean;
}

export interface EngineError {
  step?: string;
  msg: string;
  output?: string;
}

// EngineEvent is one line of the engine's newline-delimited JSON stream. A
// terminal success line has step "done"; a terminal failure sets `error`.
// `data` carries step-specific machine-readable payloads when the engine emits
// them (otpauth URI, recovery ZIP path, login URL); the UI falls back to
// scanning `msg` when it is absent.
export interface EngineEvent {
  step?: string;
  status?: 'running' | 'ok' | 'failed';
  pct?: number;
  msg?: string;
  data?: Record<string, string>;
  error?: EngineError;
}

export interface ExitPayload {
  code: number | null;
  success: boolean;
}

export interface Handoff {
  loginUrl?: string;
  otpauthUri?: string;
  secret?: string;
  zipPath?: string;
}

export interface RunHandlers {
  onEvent: (event: EngineEvent) => void;
  onLog: (line: string) => void;
  onExit: (exit: ExitPayload) => void;
}
