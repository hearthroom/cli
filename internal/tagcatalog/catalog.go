// Package tagcatalog holds the platform's fixed preset category and play-style
// tag catalog shared with the Hearthroom community service and web editor.
package tagcatalog

import "strings"

// StoredTagsMax is the server's projected tag limit (matching STORED_TAGS_MAX).
const StoredTagsMax = 20

// Locales are the supported display-name languages in catalog order.
var Locales = []string{"zh-Hant", "zh-Hans", "en", "ja", "ko"}

// Groups are the four preset category groups in display order.
var Groups = []string{"play", "setting", "character", "kink"}

// Item is one entry in the preset tag catalog.
type Item struct {
	Key   string            `json:"key"`
	Group string            `json:"group"`
	Name  string            `json:"name,omitempty"`
	Names map[string]string `json:"names"`
}

// CatalogResponse matches GET /v1/tags/catalog.
type CatalogResponse struct {
	Locale string   `json:"locale,omitempty"`
	Groups []string `json:"groups"`
	Items  []Item   `json:"items"`
}

type rawEntry struct {
	key    string
	zhHans string
	zhHant string
	en     string
	ja     string
	ko     string
}

var entries = []rawEntry{
	{"roleplay", "角色扮演", "角色扮演", "Roleplay", "ロールプレイ", "롤플레이"},
	{"world-sim", "世界模拟器", "世界模擬器", "World simulator", "世界シミュレーター", "세계 시뮬레이터"},
	{"gameplay-sim", "玩法模拟器", "玩法模擬器", "Gameplay simulator", "ゲームシミュレーター", "게임 시뮬레이터"},
	{"loli", "萝莉", "蘿莉", "Loli", "ロリ", "로리"},
	{"hypnosis", "催眠", "催眠", "Hypnosis", "催眠", "최면"},
	{"female-pov", "女生视角", "女生視角", "Female POV", "女性視点", "여성 시점"},
	{"taboo", "伦理", "倫理", "Taboo", "背徳", "금기"},
	{"training", "调教&强迫", "調教&強迫", "Training & coercion", "調教&強制", "조교&강제"},
	{"ntl", "NTL", "NTL", "NTL", "NTL", "NTL"},
	{"ntr", "NTR", "NTR", "NTR", "NTR", "NTR"},
	{"story", "剧情", "劇情", "Story", "ストーリー", "스토리"},
	{"motherly", "母系", "母系", "Motherly", "母系", "모성"},
	{"mature", "熟女", "熟女", "Mature women", "熟女", "숙녀"},
	{"submissive", "顺从", "順從", "Submissive", "従順", "순종"},
	{"wuxia", "武侠", "武俠", "Wuxia", "武侠", "무협"},
	{"western-fantasy", "西幻", "西幻", "Western fantasy", "西洋ファンタジー", "서양 판타지"},
	{"womens-fiction", "女频（bl/4i/abo/np）", "女頻（bl/4i/abo/np）", "Women’s fiction (BL/ABO/NP)", "女性向け（BL/ABO/NP）", "여성향(BL/ABO/NP)"},
	{"xianxia", "修仙", "修仙", "Xianxia", "仙侠", "선협"},
	{"mystery", "悬疑", "懸疑", "Mystery", "ミステリー", "미스터리"},
	{"onee-san", "御姐", "御姐", "Onee-san", "お姉さん", "누님"},
	{"redemption", "救赎", "救贖", "Redemption", "救済", "구원"},
	{"fantasy-scifi", "玄幻&科幻", "玄幻&科幻", "Fantasy & sci-fi", "ファンタジー&SF", "판타지&SF"},
	{"aesthetic", "唯美", "唯美", "Aesthetic", "耽美", "유미"},
	{"management-sim", "模拟经营", "模擬經營", "Management sim", "経営シミュレーション", "경영 시뮬레이션"},
	{"r18g", "R18G", "R18G", "R18G", "R18G", "R18G"},
	{"bestiality", "人兽", "人獸", "Bestiality", "獣姦", "수간"},
	{"gender-swap", "性转", "性轉", "Gender swap", "性転換", "성전환"},
	{"furry", "福瑞", "福瑞", "Furry", "ケモノ", "퍼리"},
	{"shota", "正太", "正太", "Shota", "ショタ", "쇼타"},
	{"school", "校园", "校園", "School", "学園", "학원"},
	{"blacked", "媚黑", "媚黑", "Blacked", "媚黒", "미흑"},
	{"sfw", "SFW", "SFW", "SFW", "SFW", "SFW"},
	{"ancient", "古风", "古風", "Ancient China", "古風", "고풍"},
	{"hardcore", "重口味", "重口味", "Hardcore", "ハード", "하드코어"},
	{"romance", "恋爱&攻略", "戀愛&攻略", "Romance & dating", "恋愛&攻略", "연애&공략"},
	{"femboy", "男娘", "男娘", "Femboy", "男の娘", "여장남자"},
	{"fanwork", "同人", "同人", "Fan work", "二次創作", "팬픽"},
	{"tomboy", "假小子", "假小子", "Tomboy", "ボーイッシュ", "톰보이"},
	{"futanari", "扶她", "扶她", "Futanari", "ふたなり", "후타나리"},
	{"beastfolk", "兽人", "獸人", "Beastfolk", "獣人", "수인"},
	{"harem", "后宫", "後宮", "Harem", "ハーレム", "하렘"},
	{"foot", "恋足", "戀足", "Foot fetish", "足フェチ", "발 페티시"},
	{"tool", "工具", "工具", "Tool", "ツール", "도구"},
	{"game", "游戏", "遊戲", "Game", "ゲーム", "게임"},
	{"absurd", "抽象", "抽象", "Absurd", "シュール", "병맛"},
	{"system", "系统", "系統", "System", "システム", "시스템"},
	{"secret", "隐奸", "隱姦", "Secret sex", "こっそり", "몰래"},
	{"exhibition", "露出", "露出", "Exhibitionism", "露出", "노출"},
	{"gap", "反差", "反差", "Gap", "ギャップ", "갭"},
	{"yuri", "百合", "百合", "Yuri", "百合", "백합"},
	{"erotic", "情欲肉爱", "情慾肉愛", "Erotic", "エロ", "에로"},
	{"humiliation", "羞辱", "羞辱", "Humiliation", "羞辱", "굴욕"},
}

