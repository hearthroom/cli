export const LOCALES = ["en", "zh-Hant", "zh-Hans", "ja", "ko"] as const;
export type Locale = (typeof LOCALES)[number];

export function localePath(locale: Locale, path: string): string {
  return locale === "en" ? path : `/${locale}${path}`;
}

/**
 * Landing copy. Rules: one job per block, one sentence per block, the user's
 * action first, no numbers (they live in the manual), nothing that does not
 * change the decision to install or to hand the tool to an agent.
 */
export type Strings = {
  skip: string;
  navLabel: string;
  language: string;
  theme: string;
  viewMarkdown: string;
  nav: { manual: string; guides: string; releases: string; developers: string };
  hero: { title: string; lead: string };
  install: {
    os: { mac: string; linux: string; windows: string };
    copy: string; copied: string;
    other: string;                 // link text to the install guide
    agent: string;                 // "Using an agent? Give it …"
    version: (v: string) => string;
  };
  terminal: { pause: string; play: string; all: string };
  scenes: Record<"import" | "push" | "play" | "pull" | "search" | "media", string>;
  props: { title: string; text: string; link: string; href: string }[];
  again: { title: string; text: string };
};

const en: Strings = {
  skip: "Skip to content", navLabel: "Site", language: "Language", theme: "Toggle light and dark", viewMarkdown: "View as Markdown",
  nav: { manual: "Manual", guides: "Guides", releases: "Releases", developers: "Developer docs" },
  hero: { title: "Write cards from the command line.", lead: "Hearthroom for your terminal and your AI agent. Free and open source." },
  install: {
    os: { mac: "macOS", linux: "Linux", windows: "Windows" }, copy: "Copy", copied: "Copied",
    other: "Other ways to install", agent: "Using an agent? Give it", version: (v) => `Latest release ${v}`,
  },
  terminal: { pause: "Pause animation", play: "Play animation", all: "View all commands" },
  scenes: {
    import: "Turn a SillyTavern or MMD card into a folder of files.",
    push: "Push the folder to a private trial card and get the validation report.",
    play: "Send a turn and read the reply without leaving the shell.",
    pull: "Pull any card you own into files you can keep in git.",
    search: "Browse the community board.",
    media: "Manage your media library; the CLI names the card that still uses an image.",
  },
  props: [
    { title: "Cards are files", text: "Markdown for the writing, JSON for the fields, a folder you can diff, version and hand to anyone.", link: "The card folder format", href: "/guides/card-folder/" },
    { title: "Made for agents", text: "Every command speaks JSON, sign-in works without a browser, and nothing spends credits without an explicit flag.", link: "Using it with agents", href: "/guides/agents/" },
    { title: "Try before you keep", text: "Pushes land on a private trial card you can play at once; one flag turns it into a card that stays.", link: "Trial cards and real cards", href: "/guides/trial-cards/" },
  ],
  again: { title: "Try Hearthroom on the command line", text: "Install in one line, then hand the manual to your agent." },
};

