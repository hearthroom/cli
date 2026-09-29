export const LOCALES = ["en", "zh-Hant", "zh-Hans", "ja", "ko"] as const;
export type Locale = (typeof LOCALES)[number];

export function localePath(locale: Locale, path: string): string {
  return locale === "en" ? path : `/${locale}${path}`;
}

type Strings = {
  skip: string;
  navLabel: string;
  language: string;
  theme: string;
  viewMarkdown: string;
  nav: { manual: string; guides: string; releases: string; developers: string };
  hero: { title: string; lead: string; install: string; manual: string; version: (v: string) => string };
  install: { title: string; mac: string; linux: string; windows: string; verify: string; upgrade: string; source: string; copy: string; copied: string };
  features: { title: string; items: { cmd: string; title: string; text: string }[] };
  props: { title: string; text: string }[];
  agents: { title: string; text: string; link: string };
};

const en: Strings = {
  skip: "Skip to content",
  navLabel: "Site",
  language: "Language",
  theme: "Toggle light and dark",
  viewMarkdown: "View as Markdown",
  nav: { manual: "Manual", guides: "Guides", releases: "Releases", developers: "Developer docs" },
  hero: {
    title: "Your cards, from the terminal.",
    lead: "hearthroom brings Hearthroom to the command line: write character cards as plain files, push them to try, validate, import from other tools, and let your AI agent do the same. Free and open source.",
    install: "Install",
    manual: "Read the manual",
    version: (v) => `Latest release ${v}`,
  },
  install: {
    title: "Install",
    mac: "macOS", linux: "Linux", windows: "Windows",
    verify: "The script verifies the release checksum and sets up tab completion. Prefer a manual download? Every build is on GitHub Releases with a provenance attestation.",
    upgrade: "Later: hearthroom upgrade",
    source: "From source (Go 1.26+): go install github.com/hearthroom/cli/cmd/hearthroom@latest",
    copy: "Copy", copied: "Copied",
  },
  features: {
    title: "Your whole authoring loop",
    items: [
      { cmd: "hearthroom card import", title: "Bring cards you already have", text: "SillyTavern PNG, JSON and CHARX, or an MMD three-file set, become a folder of plain files. Anything that had no place to go is listed, never dropped." },
      { cmd: "hearthroom card push --validate", title: "Push and hear back at once", text: "Assets upload once, only changed sections travel, and the provider's pre-publish report comes back with the exact limits it enforces. The default target is a private trial card; --create keeps it." },
      { cmd: "hearthroom play", title: "Try it without leaving the shell", text: "Start or resume a conversation, read history, send a turn and stream the reply. Only sending spends credits, and only with --allow-spend." },
      { cmd: "hearthroom card pull", title: "Everything is files", text: "Pull any card you own into a folder, keep it in git, hand it to an agent, push it back. The format is versioned and documented." },
      { cmd: "hearthroom search", title: "Browse the community", text: "Search the board, look up tags and authors, view a card, all without signing in." },
      { cmd: "hearthroom media rm", title: "Manage your library", text: "Upload, list and delete media. When a card still uses an image, the CLI tells you which one." },
    ],
  },
  props: [
    { title: "Made for agents too", text: "Every command takes --json, sign-in works without a browser through HEARTHROOM_TOKEN, and paid actions need an explicit flag. Claude Code or Codex can run the whole loop." },
    { title: "The manual is the binary", text: "Every page under /manual is generated from the CLI's own help for that release, so the site never drifts from --help. Agents get the same text at /llms-full.txt." },
    { title: "One install, everywhere", text: "A single static binary for Linux, macOS and Windows on amd64 and arm64. Checksums, build provenance and self-update included." },
  ],
  agents: { title: "Working with an AI agent?", text: "Point it at the Markdown manual and let it import, push, validate and play. Spending stays opt-in.", link: "Guide: using it with agents" },
};

