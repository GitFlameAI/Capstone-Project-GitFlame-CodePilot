// Single entry point the UI imports. It transparently selects the mock backend
// or the real Go backend depending on whether VITE_API_BASE is configured.
//
//   import { api, USING_MOCK, pollTask } from '@/api'
//
// Every function returns the same shape in both modes, so components never need
// to know which backend they are talking to. The frontend NEVER talks to the
// SERGE-based Agent Engine directly; it only ever calls the Go backend, which
// owns orchestration, the Redis queue and task state. In live mode the backend
// also owns the GitFlame access token — the frontend only holds a session cookie.

import { httpApi, ApiError } from './client.js'
import { mockApi } from './mock.js'
import { isTokenProblem } from './errors.js'
import { markTokenInvalid } from '../store/session.js'

export const USING_MOCK = !import.meta.env.VITE_API_BASE

// In live mode, wrap every backend call so an auth failure (the session cookie
// or the stored GitFlame token being missing / expired / revoked, surfaced by the
// backend as 401/403 or a gitflame_* connection code) is turned into an explicit,
// visible reconnect prompt instead of a generic error.
function withAuthGuard(realApi) {
  const wrapped = {}
  for (const [name, fn] of Object.entries(realApi)) {
    wrapped[name] = async (...args) => {
      try {
        return await fn(...args)
      } catch (e) {
        // Creating / replacing a connection legitimately sends a token and may
        // report an auth error inline; don't trip the global reconnect gate for
        // those — the calling screen shows the message itself.
        const isConnectionCall = name === 'createConnection' || name === 'reconnectConnection'
        if (!isConnectionCall && isTokenProblem(e)) {
          markTokenInvalid(e.message || 'The session or access token is invalid or has expired.')
        }
        throw e
      }
    }
  }
  return wrapped
}

export const api = USING_MOCK ? mockApi : withAuthGuard(httpApi)
export { ApiError }

// pollTask repeatedly calls GET /ai/tasks/{taskId} until the agent task reaches a
// terminal state (`completed` or `failed`), or until the client-side timeout is
// hit. It is used for both the initial plan generation and plan corrections,
// which are asynchronous.
//
//   const task = await pollTask(taskId, { onTick: (t) => (status.value = t.status) })
//
// Returns the terminal task object (caller inspects task.status / task.error).
// Throws ApiError(code:'client_timeout') if the task never finishes in time, or
// ApiError(code:'cancelled') if an AbortSignal is aborted (e.g. modal closed).
export async function pollTask(taskId, { interval = 1200, timeoutMs = 120000, signal, onTick } = {}) {
  const start = Date.now()
  // First tick immediately so the UI reflects the real status without waiting.
  for (;;) {
    if (signal?.aborted) throw new ApiError('Plan generation was cancelled.', 0, 'cancelled')
    const task = await api.getTask(taskId)
    if (typeof onTick === 'function') onTick(task)
    if (task.status === 'completed' || task.status === 'failed') return task
    if (Date.now() - start > timeoutMs) {
      throw new ApiError(
        'The Agent Engine is taking longer than expected to generate the plan.',
        504,
        'client_timeout',
      )
    }
    await new Promise((resolve) => setTimeout(resolve, interval))
  }
}
