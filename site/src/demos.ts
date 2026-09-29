import type { Locale } from "./i18n";

// Demo transcripts for the landing page. Shapes and wording match what the CLI
// really prints, but every card, author, id and number here is invented.
// Ids look like the CLI's UUIDv7 role ids; none exists.

export const CARD = "Mira";
export const ROLE = "01a1a2b3-4c5d-7e6f-8a9b-0c1d2e3f4a5b";
export const CONV = "01a1a2b3-5d6e-7f70-9a0b-1c2d3e4f5a6b";
export const ASSET = "01a1a2b3-6e7f-7081-a1b2-c3d4e5f6a7b8";

export const cardImport = [
  "$ hearthroom card import mira.png",
  `Imported chara_card_v3 card "Mira" into ${CARD}`,
  "  card.json", "  definition.md", "  welcome.md", "  openings/alt-01.md", "  lorebook.json", "  rules.json", "  assets/portrait.png",
  "  Lorebook entries: 4", "  display rules: 2",
  "Not imported or changed:",
  "  - 2 Lorebook entries carry an insertion position; the provider places entries itself, so it was not imported",
  `Next: review the folder, then \`hearthroom card push ${CARD}\``,
];

export const cardPush = [
  `$ hearthroom card push ${CARD} --validate`,
  `Pushed to new trial card ${ROLE}`,
  "  asset assets/portrait.png            uploaded",
  "  sections sent:      card, welcome, worldbook, authorAsset, media",
  "  trial expires:      2026-10-02T09:12:00Z",
  "  trial slots:        1 of 5 used",
  `Play: https://hearthroom.club/play/${ROLE}`,
  "Validation: PASS",
  "Budget (~1240 tokens):",
  "  summary                  84 / 2500",
  "  definition             3120 / 50000",
  "  opening                 610 / 10000",
];

export const play = [
  `$ hearthroom play ${CARD} -m "Is the light on tonight?" --allow-spend`,
  "The lamp has been lit since dusk. Mira glances up from the logbook.",
  "\"It always is. Are you the one from the mainland boat?\"",
  "[finished: stop]",
  `$ hearthroom play ${CARD} --history --limit 2`,
  `Conversation ${CONV} with Mira (3 messages)`,
  "", "[USER]", "Is the light on tonight?", "", "[AI]", "The lamp has been lit since dusk. Mira glances up from the logbook.",
];

export const cardPull = [
  `$ hearthroom card pull ${ROLE} mira --download`,
  `Pulled ${ROLE} into mira`,
  "  card.json", "  definition.md", "  welcome.md", "  openings/alt-01.md", "  lorebook.json", "  rules.json",
  "  assets/portrait.png (downloaded)",
  "$ git -C mira init -q && git -C mira add -A && git -C mira commit -qm 'Mira, first draft'",
];

export const mediaRm = [
  `$ hearthroom media rm ${ASSET}`,
  `${ASSET}: still used by 1 card(s); change their image first:`,
  `    ${ROLE}  Mira`,
  `$ hearthroom card push ${CARD}      # after pointing card.json at a new portrait`,
  "  sections sent:      media",
  `$ hearthroom media rm ${ASSET}`,
  `${ASSET} deleted`,
];

// The search demo follows the interface language, since the board itself is
// split into language zones.
const searchByLocale: Record<Locale, string[]> = {
  en: [
    "$ hearthroom search lighthouse --zone en --limit 3",
    "ID      NAME                 AUTHOR     TAGS                    SUMMARY",
    "100412  The Keeper of Ashby  tidewrite  slice-of-life,romance   A lighthouse keeper who talks to the storm…",
    "100377  Salt & Signal        northlamp  mystery,slow-burn       Someone is answering the light from the cliffs…",
    "100298  Harbor Watch         quillfox   drama,ensemble          Three shifts, one harbor, and a secret the fog…",
  ],
  "zh-Hant": [
    "$ hearthroom search 燈塔 --zone zh --limit 3",
    "ID      NAME          AUTHOR     TAGS               SUMMARY",
    "100412  艾許比的守塔人   tidewrite  日常,戀愛           一個會對暴風雨說話的守塔人…",
    "100377  鹽與信號       northlamp  懸疑,慢熱           懸崖那頭有人在回應燈光…",
    "100298  港口值夜       quillfox   劇情,群像           三班輪值、一座港口，和一個藏在霧裡的秘密…",
  ],
  "zh-Hans": [
    "$ hearthroom search 灯塔 --zone zh --limit 3",
    "ID      NAME          AUTHOR     TAGS               SUMMARY",
    "100412  艾许比的守塔人   tidewrite  日常,恋爱           一个会对暴风雨说话的守塔人…",
    "100377  盐与信号       northlamp  悬疑,慢热           悬崖那头有人在回应灯光…",
    "100298  港口值夜       quillfox   剧情,群像           三班轮值、一座港口，和一个藏在雾里的秘密…",
  ],
  ja: [
    "$ hearthroom search 灯台 --zone ja --limit 3",
    "ID      NAME              AUTHOR     TAGS               SUMMARY",
    "100412  アシュビーの灯台守   tidewrite  日常,恋愛           嵐に語りかける灯台守…",
    "100377  塩と信号           northlamp  ミステリー,スロー    崖の向こうで誰かが光に応えている…",
    "100298  港の夜番           quillfox   ドラマ,群像         三交代、ひとつの港、霧に隠れた秘密…",
  ],
  ko: [
    "$ hearthroom search 등대 --zone ko --limit 3",
    "ID      NAME            AUTHOR     TAGS               SUMMARY",
    "100412  애시비의 등대지기   tidewrite  일상,로맨스          폭풍에게 말을 거는 등대지기…",
    "100377  소금과 신호       northlamp  미스터리,슬로우번     절벽 너머에서 누군가 불빛에 답하고 있다…",
    "100298  항구 야간 근무     quillfox   드라마,군상          세 교대, 하나의 항구, 안개 속에 숨은 비밀…",
  ],
};

export function search(locale: Locale): string[] {
  return searchByLocale[locale] ?? searchByLocale.en;
}

export function demos(locale: Locale): Record<string, string[]> {
  return {
    "hearthroom card import": cardImport,
    "hearthroom card push --validate": cardPush,
    "hearthroom play": play,
    "hearthroom card pull": cardPull,
    "hearthroom search": search(locale),
    "hearthroom media rm": mediaRm,
  };
}

/** Scenes for the animated hero terminal: a typed command, then its output. */
export type SceneKey = "import" | "push" | "play" | "pull" | "search" | "media";
export const heroScenes: { key: SceneKey; cmd: string; out: string[] }[] = [
  { key: "import", cmd: "hearthroom card import mira.png", out: cardImport.slice(1, 9).concat([`Next: hearthroom card push ${CARD}`]) },
  { key: "push", cmd: `hearthroom card push ${CARD} --validate`, out: cardPush.slice(1, 8) },
  { key: "play", cmd: `hearthroom play ${CARD} -m "Is the light on tonight?" --allow-spend`, out: play.slice(1, 4) },
  { key: "pull", cmd: `hearthroom card pull ${ROLE} mira`, out: cardPull.slice(1, 7) },
  { key: "search", cmd: search("en")[0].slice(2), out: search("en").slice(1) },
  { key: "media", cmd: `hearthroom media rm ${ASSET}`, out: mediaRm.slice(1, 3) },
];
