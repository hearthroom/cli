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
  const locale = params.get('locale') || 'zh-Hant'
  const PROTOCOL = 1
  const frame = document.getElementById('frame')
  const logEl = document.getElementById('log')
  const log = (s, cls) => { const d = document.createElement('div'); if (cls) d.className = cls; d.textContent = s; logEl.appendChild(d); logEl.scrollTop = logEl.scrollHeight }

  const cardBase = (params.get('card') || '/card/').replace(/\/?$/, '/')
  const state = { card: null, seq: 0, messages: [], helloed: false, busy: false, samples: [], inputValue: '' }
  let streamToken = 0
  let activeReply = null

  async function fetchText(name) {
    const r = await fetch(cardBase + name, { cache: 'no-store' })
    if (!r.ok) return null
    return await r.text()
  }
  // 規則裡的外鏈 <script src="assets/…">：上線後是素材庫的 https 網址，殼照跑；本機只能是 http，殼只認 https，會靜默跳過。
  // 這裡把它們讀出來內聯，放進最後一條永不命中的合成規則：殼「不論規則有沒有命中」都抽出腳本，所以照樣執行，
  // 順序也同線上（所有內聯傳統腳本之後、按規則順序）。差別是線上從素材庫非同步載入、可能晚到或載入失敗。
  // 與 internal/preview/check.go 的 assetScriptRe 同一條規則。
  const ASSET_SCRIPT_RE = /<script\b([^>]*?)\bsrc\s*=\s*(["'])assets\/([^"']+)\2([^>]*)>\s*<\/script\s*>/gi
  async function inlineAssetScripts(rules) {
    const out = []
    const seen = new Set()
    for (const r of rules) {
      if (!r.enabled) continue
      const found = []
      r.replace = r.replace.replace(ASSET_SCRIPT_RE, (_m, pre, _q, rel, post) => { found.push({ rel, module: /\btype\s*=\s*["']?module\b/i.test(pre + post) }); return '' })
      for (const f of found) {
        if (seen.has(f.rel)) continue
        seen.add(f.rel)
        const code = await fetchText('assets/' + f.rel)
        if (code == null) { log(`預覽：讀不到外部腳本 assets/${f.rel}`, 'warn'); continue }
        if (code.includes('```')) log(`預覽：assets/${f.rel} 含 \`\`\`，內聯後可能被當成程式碼圍欄而不執行`, 'warn')
        out.push(`<script${f.module ? ' type="module"' : ''}>${code.replace(/<\/script/gi, '<\\/script')}</script>`)
        log(`預覽：外部腳本 assets/${f.rel} 已內聯執行（線上由素材庫非同步載入）`)
      }
    }
    return out
  }
  async function loadCard() {
    const rulesText = await fetchText('rules.json')
    const rules = rulesText ? JSON.parse(rulesText) : { rules: [] }
    const cardCss = await fetchText('card.css')
    // 檔案存在（包括明確留空）才用正式掛點；rules=off 補出的空 payload 不改舊卡的殼。
    state.cardCssPresent = cardCss !== null
    state.inputValue = ''
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
    // targets（ai／user／system）限定規則作用的訊息；沒有就是每一則，玩家的句子也算。
    const raw = rulesOff ? [] : (rules.rules || []).map((r, i) => ({ id: r.id ?? i, name: r.name || '', find: r.find, replace: String(r.replace ?? ''), enabled: r.enabled !== false, ...(Array.isArray(r.targets) ? { targets: r.targets } : {}), ...(r.cssDialect ? { cssDialect: r.cssDialect } : {}) }))
    const inlined = await inlineAssetScripts(raw)
    state.card = {
      format: rules.cardFormat || 'mmd',
      rules: raw.map((r) => ({ ...r, replace: assets(r.replace) })),
      statusbar: rulesOff ? '' : assets(rules.mountTrigger || ''),
      ...(rulesOff ? { cardCss: '' } : cardCss !== null ? { cardCss: assets(cardCss) } : {}),
      ...(typeof rules.cardCssDialect === 'string' ? { cardCssDialect: rules.cardCssDialect } : {}),
    }
    if (inlined.length) state.card.rules.push({ id: '__preview_external__', name: 'preview: external scripts from assets/', find: '\u0000__never__', replace: inlined.join(''), enabled: true })
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
  const standardChrome = () => state.cardCssPresent && document.getElementById('chrome').value !== 'host'
  // 標準殼由宿主供應 --lt-canvas-*。離線宿主沿用殼自己的深淺調色盤，
  // 尺寸對齊標準畫布；var() 保留換色與作者 --chat-* 覆寫，不另注入作者 CSS。
  const themeVars = Object.fromEntries(Object.entries({
    bg: 'var(--chat-bg)', fg: 'var(--chat-text)', muted: 'var(--chat-text-muted)', line: 'var(--chat-border)', accent: 'var(--chat-accent)',
    font: '-apple-system, BlinkMacSystemFont, "Helvetica Neue", "PingFang TC", "Noto Sans TC", sans-serif',
    'font-size': '15px', 'line-height': 'normal',
    'header-bg': 'var(--chat-bg)', 'header-fg': 'var(--chat-text)', 'header-height': '52px', 'header-blur': '12px',
    'title-size': '15px', 'title-weight': '600', 'avatar-size': '30px', 'avatar-radius': '9999px',
    'name-size': '12px', 'name-color': 'var(--chat-text-muted)', 'msg-avatar-size': '32px',
    'bubble-ai-bg': 'var(--chat-bubble-ai-bg)', 'bubble-user-bg': 'var(--chat-bubble-user-bg)', 'bubble-fg': 'var(--chat-bubble-text)',
    'bubble-radius': '14px', 'bubble-padding': '10px 14px', 'bubble-max-width': '100%',
    'composer-bg': 'var(--chat-composer-bg)', 'composer-fg': 'var(--chat-composer-text)', 'composer-field-bg': 'var(--chat-input-bg)',
    'composer-border': 'var(--chat-border)', 'composer-radius': '18px', 'placeholder-color': 'var(--chat-input-placeholder)',
    'send-size': '32px', 'send-bg': 'var(--chat-surface)', 'send-fg': 'var(--chat-text)',
    'pill-bg': 'var(--chat-shortcut-bg)', 'pill-fg': 'var(--chat-shortcut-text)',
    'stop-bg': 'var(--chat-modal-bg)', 'stop-fg': 'var(--chat-modal-text)', 'stop-border': 'var(--chat-border)',
  }).map(([key, value]) => [`--lt-canvas-${key}`, value]))
  // 沿 Stage src/locale 的標準輸入區詞彙；語系選擇與 sandbox/strings.ts 一致。
  const COMPOSER_STRINGS = {
    'zh-Hant': {
      placeholder: '說點什麼⋯', stop: '停止', more: '更多', send: '送出', paste: '貼上', clear: '清空',
      model: '模型設定', assist: '不知道怎麼回？讓 AI 幫你起個頭', perTurn: '每輪點數',
    },
    'zh-Hans': {
      placeholder: '说点什么⋯', stop: '停止', more: '更多', send: '发送', paste: '粘贴', clear: '清空',
      model: '模型设置', assist: '不知道怎么回？让 AI 帮你起个头', perTurn: '每轮点数',
    },
    en: {
      placeholder: 'Say something…', stop: 'Stop', more: 'More', send: 'Send', paste: 'Paste', clear: 'Clear',
      model: 'Model', assist: 'Stuck? Let AI draft a reply', perTurn: 'Credits per turn',
    },
    ja: {
      placeholder: '何か話しかけて⋯', stop: '停止', more: 'その他', send: '送信', paste: '貼り付け', clear: 'クリア',
      model: 'モデル設定', assist: '返事に迷ったら AI に下書きしてもらう', perTurn: '1 ターンあたりのポイント',
    },
    ko: {
      placeholder: '무엇이든 말해 보세요⋯', stop: '중지', more: '더보기', send: '보내기', paste: '붙여넣기', clear: '지우기',
      model: '모델 설정', assist: '뭐라고 답할지 막히면 AI에게 초안을 맡기세요', perTurn: '1턴당 포인트',
    },
  }
  function composerStrings(key) {
    if (Object.hasOwn(COMPOSER_STRINGS, key)) return COMPOSER_STRINGS[key]
    const base = key.split(/[-_]/)[0]
    if (base === 'zh') return /hans|cn|sg/i.test(key) ? COMPOSER_STRINGS['zh-Hans'] : COMPOSER_STRINGS['zh-Hant']
    return Object.hasOwn(COMPOSER_STRINGS, base) ? COMPOSER_STRINGS[base] : COMPOSER_STRINGS.en
  }
  const chromeState = () => {
    const { placeholder, ...labels } = composerStrings(locale)
    return {
      header: {
        roleName: state.name, avatar: '', badge: '', backLabel: '', showBack: false,
        fullscreenSupported: false, favoriteSupported: false, commentsSupported: false,
      },
      composer: {
        placeholder, labels, sendState: state.busy ? 'pending' : state.inputValue.trim() ? 'send' : 'send-disabled',
        generating: state.busy, enterSends: true, shortcuts: [], moreOpen: false, moreItems: [],
        modelScore: '', assistEnabled: false, assistBusy: false, assistCost: '',
      },
    }
  }
  const syncChrome = () => { if (standardChrome()) post({ type: 'chrome', state: chromeState() }) }
  const hello = () => ({
    theme: document.getElementById('theme').value,
    locale,
    role: { name: state.name, avatarUrl: '' },
    user: { nickname: state.playerName, avatarUrl: '' },
    card: state.card,
    variants: null,
    capabilities: { saves: true, edit: false, send: true, archive: true },
    composer: true,
    chrome: document.getElementById('chrome').value === 'host' ? 'host' : 'shell',
    ...(standardChrome() ? { chromeState: chromeState(), themeVars } : {}),
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
    if (standardChrome() && m.type === 'input') {
      state.inputValue = String(m.value ?? '')
      syncChrome()
      return
    }
    if (standardChrome() && m.type === 'ui') {
      if (m.event === 'send' && !state.busy && state.inputValue.trim()) {
        const text = state.inputValue
        state.inputValue = ''
        post({ type: 'input', value: '' })
        addUser(text)
        const next = state.samples.length ? state.samples[state.seq % state.samples.length] : 'OK.'
        streamReply(next)
      } else if (m.event === 'stop') stopReply()
      return
    }
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
    const token = ++streamToken
    const id = `l${++state.seq}`
    activeReply = { id, role: 'ai', content: '', serverId: null }
    syncChrome()
    post({ type: 'message.new', message: { id, role: 'ai', content: '', serverId: null } })
    // the waits before the first word, as the platform reports them (sdk.generation / generation:phase)
    const wait = (document.getElementById('wait') || {}).value || ''
    const phase = async (p, ms) => { post({ type: 'generation', busy: true, phase: p }); log(`phase ${p}`); await new Promise((r) => setTimeout(r, ms)) }
    await phase('preparing', 500)
    if (token !== streamToken) return
    if (wait === 'summarize') { await phase('summarizing', 3000); if (token !== streamToken) return }
    if (wait === 'think' || wait === 'summarize') { await phase('thinking', 3000); if (token !== streamToken) return }
    post({ type: 'generation', busy: true, phase: 'writing' })
    const step = Math.max(1, Math.ceil(text.length / chunks))
    for (let k = step; k < text.length; k += step) {
      activeReply.content = text.slice(0, k)
      post({ type: 'message.stream', id, content: text.slice(0, k) })
      await new Promise((r) => setTimeout(r, 60))
      if (token !== streamToken) return
    }
    finishReply(text)
  }
  function finishReply(content) {
    if (!activeReply) return
    activeReply.content = content
    activeReply.serverId = String(9000 + state.seq)
    post({ type: 'message.done', id: activeReply.id, content, serverId: activeReply.serverId })
    post({ type: 'generation', busy: false, phase: 'idle' })
    state.messages.push(activeReply)
    activeReply = null
    state.busy = false
    syncChrome()
  }
  function stopReply() {
    if (!activeReply) return
    ++streamToken
    finishReply(activeReply.content)
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
    if (state.cardCssPresent) stopReply()
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
