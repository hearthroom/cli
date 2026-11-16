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
  hero: {"title":"Let your AI agent build cards from the terminal","lead":"Hearthroom CLI lets Claude Code, Codex, and Cursor draft cards, edit Lorebooks, preview screens, and test turns directly, without copying and pasting between chat windows and web forms. Free and open source."},
  install: {
    label: { brew: "Install with Homebrew", sh: "Install with the shell script", ps: "Install with PowerShell", scoop: "Install with Scoop" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — Shell script", shLinux: "Linux — Shell script", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "Download a binary" },
    copy: "Copy", copied: "Copied",
    instructions: "View installation instructions", agent: "Full manual for your AI agent:", version: (v) => `Latest release ${v}`,
  },
  terminal: { pause: "Pause animation", play: "Play animation", all: "View all Hearthroom CLI commands" },
  scenes: {
    "import": "Import a SillyTavern or MMD card in one command, split cleanly into persona, Lorebook, and display rules.",
    "push": "Push to a private trial card and check field lengths and rules on the spot.",
    "play": "Send a real line from the terminal to see how the character responds.",
    "pull": "Pull an existing card and its images down to your machine to edit.",
    "search": "Search published cards on the community board from the terminal.",
    "media": "Manage your image library; images still used by a card are protected from accidental deletion."
  },
  headline: "Stop pasting fields into web forms. Let your agent run the whole loop.",
  props: [
    {
      "title": "Hand card edits straight to your agent",
      "text": "Once the CLI is installed, Claude Code, Codex, and Cursor can create cards, revise definitions, upload art, and update drafts directly. Sign in using a one-time code without opening a local browser, and nothing spends credits without --allow-spend.",
      "link": "Use it with an agent",
      "href": "/guides/agents/"
    },
    {
      "title": "Check layouts and display rules locally before pushing",
      "text": "Preview phone and desktop screens in both dark and light mode inside the real sandbox chat shell (card preview), and catch broken patterns up front (card check). Your agent reads the screenshots and error reports and fixes issues before handing the card back.",
      "link": "See the preview command",
      "href": "/manual/card/preview/"
    },
    {
      "title": "Bring existing cards over, generate art, and playtest in one place",
      "text": "Import exports from SillyTavern, MMD, AI Fengyue, or RiliAI (card import). Generate banners or expression variants from your character's portrait as a reference (media generate), then run multi-turn playtests right in the terminal (play).",
      "link": "See the import command",
      "href": "/manual/card/import/"
    },
    {
      "title": "Edit on the web while your agent updates entries without conflicts",
      "text": "Your agent can add or patch a single Lorebook entry (lorebook entry) and stops on version conflicts instead of wiping changes you just saved in the browser. Commands also cover Workshop DLC, shared polls, and review submission. Free and open source under AGPL-3.0.",
      "link": "View source on GitHub",
      "href": "https://github.com/hearthroom/cli"
    }
  ],
  again: {"title":"Install once and hand it to your agent","text":"A single binary that works with your existing Hearthroom account. Pushing, checking, and previewing cost zero credits."},
};

