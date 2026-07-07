// Copy text to the clipboard with a graceful fallback.
//
// navigator.clipboard.writeText only works in "secure contexts" (https or
// localhost). On the deployed VM the demo is served over plain http, where the
// async Clipboard API is unavailable, so a naive `await navigator.clipboard...`
// silently fails — which is exactly the "copy button does nothing" the user
// reported. This helper tries the modern API first and falls back to a hidden
// <textarea> + document.execCommand('copy'), which works over http too.
//
//   import { copyText } from '@/utils/clipboard'
//   const ok = await copyText(url)   // -> true on success, false otherwise
export async function copyText(text) {
  const value = String(text ?? '')
  // 1. Modern async clipboard (secure contexts only).
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(value)
      return true
    }
  } catch {
    // fall through to the legacy path
  }
  // 2. Legacy execCommand fallback (works over http).
  try {
    const ta = document.createElement('textarea')
    ta.value = value
    ta.setAttribute('readonly', '')
    // Keep it out of view and out of the layout flow.
    ta.style.position = 'fixed'
    ta.style.top = '-1000px'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    ta.setSelectionRange(0, value.length)
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
