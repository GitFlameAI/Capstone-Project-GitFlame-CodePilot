// Single entry point the UI imports. It transparently selects the mock backend
// or the real Go backend depending on whether VITE_API_BASE is configured.
//
//   import { api, USING_MOCK } from '@/api'
//
// Every function returns the same shape in both modes, so components never need
// to know which backend they are talking to.

import { httpApi, ApiError } from './client.js'
import { mockApi } from './mock.js'

export const USING_MOCK = !import.meta.env.VITE_API_BASE
export const api = USING_MOCK ? mockApi : httpApi
export { ApiError }
