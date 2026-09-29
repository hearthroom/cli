export const LOCALES = ["en", "zh-Hant", "zh-Hans", "ja", "ko"] as const;
export type Locale = (typeof LOCALES)[number];

export function localePath(locale: Locale, path: string): string {
  return locale === "en" ? path : `/${locale}${path}`;
}

export const LANG_NAMES: Record<Locale, string> = { en: "English", "zh-Hant": "繁體中文", "zh-Hans": "简体中文", ja: "日本語", ko: "한국어" };

/**
 * Landing copy, written per language rather than translated: each locale uses
 * the words the Hearthroom app itself uses (角色卡 / 試玩 / 世界書, キャラクター
 * カード / 試用カード, 캐릭터 카드 / 체험 카드). The register follows cli.github.com:
 * a plain noun heading, one plain sentence, one "Learn about … →" link.
 */
export type InstallMethod = "brew" | "sh" | "ps" | "scoop";
export type Strings = {
  skip: string;
  navLabel: string;
  language: string;
  theme: string;
  viewMarkdown: string;
  nav: { manual: string; guides: string; releases: string; developers: string };
  hero: { title: string; lead: string };
  install: {
    label: Record<InstallMethod, string>;           // summary text: "Install with Homebrew"
    method: { brewMac: string; shMac: string; shLinux: string; ps: string; scoop: string; download: string };
    copy: string; copied: string;
    instructions: string;                            // "View installation instructions"
    agent: string;                                   // "Working with an AI agent? Give it the manual:"
    version: (v: string) => string;
  };
  terminal: { pause: string; play: string; all: string };
  scenes: Record<"import" | "push" | "play" | "pull" | "search" | "media", string>;
  headline: string;
  props: { title: string; text: string; link: string; href: string }[];
  again: { title: string; text: string };
};

