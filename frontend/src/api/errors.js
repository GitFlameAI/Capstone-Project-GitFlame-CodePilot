// Central mapping from backend errors to what the UI should show and do.
//
// Sprint 5 goal: every failure the backend can return (an invalid/expired token,
// GitFlame being unreachable, the Agent Engine being down, the database/queue
// being unavailable, an apply that failed) becomes a clear, human message and a
// clear next action instead of a generic "something went wrong".
//
// The backend returns machine-readable error codes (ApiError.code) alongside an
// HTTP status (ApiError.status). We key off the code first and fall back to the
// status, so the UI never has to string-match human-readable text.
//
//   import { describeError, isTokenProblem, isRetryable } from '@/api/errors'
//   const info = describeError(e)   // { title, message, kind, retryable, tokenProblem }

import { ApiError } from './client.js'

// Error codes that mean "the GitFlame access token / connection is missing,
// expired, revoked or belongs to another user" — the user must (re)connect.
const TOKEN_CODES = new Set([
  'unauthorized',
  'gitflame_auth_error',
  'gitflame_connection_required',
  'gitflame_connection_error',
  'gitflame_token_inactive',
  'gitflame_token_expired',
  'gitflame_reauth_required',
  'gitflame_user_mismatch',
  'missing_access_token',
  'connection_not_found',
  'invalid_token',
])

// Error codes that describe a transient outage — retrying later can succeed.
const RETRYABLE_CODES = new Set([
  'network_error',
  'queue_unavailable',
  'agent_engine_error',
  'agent_engine_unreachable',
  'inference_timeout',
  'client_timeout',
  'gitflame_client_unavailable',
  'gitflame_unreachable',
  'storage_error',
  'recommendation_service_unavailable',
])

export function isTokenProblem(err) {
  if (!(err instanceof ApiError)) return false
  if (TOKEN_CODES.has(err.code)) return true
  return err.status === 401 || err.status === 403
}

export function isRetryable(err) {
  if (!(err instanceof ApiError)) return false
  if (RETRYABLE_CODES.has(err.code)) return true
  // 502/503/504 are upstream/queue/timeout problems that are worth retrying.
  return err.status === 502 || err.status === 503 || err.status === 504
}

// Turn any thrown value into a { title, message, kind, retryable, tokenProblem }
// descriptor the UI can render directly. `kind` is one of:
//   'auth' | 'gitflame' | 'agent' | 'storage' | 'validation' | 'apply' | 'generic'
export function describeError(err) {
  const tokenProblem = isTokenProblem(err)
  const retryable = isRetryable(err)

  if (!(err instanceof ApiError)) {
    return {
      title: 'Something went wrong',
      message: (err && err.message) || 'An unexpected error occurred. Please try again.',
      kind: 'generic',
      retryable: false,
      tokenProblem: false,
    }
  }

  const code = err.code || ''
  const status = err.status || 0
  const detail = err.message || ''

  // 1. Backend itself is unreachable (network error / wrong URL / service down).
  if (code === 'network_error' || status === 0) {
    return {
      title: 'Can’t reach CodePilot',
      message:
        'The CodePilot backend didn’t respond. Check that the service is running and reachable, then try again.',
      kind: 'storage',
      retryable: true,
      tokenProblem: false,
    }
  }

  // 2. Token / connection problems — the user must (re)connect the repository.
  if (tokenProblem) {
    let message = 'Your GitFlame access token is missing, expired or was revoked. Re-connect the repository to continue.'
    if (code === 'gitflame_user_mismatch') message = 'This GitFlame token belongs to a different account than the one already connected.'
    else if (code === 'gitflame_connection_required') message = 'This repository isn’t connected yet. Connect it with a GitFlame access token to continue.'
    else if (code === 'missing_access_token') message = 'An access token is required to connect the repository.'
    return {
      title: 'Access token problem',
      message,
      kind: 'auth',
      retryable: false,
      tokenProblem: true,
    }
  }

  // 3. GitFlame API unreachable / apply failure (writing branch/commit/PR).
  if (code === 'gitflame_apply_error' || code === 'gitflame_apply_failed') {
    return {
      title: 'Couldn’t apply the changes to GitFlame',
      message:
        detail ||
        'CodePilot generated the files but GitFlame rejected the branch, commit or pull request. Nothing was changed on your default branch — you can safely retry.',
      kind: 'apply',
      retryable: true,
      tokenProblem: false,
    }
  }
  if (code === 'gitflame_client_unavailable' || code === 'gitflame_unreachable') {
    return {
      title: 'GitFlame is unavailable',
      message:
        'CodePilot couldn’t reach GitFlame. This is usually temporary — please try again in a moment.',
      kind: 'gitflame',
      retryable: true,
      tokenProblem: false,
    }
  }

  // 4. Agent Engine problems (plan / code generation).
  if (
    code === 'agent_engine_error' ||
    code === 'agent_engine_unreachable' ||
    code === 'inference_timeout' ||
    code === 'client_timeout'
  ) {
    return {
      title: 'The AI service is busy',
      message:
        'The Agent Engine couldn’t finish in time. It may still be warming up — please retry.',
      kind: 'agent',
      retryable: true,
      tokenProblem: false,
    }
  }

  // 5. Queue / storage / database problems.
  if (code === 'queue_unavailable' || code === 'storage_error' || status === 503) {
    return {
      title: 'Service temporarily unavailable',
      message:
        detail && status !== 503
          ? detail
          : 'A backend component (queue or database) is temporarily unavailable. Please try again shortly.',
      kind: 'storage',
      retryable: true,
      tokenProblem: false,
    }
  }

  // 6. Validation errors — show the backend's own detail, it is user-facing.
  if (code === 'validation_error' || status === 422) {
    return {
      title: 'Check the details',
      message: detail || 'Some of the information provided is invalid.',
      kind: 'validation',
      retryable: false,
      tokenProblem: false,
    }
  }

  // 7. Not-found (e.g. session/task expired server-side).
  if (status === 404) {
    return {
      title: 'Not found',
      message: detail || 'The requested item no longer exists. It may have expired — start again.',
      kind: 'generic',
      retryable: false,
      tokenProblem: false,
    }
  }

  // Fallback: use the backend detail if present.
  return {
    title: 'Something went wrong',
    message: detail || `Request failed (${status}).`,
    kind: 'generic',
    retryable,
    tokenProblem: false,
  }
}