const zhHant: Strings = {
  skip: "跳到主要內容", navLabel: "網站", language: "語言", theme: "切換深淺色", viewMarkdown: "以 Markdown 檢視",
  nav: { manual: "手冊", guides: "指南", releases: "版本", developers: "開發者文件" },
  hero: {
    title: "在終端機裡寫你的卡。",
    lead: "hearthroom 把 Hearthroom 帶進命令列：用純文字檔寫角色卡、推上去試玩、驗證、從其他工具匯入，你的 AI agent 也能照著做。免費、開源。",
    install: "安裝", manual: "閱讀手冊", version: (v) => `最新版本 ${v}`,
  },
  install: {
    title: "安裝", mac: "macOS", linux: "Linux", windows: "Windows",
    verify: "安裝腳本會驗證發行檔的校驗和，並自動設定 Tab 補全。想自己下載？每個版本都在 GitHub Releases，附來源證明。",
    upgrade: "之後更新：hearthroom upgrade",
    source: "從原始碼安裝（Go 1.26+）：go install github.com/hearthroom/cli/cmd/hearthroom@latest",
    copy: "複製", copied: "已複製",
  },
  features: {
    title: "整個寫卡迴圈都在這裡",
    items: [
      { cmd: "hearthroom card import", title: "把手上的卡帶過來", text: "SillyTavern 的 PNG、JSON、CHARX，或魅魔島的三件套，都會變成一個純文字檔資料夾。放不下的欄位會逐條列出，不會默默丟掉。" },
      { cmd: "hearthroom card push --validate", title: "推上去，馬上收到回報", text: "素材只上傳一次、只送有變的段落，服務商的發布前檢查連同它實際執行的字數上限一起回來。預設推到私有試玩卡；--create 才會真正建卡。" },
      { cmd: "hearthroom play", title: "不用離開終端機就能試", text: "開啟或接續對話、看歷史、送一句並串流回覆。只有送訊息會花點數，而且要加 --allow-spend。" },
      { cmd: "hearthroom card pull", title: "一切都是檔案", text: "把你的任何一張卡拉成資料夾，放進 git、交給 agent、再推回去。格式有版本、有文件。" },
      { cmd: "hearthroom search", title: "逛社群", text: "搜榜單、查標籤與作者、看一張卡，不用登入。" },
      { cmd: "hearthroom media rm", title: "管理你的媒體庫", text: "上傳、列出、刪除媒體。圖還被某張卡用著時，CLI 會告訴你是哪一張。" },
    ],
  },
  props: [
    { title: "也是為 agent 做的", text: "每個指令都有 --json，透過 HEARTHROOM_TOKEN 不用瀏覽器就能登入，付費動作要明確加旗標。Claude Code 或 Codex 可以跑完整個迴圈。" },
    { title: "手冊就是程式本身", text: "/manual 底下每一頁都從該版本 CLI 自己的 --help 產生，網站永遠不會和程式脫節。agent 在 /llms-full.txt 拿到同一份文字。" },
    { title: "一次安裝，到處能用", text: "Linux、macOS、Windows 的 amd64 與 arm64 各一個靜態二進位。校驗和、來源證明、自我更新都有。" },
  ],
  agents: { title: "和 AI agent 一起用？", text: "把 Markdown 手冊丟給它，讓它匯入、推送、驗證、試玩。花錢仍然要你點頭。", link: "指南：和 agent 一起用" },
};

const zhHans: Strings = {
  skip: "跳到主要内容", navLabel: "网站", language: "语言", theme: "切换深浅色", viewMarkdown: "以 Markdown 查看",
  nav: { manual: "手册", guides: "指南", releases: "版本", developers: "开发者文档" },
  hero: {
    title: "在终端里写你的卡。",
    lead: "hearthroom 把 Hearthroom 带进命令行：用纯文本文件写角色卡、推上去试玩、验证、从其他工具导入，你的 AI agent 也能照着做。免费、开源。",
    install: "安装", manual: "阅读手册", version: (v) => `最新版本 ${v}`,
  },
  install: {
    title: "安装", mac: "macOS", linux: "Linux", windows: "Windows",
    verify: "安装脚本会校验发行文件的校验和，并自动设置 Tab 补全。想自己下载？每个版本都在 GitHub Releases，附来源证明。",
    upgrade: "之后更新：hearthroom upgrade",
    source: "从源码安装（Go 1.26+）：go install github.com/hearthroom/cli/cmd/hearthroom@latest",
    copy: "复制", copied: "已复制",
  },
  features: {
    title: "整个写卡循环都在这里",
    items: [
      { cmd: "hearthroom card import", title: "把手上的卡带过来", text: "SillyTavern 的 PNG、JSON、CHARX，或魅魔岛的三件套，都会变成一个纯文本文件夹。放不下的字段会逐条列出，不会默默丢掉。" },
      { cmd: "hearthroom card push --validate", title: "推上去，马上收到反馈", text: "素材只上传一次、只发送有变化的段落，服务商的发布前检查连同它实际执行的字数上限一起返回。默认推到私有试玩卡；--create 才会真正建卡。" },
      { cmd: "hearthroom play", title: "不用离开终端就能试", text: "开启或继续对话、看历史、发一句并流式接收回复。只有发消息会花点数，而且要加 --allow-spend。" },
      { cmd: "hearthroom card pull", title: "一切都是文件", text: "把你的任何一张卡拉成文件夹，放进 git、交给 agent、再推回去。格式有版本、有文档。" },
      { cmd: "hearthroom search", title: "逛社区", text: "搜榜单、查标签与作者、看一张卡，不用登录。" },
      { cmd: "hearthroom media rm", title: "管理你的媒体库", text: "上传、列出、删除媒体。图还被某张卡用着时，CLI 会告诉你是哪一张。" },
    ],
  },
  props: [
    { title: "也是为 agent 做的", text: "每个命令都有 --json，通过 HEARTHROOM_TOKEN 不用浏览器就能登录，付费动作要明确加参数。Claude Code 或 Codex 可以跑完整个循环。" },
    { title: "手册就是程序本身", text: "/manual 下每一页都从该版本 CLI 自己的 --help 生成，网站永远不会和程序脱节。agent 在 /llms-full.txt 拿到同一份文本。" },
    { title: "一次安装，到处能用", text: "Linux、macOS、Windows 的 amd64 与 arm64 各一个静态二进制。校验和、来源证明、自我更新都有。" },
  ],
  agents: { title: "和 AI agent 一起用？", text: "把 Markdown 手册交给它，让它导入、推送、验证、试玩。花钱仍然要你点头。", link: "指南：和 agent 一起用" },
};