const en: Strings = {
  skip: "Skip to content", navLabel: "Site", language: "Language", theme: "Switch between light and dark", viewMarkdown: "View as Markdown",
  nav: { manual: "Manual", guides: "Guides", releases: "Releases", developers: "Developer docs" },
  hero: { title: "Take Hearthroom to the command line", lead: "Hearthroom CLI brings your character cards to the terminal, and to your AI agent. Free and open source." },
  install: {
    label: { brew: "Install with Homebrew", sh: "Install with the shell script", ps: "Install with PowerShell", scoop: "Install with Scoop" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — Shell script", shLinux: "Linux — Shell script", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "Download a binary" },
    copy: "Copy", copied: "Copied",
    instructions: "View installation instructions", agent: "Using an AI agent? Give it the manual:", version: (v) => `Latest release ${v}`,
  },
  terminal: { pause: "Pause animation", play: "Play animation", all: "View all Hearthroom CLI commands" },
  scenes: {
    import: "Import a SillyTavern or MMD card as a folder of files.",
    push: "Push a card to a private trial card and validate it.",
    play: "Play a turn without leaving the terminal.",
    pull: "Pull a card you own into files.",
    search: "Search the community board.",
    media: "Manage the images in your library.",
  },
  headline: "Write cards the way you write code.",
  props: [
    { title: "Write in your own editor", text: "A card is a folder of Markdown and JSON. Edit it in the editor you already use, keep every version in git, and hand it to anyone as files.", link: "Learn about the card folder", href: "/guides/card-folder/" },
    { title: "Built for AI agents", text: "Every command outputs JSON, sign-in works without a browser, and nothing spends credits without a flag.", link: "Use it with an agent", href: "/guides/agents/" },
    { title: "Play it before anyone sees it", text: "Every push lands on a private trial card you can play right away. Nothing goes public until you say so.", link: "Learn about trial cards", href: "/guides/trial-cards/" },
    { title: "Free and open source", text: "AGPL-3.0, a single binary, and nothing to sign up for beyond your Hearthroom account.", link: "Contribute on GitHub", href: "https://github.com/hearthroom/cli" },
  ],
  again: { title: "Try Hearthroom on the command line", text: "Hearthroom CLI brings your character cards to the terminal. Free and open source." },
};

const zhHant: Strings = {
  skip: "跳到主要內容", navLabel: "網站", language: "語言", theme: "切換深色與淺色", viewMarkdown: "以 Markdown 檢視",
  nav: { manual: "手冊", guides: "指南", releases: "版本", developers: "開發者文件" },
  hero: { title: "把 Hearthroom 帶進終端機", lead: "Hearthroom CLI 讓你在終端機裡管理角色卡，也能交給 AI Agent 代勞。免費、開源。" },
  install: {
    label: { brew: "用 Homebrew 安裝", sh: "用安裝腳本安裝", ps: "用 PowerShell 安裝", scoop: "用 Scoop 安裝" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — 安裝腳本", shLinux: "Linux — 安裝腳本", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "下載執行檔" },
    copy: "複製", copied: "已複製",
    instructions: "查看安裝說明", agent: "搭配 AI Agent 使用？把手冊交給它：", version: (v) => `最新版本 ${v}`,
  },
  terminal: { pause: "暫停動畫", play: "播放動畫", all: "查看全部指令" },
  scenes: {
    import: "把 SillyTavern 或 MMD 的角色卡匯入成一個資料夾。",
    push: "把角色卡推送到私人試玩卡，並檢查內容。",
    play: "不用離開終端機，就能試玩一回合。",
    pull: "把你的角色卡下載成檔案。",
    search: "搜尋社群榜單。",
    media: "管理資源庫裡的圖片。",
  },
  headline: "像寫程式一樣寫角色卡。",
  props: [
    { title: "用你自己的編輯器寫", text: "角色卡就是一個放 Markdown 和 JSON 的資料夾。用你習慣的編輯器改，每個版本都留在 git 裡，也能直接把檔案交給別人。", link: "認識卡片資料夾", href: "/guides/card-folder/" },
    { title: "AI Agent 也能直接操作", text: "每個指令都能輸出 JSON，登入不需要瀏覽器，沒有明確加上參數就不會扣點數。", link: "搭配 AI Agent 使用", href: "/guides/agents/" },
    { title: "先自己玩過，再給別人看", text: "每次推送都會先放到只有你看得到的試玩卡，馬上就能試玩；你不說，就不會公開。", link: "認識試玩卡", href: "/guides/trial-cards/" },
    { title: "免費、開源", text: "AGPL-3.0 授權，單一執行檔，只需要你的 Hearthroom 帳號。", link: "到 GitHub 參與開發", href: "https://github.com/hearthroom/cli" },
  ],
  again: { title: "在終端機裡試試 Hearthroom", text: "Hearthroom CLI 讓你在終端機裡管理角色卡。免費、開源。" },
};

const zhHans: Strings = {
  skip: "跳到主要内容", navLabel: "网站", language: "语言", theme: "切换深色与浅色", viewMarkdown: "以 Markdown 查看",
  nav: { manual: "手册", guides: "指南", releases: "版本", developers: "开发者文档" },
  hero: { title: "把 Hearthroom 带进终端", lead: "Hearthroom CLI 让你在终端里管理角色卡，也能交给 AI Agent 代劳。免费、开源。" },
  install: {
    label: { brew: "用 Homebrew 安装", sh: "用安装脚本安装", ps: "用 PowerShell 安装", scoop: "用 Scoop 安装" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — 安装脚本", shLinux: "Linux — 安装脚本", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "下载可执行文件" },
    copy: "复制", copied: "已复制",
    instructions: "查看安装说明", agent: "搭配 AI Agent 使用？把手册交给它：", version: (v) => `最新版本 ${v}`,
  },
  terminal: { pause: "暂停动画", play: "播放动画", all: "查看全部命令" },
  scenes: {
    import: "把 SillyTavern 或 MMD 的角色卡导入成一个文件夹。",
    push: "把角色卡推送到私人试玩卡，并检查内容。",
    play: "不用离开终端，就能试玩一回合。",
    pull: "把你的角色卡下载成文件。",
    search: "搜索社区榜单。",
    media: "管理资源库里的图片。",
  },
  headline: "像写代码一样写角色卡。",
  props: [
    { title: "用你自己的编辑器写", text: "角色卡就是一个放 Markdown 和 JSON 的文件夹。用你习惯的编辑器改，每个版本都留在 git 里，也能直接把文件交给别人。", link: "了解卡片文件夹", href: "/guides/card-folder/" },
    { title: "AI Agent 也能直接操作", text: "每个命令都能输出 JSON，登录不需要浏览器，没有明确加上参数就不会扣点数。", link: "搭配 AI Agent 使用", href: "/guides/agents/" },
    { title: "先自己玩过，再给别人看", text: "每次推送都会先放到只有你能看到的试玩卡，马上就能试玩；你不说，就不会公开。", link: "了解试玩卡", href: "/guides/trial-cards/" },
    { title: "免费、开源", text: "AGPL-3.0 许可，单个可执行文件，只需要你的 Hearthroom 账号。", link: "到 GitHub 参与开发", href: "https://github.com/hearthroom/cli" },
  ],
  again: { title: "在终端里试试 Hearthroom", text: "Hearthroom CLI 让你在终端里管理角色卡。免费、开源。" },
};

const ja: Strings = {
  skip: "本文へスキップ", navLabel: "サイト", language: "言語", theme: "ライトとダークを切り替える", viewMarkdown: "Markdown で表示",
  nav: { manual: "マニュアル", guides: "ガイド", releases: "リリース", developers: "開発者ドキュメント" },
  hero: { title: "Hearthroom をターミナルで", lead: "Hearthroom CLI は、キャラクターカードをターミナルと AI エージェントから扱えるようにします。無料でオープンソース。" },
  install: {
    label: { brew: "Homebrew でインストール", sh: "シェルスクリプトでインストール", ps: "PowerShell でインストール", scoop: "Scoop でインストール" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — シェルスクリプト", shLinux: "Linux — シェルスクリプト", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "バイナリをダウンロード" },
    copy: "コピー", copied: "コピー済み",
    instructions: "インストール手順を見る", agent: "AI エージェントと使うなら、マニュアルを渡してください：", version: (v) => `最新リリース ${v}`,
  },
  terminal: { pause: "アニメーションを停止", play: "アニメーションを再生", all: "すべてのコマンドを見る" },
  scenes: {
    import: "SillyTavern や MMD のカードをフォルダとして取り込みます。",
    push: "カードを非公開の試用カードにプッシュして検証します。",
    play: "ターミナルを離れずに 1 ターン試せます。",
    pull: "自分のカードをファイルとして取り出します。",
    search: "コミュニティのランキングを検索します。",
    media: "ライブラリの画像を管理します。",
  },
  headline: "コードを書くように、カードを書く。",
  props: [
    { title: "使い慣れたエディタで書く", text: "カードは Markdown と JSON のフォルダです。いつものエディタで編集し、すべての版を git に残し、ファイルのまま誰にでも渡せます。", link: "カードフォルダについて", href: "/guides/card-folder/" },
    { title: "AI エージェントから使える", text: "すべてのコマンドが JSON を出力し、ブラウザなしでサインインでき、フラグを付けない限りクレジットは消費しません。", link: "エージェントと使う", href: "/guides/agents/" },
    { title: "誰かに見せる前に、自分で遊ぶ", text: "プッシュしたカードは自分だけが見られる試用カードに置かれ、すぐに試せます。あなたが決めるまで公開されません。", link: "試用カードについて", href: "/guides/trial-cards/" },
    { title: "無料でオープンソース", text: "AGPL-3.0、単一バイナリ。必要なのは Hearthroom のアカウントだけです。", link: "GitHub で参加する", href: "https://github.com/hearthroom/cli" },
  ],
  again: { title: "ターミナルで Hearthroom を試す", text: "Hearthroom CLI は、キャラクターカードをターミナルから扱えるようにします。無料でオープンソース。" },
};

const ko: Strings = {
  skip: "본문으로 건너뛰기", navLabel: "사이트", language: "언어", theme: "라이트와 다크 전환", viewMarkdown: "Markdown으로 보기",
  nav: { manual: "매뉴얼", guides: "가이드", releases: "릴리스", developers: "개발자 문서" },
  hero: { title: "Hearthroom을 터미널로", lead: "Hearthroom CLI는 캐릭터 카드를 터미널과 AI Agent에서 다룰 수 있게 해 줍니다. 무료 오픈 소스입니다." },
  install: {
    label: { brew: "Homebrew로 설치", sh: "셸 스크립트로 설치", ps: "PowerShell로 설치", scoop: "Scoop으로 설치" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — 셸 스크립트", shLinux: "Linux — 셸 스크립트", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "실행 파일 다운로드" },
    copy: "복사", copied: "복사됨",
    instructions: "설치 안내 보기", agent: "AI Agent와 함께 쓴다면 매뉴얼을 건네세요:", version: (v) => `최신 릴리스 ${v}`,
  },
  terminal: { pause: "애니메이션 일시정지", play: "애니메이션 재생", all: "모든 명령 보기" },
  scenes: {
    import: "SillyTavern이나 MMD 카드를 폴더로 가져옵니다.",
    push: "카드를 비공개 체험 카드에 푸시하고 검증합니다.",
    play: "터미널을 벗어나지 않고 한 턴 체험합니다.",
    pull: "내 카드를 파일로 내려받습니다.",
    search: "커뮤니티 랭킹을 검색합니다.",
    media: "라이브러리의 이미지를 관리합니다.",
  },
  headline: "코드를 쓰듯 카드를 씁니다.",
  props: [
    { title: "쓰던 편집기로 그대로 쓰기", text: "카드는 Markdown과 JSON이 든 폴더입니다. 늘 쓰던 편집기로 고치고, 모든 버전을 git에 남기고, 파일 그대로 누구에게나 건넬 수 있습니다.", link: "카드 폴더 알아보기", href: "/guides/card-folder/" },
    { title: "AI Agent에서 바로 사용", text: "모든 명령이 JSON을 출력하고, 브라우저 없이 로그인하며, 플래그 없이는 크레딧을 쓰지 않습니다.", link: "Agent와 함께 쓰기", href: "/guides/agents/" },
    { title: "남에게 보여 주기 전에 직접 플레이", text: "푸시한 카드는 나만 볼 수 있는 체험 카드에 올라가 바로 플레이할 수 있습니다. 내가 정하기 전에는 공개되지 않습니다.", link: "체험 카드 알아보기", href: "/guides/trial-cards/" },
    { title: "무료 오픈 소스", text: "AGPL-3.0, 단일 실행 파일. Hearthroom 계정만 있으면 됩니다.", link: "GitHub에서 참여하기", href: "https://github.com/hearthroom/cli" },
  ],
  again: { title: "터미널에서 Hearthroom 써 보기", text: "Hearthroom CLI는 캐릭터 카드를 터미널에서 다룰 수 있게 해 줍니다. 무료 오픈 소스입니다." },
};

const table: Record<Locale, Strings> = { en, "zh-Hant": zhHant, "zh-Hans": zhHans, ja, ko };
export function t(locale: Locale): Strings {
  return table[locale] ?? en;
}