var groupKeys = map[string][]string{
	"play":      {"roleplay", "world-sim", "gameplay-sim", "management-sim", "game", "system", "tool", "story", "romance", "absurd", "sfw"},
	"setting":   {"wuxia", "xianxia", "western-fantasy", "fantasy-scifi", "ancient", "school", "mystery", "fanwork", "aesthetic", "redemption", "womens-fiction", "female-pov"},
	"character": {"loli", "shota", "onee-san", "mature", "motherly", "femboy", "tomboy", "futanari", "gender-swap", "beastfolk", "furry", "yuri", "harem", "submissive", "gap"},
	"kink":      {"hypnosis", "taboo", "training", "ntl", "ntr", "r18g", "bestiality", "blacked", "hardcore", "foot", "secret", "exhibition", "erotic", "humiliation"},
}

var groupOf = func() map[string]string {
	m := make(map[string]string, len(entries))
	for g, keys := range groupKeys {
		for _, k := range keys {
			m[k] = g
		}
	}
	return m
}()

// ValidLocale reports whether loc is one of the five supported tag locales.
func ValidLocale(loc string) bool {
	for _, l := range Locales {
		if l == loc {
			return true
		}
	}
	return false
}

// Catalog returns the preset tag catalog; if lang is non-empty, Locale and
// each item's Name are populated for that language.
func Catalog(lang string) CatalogResponse {
	items := make([]Item, 0, len(entries))
	for _, e := range entries {
		names := map[string]string{
			"zh-Hans": e.zhHans,
			"zh-Hant": e.zhHant,
			"en":      e.en,
			"ja":      e.ja,
			"ko":      e.ko,
		}
		item := Item{
			Key:   e.key,
			Group: groupOf[e.key],
			Names: names,
		}
		if lang != "" {
			item.Name = names[lang]
		}
		items = append(items, item)
	}
	return CatalogResponse{
		Locale: lang,
		Groups: append([]string(nil), Groups...),
		Items:  items,
	}
}

// CountPresetTags counts how many preset catalog entries match any of the first
// StoredTagsMax non-blank trimmed strings in tags across all five locales.
func CountPresetTags(tags []string) int {
	candidates := make(map[string]struct{}, StoredTagsMax)
	seen := 0
	for _, raw := range tags {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		candidates[trimmed] = struct{}{}
		seen++
		if seen >= StoredTagsMax {
			break
		}
	}
	if len(candidates) == 0 {
		return 0
	}
	count := 0
	for _, e := range entries {
		for _, n := range [5]string{e.zhHans, e.zhHant, e.en, e.ja, e.ko} {
			if _, ok := candidates[n]; ok {
				count++
				break
			}
		}
	}
	return count
}

// HasPresetTag reports whether tags contains at least one preset tag display name.
func HasPresetTag(tags []string) bool {
	return CountPresetTags(tags) > 0
}