const zhHant: Strings = {
  skip: "跳到主要內容", navLabel: "網站", language: "語言", theme: "切換深色與淺色", viewMarkdown: "以 Markdown 檢視",
  nav: { manual: "手冊", guides: "指南", releases: "版本", developers: "開發者文件" },
  hero: {"title":"讓 AI Agent 直接在終端機幫你做卡","lead":"Hearthroom CLI 讓 Claude Code、Codex、Cursor 直接幫你建卡、改世界書、預覽畫面和試聊調設定，不用自己在視窗和網頁欄位之間來回貼字。工具免費且原始碼公開。"},
  install: {
    label: { brew: "用 Homebrew 安裝", sh: "用安裝腳本安裝", ps: "用 PowerShell 安裝", scoop: "用 Scoop 安裝" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — 安裝腳本", shLinux: "Linux — 安裝腳本", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "下載執行檔" },
    copy: "複製", copied: "已複製",
    instructions: "查看安裝說明", agent: "給 AI Agent 讀的完整手冊：", version: (v) => `最新版本 ${v}`,
  },
  terminal: { pause: "暫停動畫", play: "播放動畫", all: "查看全部指令" },
  scenes: {
    "import": "一行指令匯入 SillyTavern 或魅魔島（MMD）的角色卡，自動拆好人設、世界書與規則。",
    "push": "上傳成只有自己看得到的試玩卡，順便檢查字數與欄位有沒有超標。",
    "play": "直接在終端機送出一句話，實測角色怎麼接話。",
    "pull": "把站上已有的角色卡連同圖片一起拉回本機修改。",
    "search": "直接從終端機搜尋站上的公開作品。",
    "media": "整理個人圖庫，圖片還被卡片使用時會先擋下來，不怕誤刪。"
  },
  headline: "不用再把設定一段段貼進網頁，整張卡直接交給 Agent 做完。",
  props: [
    {
      "title": "交給 Agent 直接建卡改卡，不用來回複製貼上",
      "text": "裝好之後，Claude Code、Codex、Cursor 可以直接替你建新卡、改人設、上傳圖片與更新草稿。登入只要輸入一次性驗證碼，不必在同一個環境開瀏覽器，而且沒加扣點參數（--allow-spend）就不會花你的積分。",
      "link": "搭配 AI Agent 使用",
      "href": "/guides/agents/"
    },
    {
      "title": "畫面和規則在本機就能檢查，不用推上去盲測",
      "text": "本機就能用站上同款的沙箱聊天頁預覽手機與桌機、深色與淺色畫面（card preview），也能先跑規則檢查抓出寫壞的語法（card check）。Agent 自己看得到截圖與報錯，修好再交給你。",
      "link": "查看畫面預覽指令",
      "href": "/manual/card/preview/"
    },
    {
      "title": "他站舊卡直接搬過來，生圖跟試玩也在同一處完成",
      "text": "SillyTavern、魅魔島三件套、AI 風月、RiliAI 的匯出檔都能直接轉成可改的卡片（card import）。缺封面或表情差分可以拿原圖當參考直接生圖（media generate），改完馬上送訊息實測多輪對話（play）。",
      "link": "查看匯入指令",
      "href": "/manual/card/import/"
    },
    {
      "title": "你在網頁微調、Agent 在終端機補設定，兩邊不會互蓋",
      "text": "Agent 可以單獨增改世界書裡的某一條設定（lorebook entry），遇到你在網頁剛改過的版本會自動停下來提醒，不會整本洗掉。工坊 DLC、投票計數與送審上架也都能在終端機處理，整套工具免費並以 AGPL-3.0 公開原始碼。",
      "link": "到 GitHub 看原始碼",
      "href": "https://github.com/hearthroom/cli"
    }
  ],
  again: {"title":"裝好就能交給 Agent 開工","text":"單一執行檔，登入你原本的帳號就能用。推卡、檢查與預覽都不扣積分。"},
};

const zhHans: Strings = {
  skip: "跳到主要内容", navLabel: "网站", language: "语言", theme: "切换深色与浅色", viewMarkdown: "以 Markdown 查看",
  nav: { manual: "手册", guides: "指南", releases: "版本", developers: "开发者文档" },
  hero: {"title":"让 AI Agent 直接在终端里帮你做卡","lead":"Hearthroom CLI 让 Claude Code、Codex、Cursor 直接帮你建卡、改世界书、预览画面并试聊调设定，不用自己在窗口和网页表单之间来回复制粘贴。免费且开源。"},
  install: {
    label: { brew: "用 Homebrew 安装", sh: "用安装脚本安装", ps: "用 PowerShell 安装", scoop: "用 Scoop 安装" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — 安装脚本", shLinux: "Linux — 安装脚本", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "下载可执行文件" },
    copy: "复制", copied: "已复制",
    instructions: "查看安装说明", agent: "给 AI Agent 读的完整手册：", version: (v) => `最新版本 ${v}`,
  },
  terminal: { pause: "暂停动画", play: "播放动画", all: "查看全部命令" },
  scenes: {
    "import": "一行命令导入 SillyTavern 或魅魔岛（MMD）的角色卡，自动拆好人设、世界书和规则。",
    "push": "上传成只有自己能看到的试玩卡，顺带检查字数和字段有没有超限。",
    "play": "直接在终端发出一句话，实测角色怎么接话。",
    "pull": "把站上已有的角色卡连同图片一起拉回本地修改。",
    "search": "直接从终端搜索站上的公开作品。",
    "media": "整理个人图库，图片还在被卡片使用时会自动拦下，不怕误删。"
  },
  headline: "不用再把设定一段段贴进网页，整张卡直接交给 Agent 做完。",
  props: [
    {
      "title": "交给 Agent 直接建卡改卡，不用来回复制粘贴",
      "text": "装好之后，Claude Code、Codex、Cursor 可以直接帮你建新卡、改人设、上传图片和更新草稿。登录只需输入一次性验证码，不必在同一台机器上开浏览器，不加扣点参数（--allow-spend）也不会消耗你的积分。",
      "link": "搭配 AI Agent 使用",
      "href": "/guides/agents/"
    },
    {
      "title": "画面和规则在本地就能检查，不用传上去盲测",
      "text": "本地就能用与线上同款的沙箱聊天页预览手机和电脑、深色和浅色画面（card preview），也能先跑规则检查抓出写坏的语法（card check）。Agent 自己看得到截图和报错，修好再交给你。",
      "link": "查看画面预览命令",
      "href": "/manual/card/preview/"
    },
    {
      "title": "外站旧卡直接搬过来，生图和试玩也在同一处完成",
      "text": "SillyTavern、魅魔岛三件套、AI 风月、RiliAI 的导出文件都能直接转成可编辑的卡片（card import）。缺封面或表情差分可以拿原图作参考直接生图（media generate），改完马上发消息实测多轮对话（play）。",
      "link": "查看导入命令",
      "href": "/manual/card/import/"
    },
    {
      "title": "你在网页微调、Agent 在终端补设定，两边不会互盖",
      "text": "Agent 可以单独增改世界书里的某一条设定（lorebook entry），遇到你在网页刚改过的版本会自动停下提醒，不会整本覆盖掉。工坊 DLC、投票计数与送审上架也都能在终端处理，整套工具免费并以 AGPL-3.0 开源。",
      "link": "去 GitHub 看源码",
      "href": "https://github.com/hearthroom/cli"
    }
  ],
  again: {"title":"装好就能交给 Agent 开工","text":"单个可执行文件，登录你原本的账号就能用。推卡、检查与预览都不扣积分。"},
};

const ja: Strings = {
  skip: "本文へスキップ", navLabel: "サイト", language: "言語", theme: "ライトとダークを切り替える", viewMarkdown: "Markdown で表示",
  nav: { manual: "マニュアル", guides: "ガイド", releases: "リリース", developers: "開発者ドキュメント" },
  hero: {"title":"ターミナルから、カード作りを AI エージェントに任せる","lead":"Hearthroom CLI があれば、Claude Code・Codex・Cursor がカード作成、世界設定の編集、画面プレビュー、テスト会話まで直接こなします。チャット欄と Web フォームの間でコピペを繰り返す必要はありません。無料・オープンソース。"},
  install: {
    label: { brew: "Homebrew でインストール", sh: "シェルスクリプトでインストール", ps: "PowerShell でインストール", scoop: "Scoop でインストール" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — シェルスクリプト", shLinux: "Linux — シェルスクリプト", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "バイナリをダウンロード" },
    copy: "コピー", copied: "コピー済み",
    instructions: "インストール手順を見る", agent: "AI エージェントに渡すマニュアル全文：", version: (v) => `最新リリース ${v}`,
  },
  terminal: { pause: "アニメーションを停止", play: "アニメーションを再生", all: "すべてのコマンドを見る" },
  scenes: {
    "import": "SillyTavern や MMD のカードを 1 コマンドで取り込み、設定・世界設定・表示ルールに自動で分けます。",
    "push": "自分だけが見られる試用カードに送り、文字数や項目の上限をその場でチェックします。",
    "play": "ターミナルからそのまま一言送り、キャラクターの返しを確かめます。",
    "pull": "公開中や作成中のカードを画像ごと手元に引き出して編集します。",
    "search": "コミュニティに公開されている作品をターミナルから検索します。",
    "media": "ライブラリの画像を整理します。カードで使用中の画像は誤って消えないよう保護されます。"
  },
  headline: "Web フォームへの貼り付け作業をやめて、カード作りを丸ごとエージェントへ。",
  props: [
    {
      "title": "コピペなしで、エージェントが直接カードを作成・更新",
      "text": "導入すれば、Claude Code・Codex・Cursor が新規作成、設定の書き換え、画像のアップロード、下書き更新まで直接行います。ワンタイムコードでサインインできるため同じ環境にブラウザは不要で、--allow-spend を付けない限りポイントも消費しません。",
      "link": "エージェントと使う",
      "href": "/guides/agents/"
    },
    {
      "title": "アップロード前に、手元で画面崩れとルールを検証",
      "text": "本番と同じサンドボックス画面でスマホ・PC、ダーク・ライト両方の表示を手元でプレビューでき（card preview）、正規表現や記法のミスも事前に検出できます（card check）。エージェント自身がスクリーンショットとエラーを読んで直してから渡してくれます。",
      "link": "プレビューのコマンドを見る",
      "href": "/manual/card/preview/"
    },
    {
      "title": "他サイトのカード移行も、差分イラスト生成もテスト会話も一箇所で",
      "text": "SillyTavern・MMD・AI 風月・RiliAI の書き出しデータをそのまま編集可能なカードに変換できます（card import）。立ち絵を参照画像にしてバナーや表情差分を生成し（media generate）、そのまま複数ターンのテスト会話も回せます（play）。",
      "link": "インポートのコマンドを見る",
      "href": "/manual/card/import/"
    },
    {
      "title": "Web での手直しとターミナルからの追記がぶつからない",
      "text": "世界設定の特定の 1 件だけをエージェントに追記・更新させることができ（lorebook entry）、Web 側で保存したばかりの変更があれば競合を検知して止まるため上書き事故が起きません。ワークショップ DLC や投票集計、審査提出にも対応。AGPL-3.0 の無料オープンソースです。",
      "link": "GitHub でソースを見る",
      "href": "https://github.com/hearthroom/cli"
    }
  ],
  again: {"title":"入れたらすぐにエージェントへ任せられます","text":"単一バイナリで、普段の Hearthroom アカウントのまま使えます。プッシュ・チェック・プレビューにポイントはかかりません。"},
};

const ko: Strings = {
  skip: "본문으로 건너뛰기", navLabel: "사이트", language: "언어", theme: "라이트와 다크 전환", viewMarkdown: "Markdown으로 보기",
  nav: { manual: "매뉴얼", guides: "가이드", releases: "릴리스", developers: "개발자 문서" },
  hero: {"title":"터미널에서 AI 에이전트에게 바로 카드를 맡기세요","lead":"Hearthroom CLI가 있으면 Claude Code, Codex, Cursor가 캐릭터 카드 작성부터 세계관 수정, 화면 미리보기, 테스트 대화까지 직접 처리해요. 채팅창과 웹 입력칸 사이를 오가며 복사해 붙여 넣을 필요가 없어요. 무료 오픈 소스예요."},
  install: {
    label: { brew: "Homebrew로 설치", sh: "셸 스크립트로 설치", ps: "PowerShell로 설치", scoop: "Scoop으로 설치" },
    method: { brewMac: "macOS — Homebrew", shMac: "macOS — 셸 스크립트", shLinux: "Linux — 셸 스크립트", ps: "Windows — PowerShell", scoop: "Windows — Scoop", download: "실행 파일 다운로드" },
    copy: "복사", copied: "복사됨",
    instructions: "설치 안내 보기", agent: "AI 에이전트에게 건네줄 전체 매뉴얼:", version: (v) => `최신 릴리스 ${v}`,
  },
  terminal: { pause: "애니메이션 일시정지", play: "애니메이션 재생", all: "모든 명령 보기" },
  scenes: {
    "import": "명령어 한 줄로 SillyTavern이나 MMD 카드를 가져와 캐릭터 설정, 세계관, 표시 규칙으로 깔끔하게 나눠요.",
    "push": "나만 볼 수 있는 체험 카드로 올리고 글자 수와 항목 제한을 그 자리에서 검사해요.",
    "play": "터미널에서 바로 한마디 건네 보고 캐릭터가 어떻게 받아치는지 확인해요.",
    "pull": "사이트에 올려 둔 카드를 이미지까지 통째로 내려받아 내 컴퓨터에서 수정해요.",
    "search": "커뮤니티에 공개된 작품을 터미널에서 바로 검색해요.",
    "media": "개인 이미지 보관함을 정리해요. 카드에서 아직 쓰고 있는 이미지는 실수로 지워지지 않게 막아 줘요."
  },
  headline: "설정을 웹 입력칸에 일일이 붙여 넣지 마세요. 카드 한 장을 통째로 에이전트에게 맡기면 돼요.",
  props: [
    {
      "title": "복사·붙여넣기 없이 에이전트가 직접 만들고 고쳐요",
      "text": "설치만 해 두면 Claude Code, Codex, Cursor가 새 카드를 만들고 설정을 고치고 이미지를 올리고 초안을 업데이트해요. 일회용 코드로 로그인해서 같은 환경에 브라우저가 없어도 되고, --allow-spend 옵션을 붙이지 않으면 포인트가 차감되지 않아요.",
      "link": "에이전트와 함께 쓰기",
      "href": "/guides/agents/"
    },
    {
      "title": "올리기 전에 내 컴퓨터에서 화면과 규칙을 먼저 확인해요",
      "text": "실제 채팅창과 똑같은 샌드박스 화면으로 모바일·PC, 다크·라이트 모드를 미리 보고(card preview), 깨진 정규식이나 문법 오류도 미리 잡아내요(card check). 에이전트가 직접 스크린샷과 오류를 읽고 고친 뒤 넘겨줘요.",
      "link": "미리보기 명령어 보기",
      "href": "/manual/card/preview/"
    },
    {
      "title": "다른 사이트 카드 이사부터 표정 바리에이션 생성, 테스트 플레이까지 한곳에서",
      "text": "SillyTavern, MMD 3종 세트, AI 풍월, RiliAI에서 내보낸 파일을 바로 수정 가능한 카드로 바꿔요(card import). 기존 일러스트를 참고 이미지로 삼아 배너나 표정 바리에이션을 뽑고(media generate), 곧바로 여러 턴 대화를 돌려 볼 수 있어요(play).",
      "link": "가져오기 명령어 보기",
      "href": "/manual/card/import/"
    },
    {
      "title": "웹에서 다듬는 동안 에이전트가 세계관을 보태도 덮어쓰지 않아요",
      "text": "세계관에서 특정 항목 하나만 골라 추가하거나 고칠 수 있고(lorebook entry), 방금 웹 편집기에서 저장한 내용이 있으면 충돌을 감지해 멈추기 때문에 통째로 날아갈 일이 없어요. 창작마당 DLC, 투표 집계, 심사 제출까지 모두 지원하며 AGPL-3.0 무료 오픈 소스예요.",
      "link": "GitHub에서 소스 코드 보기",
      "href": "https://github.com/hearthroom/cli"
    }
  ],
  again: {"title":"설치만 끝내면 바로 에이전트가 일해요","text":"단일 실행 파일이라 쓰던 Hearthroom 계정 그대로 로그인하면 돼요. 푸시, 검사, 미리보기에는 포인트가 들지 않아요."},
};

const table: Record<Locale, Strings> = { en, "zh-Hant": zhHant, "zh-Hans": zhHans, ja, ko };
export function t(locale: Locale): Strings {
  return table[locale] ?? en;
}