const zhHant: Strings = {
  skip: "跳到主要內容", navLabel: "網站", language: "語言", theme: "切換深淺色", viewMarkdown: "以 Markdown 檢視",
  nav: { manual: "手冊", guides: "指南", releases: "版本", developers: "開發者文件" },
  hero: { title: "在命令列裡寫卡。", lead: "給你的終端機、也給你的 AI agent 用的 Hearthroom。免費、開源。" },
  install: {
    os: { mac: "macOS", linux: "Linux", windows: "Windows" }, copy: "複製", copied: "已複製",
    other: "其他安裝方式", agent: "帶著 agent 用？把這個給它", version: (v) => `最新版本 ${v}`,
  },
  terminal: { pause: "暫停動畫", play: "播放動畫", all: "查看全部指令" },
  scenes: {
    import: "把 SillyTavern 或魅魔島的卡變成一個純文字資料夾。",
    push: "把資料夾推到私有試玩卡，並拿到驗證報告。",
    play: "不離開終端機就送一句、讀回覆。",
    pull: "把你的任何一張卡拉成可以放進 git 的檔案。",
    search: "逛社群榜單。",
    media: "管理媒體庫；圖還被哪張卡用著，CLI 會直接說。",
  },
  props: [
    { title: "卡就是檔案", text: "文字用 Markdown、欄位用 JSON，一個能 diff、能進版控、能交給任何人的資料夾。", link: "卡片資料夾格式", href: "/guides/card-folder/" },
    { title: "為 agent 而做", text: "每個指令都講 JSON，不用瀏覽器也能登入，沒有明確旗標不會花任何點數。", link: "和 agent 一起用", href: "/guides/agents/" },
    { title: "先試，再留", text: "推送落在可以立刻試玩的私有試玩卡；一個旗標就把它變成留下來的正式卡。", link: "試玩卡與正式卡", href: "/guides/trial-cards/" },
  ],
  again: { title: "在命令列試試 Hearthroom", text: "一行安裝，然後把手冊交給你的 agent。" },
};

const zhHans: Strings = {
  skip: "跳到主要内容", navLabel: "网站", language: "语言", theme: "切换深浅色", viewMarkdown: "以 Markdown 查看",
  nav: { manual: "手册", guides: "指南", releases: "版本", developers: "开发者文档" },
  hero: { title: "在命令行里写卡。", lead: "给你的终端、也给你的 AI agent 用的 Hearthroom。免费、开源。" },
  install: {
    os: { mac: "macOS", linux: "Linux", windows: "Windows" }, copy: "复制", copied: "已复制",
    other: "其他安装方式", agent: "带着 agent 用？把这个给它", version: (v) => `最新版本 ${v}`,
  },
  terminal: { pause: "暂停动画", play: "播放动画", all: "查看全部命令" },
  scenes: {
    import: "把 SillyTavern 或魅魔岛的卡变成一个纯文本文件夹。",
    push: "把文件夹推到私有试玩卡，并拿到验证报告。",
    play: "不离开终端就发一句、读回复。",
    pull: "把你的任何一张卡拉成可以放进 git 的文件。",
    search: "逛社区榜单。",
    media: "管理媒体库；图还被哪张卡用着，CLI 会直接说。",
  },
  props: [
    { title: "卡就是文件", text: "文字用 Markdown、字段用 JSON，一个能 diff、能进版本控制、能交给任何人的文件夹。", link: "卡片文件夹格式", href: "/guides/card-folder/" },
    { title: "为 agent 而做", text: "每个命令都讲 JSON，不用浏览器也能登录，没有明确参数不会花任何点数。", link: "和 agent 一起用", href: "/guides/agents/" },
    { title: "先试，再留", text: "推送落在可以立刻试玩的私有试玩卡；一个参数就把它变成留下来的正式卡。", link: "试玩卡与正式卡", href: "/guides/trial-cards/" },
  ],
  again: { title: "在命令行试试 Hearthroom", text: "一行安装，然后把手册交给你的 agent。" },
};