const ja: Strings = {
  skip: "本文へスキップ", navLabel: "サイト", language: "言語", theme: "ライト／ダーク切替", viewMarkdown: "Markdown で表示",
  nav: { manual: "マニュアル", guides: "ガイド", releases: "リリース", developers: "開発者ドキュメント" },
  hero: {
    title: "ターミナルから、あなたのカードを。",
    lead: "hearthroom は Hearthroom をコマンドラインに持ち込みます。キャラクターカードをプレーンテキストで書き、プッシュして試し、検証し、他のツールから取り込む。AI エージェントも同じことができます。無料でオープンソース。",
    install: "インストール", manual: "マニュアルを読む", version: (v) => `最新リリース ${v}`,
  },
  install: {
    title: "インストール", mac: "macOS", linux: "Linux", windows: "Windows",
    verify: "スクリプトはリリースのチェックサムを検証し、タブ補完も設定します。手動でダウンロードするなら GitHub Releases に全ビルドと来歴証明があります。",
    upgrade: "更新は hearthroom upgrade",
    source: "ソースから（Go 1.26+）：go install github.com/hearthroom/cli/cmd/hearthroom@latest",
    copy: "コピー", copied: "コピー済み",
  },
  features: {
    title: "執筆のループをまるごと",
    items: [
      { cmd: "hearthroom card import", title: "手元のカードを持ってくる", text: "SillyTavern の PNG・JSON・CHARX、あるいは MMD の三点セットが、プレーンテキストのフォルダになります。置き場のない項目は一覧され、黙って捨てられません。" },
      { cmd: "hearthroom card push --validate", title: "プッシュすればすぐ結果が返る", text: "素材は一度だけアップロード、変更のあった区分だけ送信。プロバイダーの公開前チェックが、実際に適用される文字数上限とともに返ります。既定はプライベートな試遊カード、--create で本当のカードに。" },
      { cmd: "hearthroom play", title: "シェルを離れずに試す", text: "会話を開始・再開し、履歴を読み、一言送ってストリーミングで返答を受け取る。クレジットを使うのは送信だけ、それも --allow-spend を付けたときだけ。" },
      { cmd: "hearthroom card pull", title: "すべてがファイル", text: "所有するカードをフォルダに取り出し、git に入れ、エージェントに渡し、プッシュし戻す。形式はバージョン管理され、文書化されています。" },
      { cmd: "hearthroom search", title: "コミュニティを見る", text: "ボード検索、タグと作者の照会、カードの閲覧。サインインは不要です。" },
      { cmd: "hearthroom media rm", title: "ライブラリを管理", text: "メディアのアップロード・一覧・削除。カードがまだ画像を使っていれば、CLI がどのカードかを教えます。" },
    ],
  },
  props: [
    { title: "エージェントのためにも", text: "全コマンドに --json、HEARTHROOM_TOKEN でブラウザなしにサインイン、課金操作には明示フラグ。Claude Code や Codex がループ全体を回せます。" },
    { title: "マニュアルはバイナリそのもの", text: "/manual の各ページはそのリリースの CLI 自身の --help から生成され、サイトが --help とずれることはありません。エージェントは /llms-full.txt で同じ文を得ます。" },
    { title: "一度のインストールでどこでも", text: "Linux・macOS・Windows の amd64 と arm64 向け単一静的バイナリ。チェックサム、来歴証明、自己更新つき。" },
  ],
  agents: { title: "AI エージェントと一緒に使う？", text: "Markdown のマニュアルを渡せば、取り込み・プッシュ・検証・試遊まで任せられます。課金はオプトインのまま。", link: "ガイド：エージェントと使う" },
};

