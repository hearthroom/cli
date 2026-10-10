/**
 * 卡片離線預覽：假宿主＋真沙箱殼。殼是 dist-sandbox 的正式產物（真的 sdk、淨化、Markdown、
 * 事件順序、虛擬化），宿主由這一頁冒充：讀一個卡資料夾（rules.json、welcome.md、
 * preview/replies.md），用 hello 餵進去，再用 message.new／stream／done 模擬對話。
 *
 * 用法（舞台 repo 根）：
 *   npm run build:sandbox
 *   node scripts/serve-card-preview.mjs <card-dir>     （起靜態伺服器並開 /bench/card-preview/?card=/card/）
 * 或自己起任何靜態伺服器，把卡資料夾放在同源可讀的路徑，網址帶 ?card=<路徑>。
 *
 * 主控台：`__preview` 有 reload／stream／done／addUser／switchConversation／rerun／snapshot。
 * 不是測試、不進 build。
 */
(function () {
  const params = new URLSearchParams(location.search)
  const PROTOCOL = 1
  const frame = document.getElementById('frame')
  const logEl = document.getElementById('log')
  const log = (s, cls) => { const d = document.createElement('div'); if (cls) d.className = cls; d.textContent = s; logEl.appendChild(d); logEl.scrollTop = logEl.scrollHeight }

  const cardBase = (params.get('card') || '/card/').replace(/\/?$/, '/')
  const state = { card: null, seq: 0, messages: [], helloed: false, busy: false, samples: [] }

  async function fetchText(name) {
    const r = await fetch(cardBase + name, { cache: 'no-store' })
    if (!r.ok) return null
    return await r.text()
  }
  async function loadCard() {
    const rulesText = await fetchText('rules.json')
    const rules = rulesText ? JSON.parse(rulesText) : { rules: [] }
    const welcome = (await fetchText('welcome.md')) || ''
    const cardJson = JSON.parse((await fetchText('card.json')) || '{}')
    const samplesMd = (await fetchText('preview/replies.md')) || ''
    // preview/replies.md：以 `## ` 標題分段，每段一則回覆樣本。
    state.samples = samplesMd.split(/^## .*$/m).map((s) => s.trim()).filter(Boolean)
    const select = document.getElementById('samples')
    select.innerHTML = ''
    state.samples.forEach((s, i) => { const o = document.createElement('option'); o.value = String(i); o.textContent = `sample ${i + 1}: ${s.slice(0, 28).replace(/\n/g, ' ')}`; select.appendChild(o) })
    // push 會把規則裡的字面 assets/ 路徑改寫成上傳後的網址；本機沒有那一步，把它們指到 /card/assets/。
    const assets = (s) => String(s ?? '').replace(/(["'(])assets\//g, `$1${cardBase}assets/`)
    // ?rules=off：規則全停（text-only）——劇情為核的卡在這個狀態也要讀得順；綁定型的卡要看機制在文字裡是否可讀。
    const rulesOff = params.get('rules') === 'off'
    state.rulesOff = rulesOff
    state.card = {
      format: rules.cardFormat || 'mmd',
      rules: rulesOff ? [] : (rules.rules || []).map((r, i) => ({ id: r.id ?? i, name: r.name || '', find: r.find, replace: assets(r.replace), enabled: r.enabled !== false })),
      statusbar: rulesOff ? '' : assets(rules.mountTrigger || ''),
    }
    state.welcome = assets(welcome)
    // 替代開場 openings/alt-01.md、alt-02.md…（讀到第一個沒有的為止）：sdk.archive.start(n) 用，0＝主開場、1 起＝替代開場
    state.openings = [state.welcome]
    for (let i = 1; i < 100; i++) {
      const alt = await fetchText('openings/alt-' + String(i).padStart(2, '0') + '.md')
      if (alt == null) break
      state.openings.push(assets(alt))
    }
    state.name = cardJson.name || 'Card'
    state.playerName = cardJson.playerName || 'You'
    state.pageMode = rules.pageMode || 'classic'
    if (state.pageMode !== 'sandbox') log(`rules.json pageMode is "${state.pageMode}": the sandbox page is not what the player would see`, 'warn')
    log(`card loaded: ${state.card.rules.length} rule(s), function bar ${state.card.statusbar ? 'set' : 'empty'}`)
  }

  const post = (m) => frame.contentWindow.postMessage({ ms: PROTOCOL, ...m }, '*')
  const hello = () => ({
    theme: document.getElementById('theme').value,
    locale: params.get('locale') || 'zh-Hant',
    role: { name: state.name, avatarUrl: '' },
    user: { nickname: state.playerName, avatarUrl: '' },
    card: state.card,
    variants: null,
    capabilities: { saves: true, edit: false, send: true, archive: true },
    composer: true,
    chrome: document.getElementById('chrome').value === 'host' ? 'host' : 'shell',
    saves: {},
    // 殼的 debug 訊息（sdk.debug.log、腳本錯誤）不開面板也會轉送到這一頁的記錄；?debug=1 才把殼內面板與幾何疊層打開。
    debug: params.get('debug') === '1',
    viewportHeight: frame.clientHeight,
  })

  const sent = []
  // 殼（快取時）可能比 loadCard() 先 ready：hello 要等卡載好，否則 config.card 是 null、殼會丟錯。
  let cardLoaded
  const cardReady = new Promise((r) => { cardLoaded = r })
  window.addEventListener('message', (ev) => {
    if (ev.source !== frame.contentWindow || !ev.data || ev.data.ms !== PROTOCOL) return
    const m = ev.data
    sent.push(m)
    if (m.type === 'ready-shell' && !state.helloed) {
      state.helloed = true
      cardReady.then(() => {
        post({ type: 'hello', config: hello() })
        state.messages = [{ id: 'greeting', role: 'ai', content: state.welcome, serverId: null, state: 'done' }]
        state.archives = [newArchive(state.messages)]
        state.current = state.archives[0].id
        post({ type: 'messages', messages: state.messages })
        post({ type: 'history', more: false, loading: false })
      })
      return
    }
    if (m.type === 'debug') { log(`[debug ${m.level}] ${m.args.map((a) => (typeof a === 'string' ? a : JSON.stringify(a))).join(' ')}`, m.level === 'error' ? 'err' : m.level === 'warn' ? 'warn' : '') ; return }
    if (m.type === 'request') {
      log(`request ${m.op} ${JSON.stringify(m.args).slice(0, 120)}`)
      if (m.op === 'message.send') {
        post({ type: 'reply', reqId: m.reqId, ok: true, value: null })
        addUser(String(m.args[0]))
        // 模擬模型：若有樣本就串下一則樣本；沒有就回一句。
        const next = state.samples.length ? state.samples[(state.seq) % state.samples.length] : 'OK.'
        setTimeout(() => streamReply(next), 300)
        return
      }
      if (String(m.op).startsWith('archive.')) { archiveRequest(m); return }
      post({ type: 'reply', reqId: m.reqId, ok: true, value: null })
      return
    }
    if (m.type === 'ready') log('shell ready')
    if (m.type === 'stage') log(`stage ${m.state}`)
    if (m.type === 'composer') log(`composer ${m.visible ? 'shown' : 'hidden'}`)
    if (m.type === 'open-url') log(`open-url ${m.url}`)
  })

  function addUser(text) {
    const id = `l${++state.seq}u`
    state.messages.push({ id, role: 'user', content: text, serverId: null })
    post({ type: 'message.new', message: { id, role: 'user', content: text, serverId: null } })
  }
  async function streamReply(text, chunks = 24) {
    if (state.busy) { log('still streaming', 'warn'); return }
    state.busy = true
    const id = `l${++state.seq}`
    post({ type: 'message.new', message: { id, role: 'ai', content: '', serverId: null } })
    // the waits before the first word, as the platform reports them (sdk.generation / generation:phase)
    const wait = (document.getElementById('wait') || {}).value || ''
    const phase = async (p, ms) => { post({ type: 'generation', busy: true, phase: p }); log(`phase ${p}`); await new Promise((r) => setTimeout(r, ms)) }
    await phase('preparing', 500)
    if (wait === 'summarize') await phase('summarizing', 3000)
    if (wait === 'think' || wait === 'summarize') await phase('thinking', 3000)
    post({ type: 'generation', busy: true, phase: 'writing' })
    const step = Math.max(1, Math.ceil(text.length / chunks))
    for (let k = step; k < text.length; k += step) {
      post({ type: 'message.stream', id, content: text.slice(0, k) })
      await new Promise((r) => setTimeout(r, 60))
    }
    post({ type: 'message.done', id, content: text, serverId: String(9000 + state.seq) })
    post({ type: 'generation', busy: false, phase: 'idle' })
    state.messages.push({ id, role: 'ai', content: text, serverId: String(9000 + state.seq) })
    state.busy = false
  }
  function addInstant(text) {
    const id = `l${++state.seq}`
    state.messages.push({ id, role: 'ai', content: text, serverId: String(9000 + state.seq) })
    post({ type: 'message.new', message: { id, role: 'ai', content: text, serverId: String(9000 + state.seq) } })
    post({ type: 'message.done', id, content: text, serverId: String(9000 + state.seq) })
  }
  // 平台對話存檔的模擬（sdk.archive.*）：記在這一頁的記憶體，上限 20 段（含目前這段），行為照 Harbor：
  // 讀檔＝切過去、原本那段原樣留著；存檔＝從最新一則分叉、留下的那份取名；分叉＝複製到那一則為止；滿了回 LIMIT_REACHED。
  const ARCHIVE_LIMIT = 20
  function newArchive(messages, title = '') {
    const now = new Date().toISOString()
    return { id: 'c' + Math.random().toString(36).slice(2, 8), title, messages, createTime: now, lastUpdateTime: now }
  }
  const currentArchive = () => state.archives.find((a) => a.id === state.current)
  function adoptArchive(a) {
    state.current = a.id
    state.messages = a.messages
    post({ type: 'conversation.switch', conversationId: a.id })
    post({ type: 'messages', messages: state.messages })
  }
  function archiveRequest(m) {
    const ok = (value) => post({ type: 'reply', reqId: m.reqId, ok: true, value })
    const fail = (code, data) => post({ type: 'reply', reqId: m.reqId, ok: false, error: { code, ...(data ? { data } : {}) } })
    const full = () => state.archives.length >= ARCHIVE_LIMIT
    const copy = (list) => list.map((x) => ({ ...x }))
    const [a0, a1] = m.args || []
    switch (m.op) {
      case 'archive.list':
        return ok({
          items: state.archives.slice().reverse().map((a) => ({ id: a.id, title: a.title, isCurrent: a.id === state.current, messageCount: a.messages.length, lastMessage: String(a.messages[a.messages.length - 1]?.content || '').replace(/\s+/g, ' ').slice(0, 80), createTime: a.createTime, lastUpdateTime: a.lastUpdateTime })),
          count: state.archives.length, limit: ARCHIVE_LIMIT,
        })
      case 'archive.save': {
        if (full()) return fail('LIMIT_REACHED', { count: state.archives.length, limit: ARCHIVE_LIMIT })
        const kept = currentArchive()
        if (a0) kept.title = String(a0)
        const next = newArchive(copy(kept.messages))
        state.archives.push(next)
        adoptArchive(next)
        return ok({ id: kept.id, current: next.id })
      }
      case 'archive.fork': {
        if (full()) return fail('LIMIT_REACHED', { count: state.archives.length, limit: ARCHIVE_LIMIT })
        const at = state.messages.findIndex((x) => x.serverId === String(a0))
        if (at < 0) return fail('INVALID_ARGS')
        const next = newArchive(copy(state.messages.slice(0, at + 1)))
        state.archives.push(next)
        adoptArchive(next)
        return ok({ id: next.id })
      }
      case 'archive.open': {
        const a = state.archives.find((x) => x.id === String(a0))
        if (!a) return fail('INVALID_ARGS')
        if (a.id !== state.current) adoptArchive(a)
        return ok()
      }
      case 'archive.new': {
        // sdk.archive.start(n) 送 archive.new [n]：從第 n 個開場另開新檔（0＝主開場、1 起＝替代開場），目前這段留在清單裡
        const n = a0 == null ? 0 : Number(a0)
        if (!Number.isInteger(n) || n < 0 || n >= state.openings.length) return fail('INVALID_ARGS')
        if (full()) return fail('LIMIT_REACHED', { count: state.archives.length, limit: ARCHIVE_LIMIT })
        const next = newArchive([{ id: 'greeting', role: 'ai', content: state.openings[n], serverId: null, state: 'done' }])
        state.archives.push(next)
        adoptArchive(next)
        return ok({ id: next.id })
      }
      case 'archive.rename': {
        const a = state.archives.find((x) => x.id === String(a0))
        if (!a) return fail('INVALID_ARGS')
        a.title = String(a1 || '')
        return ok()
      }
      case 'archive.remove': {
        const i = state.archives.findIndex((x) => x.id === String(a0))
        if (i < 0) return ok()
        const wasCurrent = state.archives[i].id === state.current
        state.archives.splice(i, 1)
        if (wasCurrent) adoptArchive(state.archives[state.archives.length - 1] || (state.archives.push(newArchive([{ id: 'greeting', role: 'ai', content: state.welcome, serverId: null, state: 'done' }])), state.archives[0]))
        return ok()
      }
      default:
        return fail('UNKNOWN_CAPABILITY')
    }
  }
  function switchConversation() {
    post({ type: 'conversation.switch' })
    state.messages = [{ id: 'greeting', role: 'ai', content: state.welcome, serverId: null, state: 'done' }]
    post({ type: 'messages', messages: state.messages })
  }
  async function reload() {
    await loadCard()
    state.helloed = false
    frame.src = frame.src.split('#')[0] + '#' + Date.now()
    frame.contentWindow.location.reload()
  }
  const bare = params.get('bare') === '1'
  if (bare) document.body.classList.add('bare')
  function resize() {
    if (bare) { post({ type: 'viewport', height: frame.clientHeight }); return }
    const [w, h] = document.getElementById('size').value.split('x').map(Number)
    frame.style.width = w + 'px'; frame.style.height = h + 'px'
    post({ type: 'viewport', height: h })
  }
  if (bare) window.addEventListener('resize', resize)
  // story-first 審計：首屏裡劇情文字佔多少、可互動元素幾個、第一個選擇是否在首屏、有沒有自由輸入。
  const storyFirst = () => {
    const doc = frame.contentDocument
    const win = frame.contentWindow
    const vh = win.innerHeight, vw = win.innerWidth
    const area = (el) => { const r = el.getBoundingClientRect(); const w = Math.max(0, Math.min(r.right, vw) - Math.max(r.left, 0)); const h = Math.max(0, Math.min(r.bottom, vh) - Math.max(r.top, 0)); return w * h }
    const visible = (el) => area(el) > 0
    const bodies = Array.from(doc.querySelectorAll('[data-chat="message-body"]'))
    const panels = Array.from(doc.querySelectorAll('.hr-status, .hr-choices, [data-slot="statusbar"] > *, .hr-dock, .hr-pinned'))
    const textArea = bodies.reduce((a, el) => {
      // 正文面積 = 氣泡面積 − 氣泡內面板面積
      const inner = Array.from(el.querySelectorAll('.hr-status, .hr-choices')).reduce((b, p) => b + area(p), 0)
      return a + Math.max(0, area(el) - inner)
    }, 0)
    const uiArea = panels.reduce((a, el) => a + area(el), 0)
    const interactive = Array.from(doc.querySelectorAll('button, [onclick], a[href], input, select')).filter((el) => visible(el) && !el.closest('[data-chat="composer"], [data-chat="header"]')).length
    const firstChoice = doc.querySelector('.hr-choice, .prologue-content')
    const composer = doc.querySelector('[data-chat="input"], textarea')
    return {
      viewport: `${vw}x${vh}`,
      storyTextShare: Math.round((textArea / (vw * vh)) * 1000) / 10,
      uiShare: Math.round((uiArea / (vw * vh)) * 1000) / 10,
      interactiveAboveFold: interactive,
      firstChoiceAboveFold: !!(firstChoice && visible(firstChoice)),
      freeInputVisible: !!(composer && visible(composer)),
      rulesOff: state.rulesOff,
    }
  }
  const snapshot = () => {
    const doc = frame.contentDocument
    return {
      storyFirst: storyFirst(),
      composerValue: (() => { const el = doc.querySelector('[data-chat="input"], textarea'); return el ? el.value : null })(),
      bubbles: doc.querySelectorAll('[data-chat="message"]').length,
      statusPanels: doc.querySelectorAll('.hr-status--done').length,
      rawPanels: doc.querySelectorAll('.hr-status--raw:not(.hr-status--done)').length,
      stage: doc.querySelector('[data-chat="author-stage"]')?.getAttribute('data-stage'),
      theme: doc.querySelector('[data-chat="root"]')?.getAttribute('data-theme'),
      lastBody: doc.querySelectorAll('[data-chat="message-body"]')[doc.querySelectorAll('[data-chat="message-body"]').length - 1]?.innerHTML.slice(0, 400),
      logTail: Array.from(logEl.children).slice(-10).map((d) => d.textContent),
    }
  }

  document.getElementById('size').addEventListener('change', resize)
  document.getElementById('theme').addEventListener('change', () => post({ type: 'theme', theme: document.getElementById('theme').value }))
  document.getElementById('chrome').addEventListener('change', reload)
  document.getElementById('reload').addEventListener('click', reload)
  document.getElementById('switch').addEventListener('click', switchConversation)
  document.getElementById('rerun').addEventListener('click', reload)
  document.getElementById('stream').addEventListener('click', () => streamReply(currentReply()))
  document.getElementById('instant').addEventListener('click', () => addInstant(currentReply()))
  document.getElementById('user').addEventListener('click', () => addUser(document.getElementById('reply').value || '…'))
  document.getElementById('samples').addEventListener('change', () => { document.getElementById('reply').value = state.samples[Number(document.getElementById('samples').value)] || '' })
  function currentReply() { const t = document.getElementById('reply').value.trim(); if (t) return t; return state.samples[Number(document.getElementById('samples').value) || 0] || 'Hello.' }

  window.__preview = { reload, stream: streamReply, add: addInstant, addUser, switchConversation, snapshot, sent, state }
  loadCard().then(() => { resize(); if (state.samples.length) document.getElementById('reply').value = state.samples[0]; cardLoaded() })
})()