const ja: Strings = {
  skip: "本文へスキップ", navLabel: "サイト", language: "言語", theme: "ライト／ダーク切替", viewMarkdown: "Markdown で表示",
  nav: { manual: "マニュアル", guides: "ガイド", releases: "リリース", developers: "開発者ドキュメント" },
  hero: { title: "コマンドラインでカードを書く。", lead: "ターミナルと AI エージェントのための Hearthroom。無料でオープンソース。" },
  install: {
    os: { mac: "macOS", linux: "Linux", windows: "Windows" }, copy: "コピー", copied: "コピー済み",
    other: "その他のインストール方法", agent: "エージェントと使うなら、これを渡す", version: (v) => `最新リリース ${v}`,
  },
  terminal: { pause: "アニメーションを停止", play: "アニメーションを再生", all: "すべてのコマンドを見る" },
  scenes: {
    import: "SillyTavern や MMD のカードをプレーンテキストのフォルダにする。",
    push: "フォルダをプライベートな試遊カードにプッシュし、検証レポートを受け取る。",
    play: "シェルを離れずに一言送り、返答を読む。",
    pull: "所有するカードを git に入れられるファイルとして取り出す。",
    search: "コミュニティのボードを見る。",
    media: "メディアライブラリを管理する。画像を使っているカードは CLI が名指しする。",
  },
  props: [
    { title: "カードはファイル", text: "文章は Markdown、項目は JSON。diff でき、バージョン管理でき、誰にでも渡せるフォルダ。", link: "カードフォルダ形式", href: "/guides/card-folder/" },
    { title: "エージェントのために", text: "全コマンドが JSON を話し、ブラウザなしでサインインでき、明示フラグなしにクレジットは使われない。", link: "エージェントと使う", href: "/guides/agents/" },
    { title: "試してから、残す", text: "プッシュはすぐ遊べるプライベートな試遊カードに届く。フラグひとつで残るカードになる。", link: "試遊カードと本カード", href: "/guides/trial-cards/" },
  ],
  again: { title: "コマンドラインで Hearthroom を試す", text: "一行でインストールして、マニュアルをエージェントに渡す。" },
};

const ko: Strings = {
  skip: "본문으로 건너뛰기", navLabel: "사이트", language: "언어", theme: "라이트/다크 전환", viewMarkdown: "Markdown으로 보기",
  nav: { manual: "매뉴얼", guides: "가이드", releases: "릴리스", developers: "개발자 문서" },
  hero: { title: "명령줄에서 카드를 쓰세요.", lead: "터미널과 AI 에이전트를 위한 Hearthroom. 무료, 오픈 소스." },
  install: {
    os: { mac: "macOS", linux: "Linux", windows: "Windows" }, copy: "복사", copied: "복사됨",
    other: "다른 설치 방법", agent: "에이전트와 함께 쓴다면 이걸 건네세요", version: (v) => `최신 릴리스 ${v}`,
  },
  terminal: { pause: "애니메이션 일시정지", play: "애니메이션 재생", all: "모든 명령 보기" },
  scenes: {
    import: "SillyTavern이나 MMD 카드를 일반 텍스트 폴더로 바꿉니다.",
    push: "폴더를 비공개 시험 카드에 푸시하고 검증 보고서를 받습니다.",
    play: "셸을 떠나지 않고 한 마디 보내고 답을 읽습니다.",
    pull: "소유한 카드를 git에 넣을 수 있는 파일로 받습니다.",
    search: "커뮤니티 보드를 둘러봅니다.",
    media: "미디어 라이브러리를 관리합니다. 이미지를 쓰는 카드가 있으면 CLI가 이름을 말합니다.",
  },
  props: [
    { title: "카드는 파일", text: "글은 Markdown, 항목은 JSON. diff 하고, 버전 관리하고, 누구에게나 건넬 수 있는 폴더.", link: "카드 폴더 형식", href: "/guides/card-folder/" },
    { title: "에이전트를 위해", text: "모든 명령이 JSON을 말하고, 브라우저 없이 로그인하며, 명시적 플래그 없이는 크레딧을 쓰지 않습니다.", link: "에이전트와 함께 쓰기", href: "/guides/agents/" },
    { title: "먼저 시험, 그다음 보관", text: "푸시는 바로 플레이할 수 있는 비공개 시험 카드에 도착합니다. 플래그 하나면 남는 카드가 됩니다.", link: "시험 카드와 실제 카드", href: "/guides/trial-cards/" },
  ],
  again: { title: "명령줄에서 Hearthroom을 써 보세요", text: "한 줄로 설치하고, 매뉴얼을 에이전트에게 건네세요." },
};

const table: Record<Locale, Strings> = { en, "zh-Hant": zhHant, "zh-Hans": zhHans, ja, ko };
export function t(locale: Locale): Strings {
  return table[locale] ?? en;
}
