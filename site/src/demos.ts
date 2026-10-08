import type { Locale } from "./i18n";

// Demo transcripts for the animated terminal on the landing page. Shapes and
// wording match what the CLI really prints, but every card, author, id and
// number here is invented. Ids look like the CLI's UUIDv7 role ids; none exists.

export const CARD = "Mira";
export const ROLE = "01a1a2b3-4c5d-7e6f-8a9b-0c1d2e3f4a5b";
export const ASSET = "01a1a2b3-6e7f-7081-a1b2-c3d4e5f6a7b8";

/**
 * One output line. Plain strings print in the default colour; a line that
 * starts with "$ " prints as a further prompt. `k` marks the few lines a real
 * terminal would colour: `b` bold headline, `ok` success, `err` failure,
 * `dim` secondary.
 */
export type Line = string | { t: string; k: "b" | "ok" | "err" | "dim" };
export type SceneKey = "import" | "push" | "play" | "pull" | "search" | "media";
export type Scene = { key: SceneKey; cmd: string; out: Line[] };

const b = (t: string): Line => ({ t, k: "b" });
const ok = (t: string): Line => ({ t, k: "ok" });
const err = (t: string): Line => ({ t, k: "err" });
const dim = (t: string): Line => ({ t, k: "dim" });

// The search demo follows the interface language, since the board itself is
// split into language zones. Everything else is English because the CLI is.
const search: Record<Locale, Scene> = {
  en: { key: "search", cmd: "hearthroom search lighthouse --zone en --limit 3", out: [
    b("ID      NAME                 AUTHOR     TAGS                    SUMMARY"),
    "100412  The Keeper of Ashby  tidewrite  slice-of-life,romance   A lighthouse keeper who talks to the storm…",
    "100377  Salt & Signal        northlamp  mystery,slow-burn       Someone is answering the light from the cliffs…",
    "100298  Harbor Watch         quillfox   drama,ensemble          Three shifts, one harbor, and a secret the fog…",
  ] },
  "zh-Hant": { key: "search", cmd: "hearthroom search 燈塔 --zone zh --limit 3", out: [
    b("ID      NAME            AUTHOR     TAGS          SUMMARY"),
    "100412  艾許比的守塔人     tidewrite  日常,戀愛      一個會對暴風雨說話的守塔人…",
    "100377  鹽與信號         northlamp  懸疑,慢熱      懸崖那頭有人在回應燈光…",
    "100298  港口值夜         quillfox   劇情,群像      三班輪值、一座港口，和一個藏在霧裡的秘密…",
  ] },
  "zh-Hans": { key: "search", cmd: "hearthroom search 灯塔 --zone zh --limit 3", out: [
    b("ID      NAME            AUTHOR     TAGS          SUMMARY"),
    "100412  艾许比的守塔人     tidewrite  日常,恋爱      一个会对暴风雨说话的守塔人…",
    "100377  盐与信号         northlamp  悬疑,慢热      悬崖那头有人在回应灯光…",
    "100298  港口值夜         quillfox   剧情,群像      三班轮值、一座港口，和一个藏在雾里的秘密…",
  ] },
  ja: { key: "search", cmd: "hearthroom search 灯台 --zone ja --limit 3", out: [
    b("ID      NAME                AUTHOR     TAGS              SUMMARY"),
    "100412  アシュビーの灯台守     tidewrite  日常,恋愛          嵐に語りかける灯台守…",
    "100377  塩と信号             northlamp  ミステリー,スロー   崖の向こうで誰かが光に応えている…",
    "100298  港の夜番             quillfox   ドラマ,群像        三交代、ひとつの港、霧に隠れた秘密…",
  ] },
  ko: { key: "search", cmd: "hearthroom search 등대 --zone ko --limit 3", out: [
    b("ID      NAME              AUTHOR     TAGS              SUMMARY"),
    "100412  애시비의 등대지기     tidewrite  일상,로맨스         폭풍에게 말을 거는 등대지기…",
    "100377  소금과 신호         northlamp  미스터리,슬로우번    절벽 너머에서 누군가 불빛에 답하고 있다…",
    "100298  항구 야간 근무       quillfox   드라마,군상         세 교대, 하나의 항구, 안개 속에 숨은 비밀…",
  ] },
};

export function heroScenes(locale: Locale): Scene[] {
  return [
    { key: "import", cmd: "hearthroom card import mira.png", out: [
      b(`Imported chara_card_v3 card "Mira" into ${CARD}/`),
      "  card.json", "  definition.md", "  welcome.md", "  openings/alt-01.md", "  lorebook.json", "  rules.json", "  assets/portrait.png",
      dim("  Lorebook entries: 4 · display rules: 2"),
      `Next: hearthroom card push ${CARD}`,
    ] },
    { key: "push", cmd: `hearthroom card push ${CARD} --validate`, out: [
      b(`Pushed to new trial card ${ROLE}`),
      "  asset assets/portrait.png      uploaded",
      "  sections sent:   card, welcome, worldbook, authorAsset, media",
      "  trial expires:   2026-10-02T09:12:00Z",
      "  trial slots:     1 of 5 used",
      `Play: https://hearthroom.club/play/${ROLE}?mode=source`,
      ok("Validation: PASS"),
      dim("Budget (~1240 tokens): definition 3120 / 50000 · opening 610 / 10000"),
    ] },
    { key: "play", cmd: `hearthroom play ${CARD} -m "Is the light on tonight?" --allow-spend`, out: [
      "The lamp has been lit since dusk. Mira glances up from the logbook.",
      "\"It always is. Are you the one from the mainland boat?\"",
      dim("[finished: stop]"),
    ] },
    { key: "pull", cmd: `hearthroom card pull ${ROLE} mira --download`, out: [
      b(`Pulled ${ROLE} into mira/`),
      "  card.json", "  definition.md", "  welcome.md", "  openings/alt-01.md", "  lorebook.json", "  rules.json",
      "  assets/portrait.png (downloaded)",
      "$ git -C mira init -q && git -C mira add -A && git -C mira commit -qm 'Mira, first draft'",
    ] },
    search[locale] ?? search.en,
    { key: "media", cmd: `hearthroom media rm ${ASSET}`, out: [
      err(`${ASSET}: still used by 1 card(s); change their image first:`),
      `    ${ROLE}  Mira`,
      `$ hearthroom card push ${CARD}`,
      dim("  sections sent:   media"),
      `$ hearthroom media rm ${ASSET}`,
      ok(`${ASSET} deleted`),
    ] },
  ];
}
