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
    capabilities: { saves: true, edit: false, send: true },
    composer: true,
    chrome: document.getElementById('chrome').value === 'host' ? 'host' : 'shell',
    saves: {},
    // 殼的 debug 訊息（sdk.debug.log、腳本錯誤）不開面板也會轉送到這一頁的記錄；?debug=1 才把殼內面板與幾何疊層打開。
    debug: params.get('debug') === '1',
    viewportHeight: frame.clientHeight,
  })

  const sent = []
  window.addEventListener('message', (ev) => {
    if (ev.source !== frame.contentWindow || !ev.data || ev.data.ms !== PROTOCOL) return
    const m = ev.data
    sent.push(m)
    if (m.type === 'ready-shell' && !state.helloed) {
      state.helloed = true
      post({ type: 'hello', config: hello() })
      state.messages = [{ id: 'greeting', role: 'ai', content: state.welcome, serverId: null, state: 'done' }]
      post({ type: 'messages', messages: state.messages })
      post({ type: 'history', more: false, loading: false })
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
    post({ type: 'generation', busy: true })
    const step = Math.max(1, Math.ceil(text.length / chunks))
    for (let k = step; k < text.length; k += step) {
      post({ type: 'message.stream', id, content: text.slice(0, k) })
      await new Promise((r) => setTimeout(r, 60))
    }
    post({ type: 'message.done', id, content: text, serverId: String(9000 + state.seq) })
    post({ type: 'generation', busy: false })
    state.messages.push({ id, role: 'ai', content: text, serverId: String(9000 + state.seq) })
    state.busy = false
  }
  function addInstant(text) {
    const id = `l${++state.seq}`
    state.messages.push({ id, role: 'ai', content: text, serverId: String(9000 + state.seq) })
    post({ type: 'message.new', message: { id, role: 'ai', content: text, serverId: String(9000 + state.seq) } })
    post({ type: 'message.done', id, content: text, serverId: String(9000 + state.seq) })
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
  loadCard().then(() => { resize(); if (state.samples.length) document.getElementById('reply').value = state.samples[0] })
})()