const ko: Strings = {
  skip: "본문으로 건너뛰기", navLabel: "사이트", language: "언어", theme: "라이트/다크 전환", viewMarkdown: "Markdown으로 보기",
  nav: { manual: "매뉴얼", guides: "가이드", releases: "릴리스", developers: "개발자 문서" },
  hero: {
    title: "터미널에서, 당신의 카드를.",
    lead: "hearthroom은 Hearthroom을 명령줄로 가져옵니다. 캐릭터 카드를 일반 텍스트 파일로 쓰고, 푸시해서 시험하고, 검증하고, 다른 도구에서 가져오세요. AI 에이전트도 똑같이 할 수 있습니다. 무료, 오픈 소스.",
    install: "설치", manual: "매뉴얼 읽기", version: (v) => `최신 릴리스 ${v}`,
  },
  install: {
    title: "설치", mac: "macOS", linux: "Linux", windows: "Windows",
    verify: "스크립트는 릴리스 체크섬을 검증하고 탭 완성도 설정합니다. 직접 내려받으려면 GitHub Releases에 모든 빌드와 출처 증명이 있습니다.",
    upgrade: "나중에 업데이트: hearthroom upgrade",
    source: "소스에서 설치(Go 1.26+): go install github.com/hearthroom/cli/cmd/hearthroom@latest",
    copy: "복사", copied: "복사됨",
  },
  features: {
    title: "집필 루프 전체가 여기에",
    items: [
      { cmd: "hearthroom card import", title: "가진 카드를 가져오기", text: "SillyTavern의 PNG·JSON·CHARX, 또는 MMD 세 파일 세트가 일반 텍스트 폴더가 됩니다. 자리를 찾지 못한 항목은 하나씩 나열되며 조용히 버려지지 않습니다." },
      { cmd: "hearthroom card push --validate", title: "푸시하면 바로 응답", text: "에셋은 한 번만 업로드되고 바뀐 섹션만 전송됩니다. 제공자의 게시 전 검사가 실제 적용되는 글자 수 제한과 함께 돌아옵니다. 기본은 비공개 시험 카드, --create로 실제 카드로." },
      { cmd: "hearthroom play", title: "셸을 떠나지 않고 시험", text: "대화를 시작하거나 이어가고, 기록을 읽고, 한 마디 보내면 답이 스트리밍됩니다. 크레딧은 전송할 때만, 그것도 --allow-spend를 붙일 때만 쓰입니다." },
      { cmd: "hearthroom card pull", title: "모든 것이 파일", text: "소유한 카드를 폴더로 받아 git에 넣고, 에이전트에 맡기고, 다시 푸시하세요. 형식은 버전이 있고 문서화되어 있습니다." },
      { cmd: "hearthroom search", title: "커뮤니티 둘러보기", text: "보드 검색, 태그와 작성자 조회, 카드 보기. 로그인이 필요 없습니다." },
      { cmd: "hearthroom media rm", title: "라이브러리 관리", text: "미디어 업로드·목록·삭제. 카드가 아직 이미지를 쓰고 있으면 CLI가 어느 카드인지 알려줍니다." },
    ],
  },
  props: [
    { title: "에이전트를 위해서도", text: "모든 명령에 --json, HEARTHROOM_TOKEN으로 브라우저 없이 로그인, 유료 동작에는 명시적 플래그. Claude Code나 Codex가 루프 전체를 돌릴 수 있습니다." },
    { title: "매뉴얼이 곧 바이너리", text: "/manual의 모든 페이지는 해당 릴리스 CLI의 --help에서 생성되어 사이트가 --help와 어긋나지 않습니다. 에이전트는 /llms-full.txt에서 같은 텍스트를 얻습니다." },
    { title: "한 번 설치, 어디서나", text: "Linux·macOS·Windows의 amd64와 arm64용 단일 정적 바이너리. 체크섬, 출처 증명, 자체 업데이트 포함." },
  ],
  agents: { title: "AI 에이전트와 함께 쓰나요?", text: "Markdown 매뉴얼을 건네면 가져오기·푸시·검증·시험까지 맡길 수 있습니다. 결제는 여전히 옵트인입니다.", link: "가이드: 에이전트와 함께 쓰기" },
};

const table: Record<Locale, Strings> = { en, "zh-Hant": zhHant, "zh-Hans": zhHans, ja, ko };
export function t(locale: Locale): Strings {
  return table[locale] ?? en;
}
