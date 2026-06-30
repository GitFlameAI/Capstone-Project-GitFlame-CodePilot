// Minimal, dependency-free Markdown renderer for the implementation plan preview.
//
// We deliberately avoid pulling a markdown library to keep the dependency set at
// vue + vue-router (the project's stated constraint). It covers exactly the
// subset that `plan.md` uses: headings, bold/italic, inline code, fenced code
// blocks, ordered/unordered lists, links and paragraphs.
//
// SECURITY: every piece of source text is HTML-escaped FIRST, then a small set of
// inline patterns is re-introduced as markup. Raw HTML in the input is therefore
// rendered as text, not executed — safe for AI-generated content.

function escapeHtml(text) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

function inline(text) {
  let out = escapeHtml(text)
  // inline code first so its contents are not touched by bold/italic
  out = out.replace(/`([^`]+)`/g, (_, code) => `<code>${code}</code>`)
  out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  out = out.replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<em>$2</em>')
  out = out.replace(
    /\[([^\]]+)\]\((https?:[^)\s]+)\)/g,
    (_, label, url) => `<a href="${url}" target="_blank" rel="noopener noreferrer">${label}</a>`,
  )
  return out
}

export function renderMarkdown(markdown) {
  if (!markdown) return ''
  const lines = String(markdown).replace(/\r\n/g, '\n').split('\n')
  const html = []
  let listType = null // 'ul' | 'ol'
  let inCode = false
  let codeBuffer = []

  function closeList() {
    if (listType) {
      html.push(`</${listType}>`)
      listType = null
    }
  }

  for (const raw of lines) {
    const line = raw

    // fenced code block toggle
    if (/^\s*```/.test(line)) {
      if (inCode) {
        html.push(`<pre class="md-pre"><code>${escapeHtml(codeBuffer.join('\n'))}</code></pre>`)
        codeBuffer = []
        inCode = false
      } else {
        closeList()
        inCode = true
      }
      continue
    }
    if (inCode) {
      codeBuffer.push(line)
      continue
    }

    // horizontal rule
    if (/^\s*---\s*$/.test(line)) {
      closeList()
      html.push('<hr class="md-hr" />')
      continue
    }

    // headings
    const heading = line.match(/^(#{1,6})\s+(.*)$/)
    if (heading) {
      closeList()
      const level = heading[1].length
      html.push(`<h${level} class="md-h md-h${level}">${inline(heading[2])}</h${level}>`)
      continue
    }

    // ordered list
    const ol = line.match(/^\s*\d+\.\s+(.*)$/)
    if (ol) {
      if (listType !== 'ol') {
        closeList()
        html.push('<ol class="md-ol">')
        listType = 'ol'
      }
      html.push(`<li>${inline(ol[1])}</li>`)
      continue
    }

    // unordered list
    const ul = line.match(/^\s*[-*]\s+(.*)$/)
    if (ul) {
      if (listType !== 'ul') {
        closeList()
        html.push('<ul class="md-ul">')
        listType = 'ul'
      }
      html.push(`<li>${inline(ul[1])}</li>`)
      continue
    }

    // blank line
    if (/^\s*$/.test(line)) {
      closeList()
      continue
    }

    // paragraph
    closeList()
    html.push(`<p class="md-p">${inline(line)}</p>`)
  }

  if (inCode && codeBuffer.length) {
    html.push(`<pre class="md-pre"><code>${escapeHtml(codeBuffer.join('\n'))}</code></pre>`)
  }
  closeList()
  return html.join('\n')
}
