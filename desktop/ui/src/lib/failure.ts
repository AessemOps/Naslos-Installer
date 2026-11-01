// Failure classification shared by the wizard screens.
//
// The engine reports a failure as a short summary (`msg`) plus a detail line
// (`output`). The stale-state dead end is the only failure the error screen can
// recover from on its own, and the actionable hint ("pass --reinstall") lives
// in the detail, so both fields have to be searched. Checking only `msg` made
// the "Start over" action unreachable.

import type { EngineError } from './types';

export const STALE_STATE_HINT = '--reinstall';

export function isStaleStateFailure(err: Pick<EngineError, 'msg' | 'output'> | null | undefined): boolean {
  if (!err) return false;
  return `${err.msg ?? ''} ${err.output ?? ''}`.includes(STALE_STATE_HINT);
}
