package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/sync"
)

// questionnaire is GET /v1/rating/questionnaire.
type questionnaire struct {
	Version  int    `json:"version"`
	Locale   string `json:"locale"`
	Intro    string `json:"intro"`
	PickRule string `json:"pickRule"`
	Ratings  []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"ratings"`
	Topics []ratingTopic `json:"topics"`
	Other  struct {
		Title   string         `json:"title"`
		Options []ratingOption `json:"options"`
	} `json:"other"`
}

type ratingTopic struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Options []ratingOption `json:"options"`
}

type ratingOption struct {
	ID     string `json:"id"`
	Rating string `json:"rating"`
	Text   string `json:"text"`
}

// ratingResult is what the site computes from answers.
type ratingResult struct {
	Rating      string             `json:"rating"`
	Descriptors []string           `json:"descriptors"`
	Answers     card.RatingAnswers `json:"answers"`
}

// ratingLocales are the questionnaire's languages.
var ratingLocales = []string{"zh-Hant", "zh-Hans", "en", "ja", "ko"}

// defaultRatingLocale picks the questionnaire language from the environment
// (LC_ALL, LC_MESSAGES, LANG), then the card's language, then English.
func defaultRatingLocale(cardLanguage string) string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if l := localeFromEnv(os.Getenv(k)); l != "" {
			return l
		}
	}
	for _, l := range ratingLocales {
		if l == cardLanguage {
			return l
		}
	}
	return "en"
}

func localeFromEnv(v string) string {
	v = strings.ToLower(strings.SplitN(strings.SplitN(v, ".", 2)[0], "@", 2)[0])
	v = strings.ReplaceAll(v, "-", "_")
	switch {
	case v == "" || v == "c" || v == "posix":
		return ""
	case strings.HasPrefix(v, "zh_tw"), strings.HasPrefix(v, "zh_hk"), strings.HasPrefix(v, "zh_mo"), strings.HasPrefix(v, "zh_hant"):
		return "zh-Hant"
	case strings.HasPrefix(v, "zh"):
		return "zh-Hans"
	case strings.HasPrefix(v, "ja"):
		return "ja"
	case strings.HasPrefix(v, "ko"):
		return "ko"
	case strings.HasPrefix(v, "en"):
		return "en"
	}
	return ""
}

func (a *App) cardRate() *cobra.Command {
	var locale, answersPath string
	var questions bool
	c := &cobra.Command{
		Use:   "rate [dir]",
		Short: "Answer the content-rating questionnaire and write rating.json",
		Long: `Submitting a card for review needs a content rating. The questionnaire follows
Taiwan's game software rating (GSRR): pick which content types the card has, then
for each one the closest option that is not milder than the actual content, then
one question about anything else. The site computes the rating (G, P, PG12, PG15,
R) from the answers; the CLI only sends them.

In a terminal the command asks the questions. Without one, read the questionnaire
with --questions --json, write the answers as JSON and pass them with --answers
(a file, or - for stdin):

  {"version": 1, "topics": {"violence": "violence.bloody"}, "other": "other.none"}

topics lists only the content types the card has ({} for none); other is required.
The answers are checked by the site and written to rating.json in the folder,
which card submit sends with the card. rating.json never goes to the provider.
When the folder is linked to a card you own and you are signed in, the answers
are also saved as the website's rating draft for that card.

The questionnaire language is --locale, else LC_ALL / LC_MESSAGES / LANG, else
the card's language, else English. Rating is free and never submits the card.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if locale != "" && !contains(ratingLocales, locale) {
				return output.Exitf(2, "--locale must be one of %s", strings.Join(ratingLocales, ", "))
			}
			var f *card.Folder
			if !questions || len(args) > 0 {
				var err error
				if f, err = loadFolder(args); err != nil {
					return err
				}
			}
			if locale == "" {
				lang := ""
				if f != nil {
					lang = f.Manifest.Language
				}
				locale = defaultRatingLocale(lang)
			}
			if questions {
				raw, err := a.fetchQuestionnaire(ctx, locale)
				if err != nil {
					return err
				}
				if a.Out.JSON {
					return a.Out.JSONValue(raw)
				}
				var q questionnaire
				if err := json.Unmarshal(raw, &q); err != nil {
					return err
				}
				a.printQuestionnaire(&q)
				return nil
			}

			var answers json.RawMessage
			switch {
			case answersPath != "":
				var err error
				if answers, err = a.readAnswers(answersPath); err != nil {
					return output.Exit(2, err)
				}
			case a.Interactive():
				raw, err := a.fetchQuestionnaire(ctx, locale)
				if err != nil {
					return err
				}
				var q questionnaire
				if err := json.Unmarshal(raw, &q); err != nil {
					return err
				}
				picked, err := a.askRating(&q, bufio.NewReader(a.In))
				if err != nil {
					return err
				}
				if answers, err = json.Marshal(picked); err != nil {
					return err
				}
			default:
				return output.Exitf(2, "no terminal to ask the questions in: run `hearthroom card rate --questions --json` to read the questionnaire, then `hearthroom card rate %s --answers <file|->` with the answers as JSON", dirArg(args))
			}

			var res ratingResult
			if err := a.Client().SitePost(ctx, "/rating/evaluate", answers, &res, false); err != nil {
				return ratingError(err, dirArg(args))
			}
			if err := card.WriteRating(f.Dir, &card.Rating{Answers: res.Answers, Rating: res.Rating, Descriptors: res.Descriptors}); err != nil {
				return err
			}
			file := filepath.Join(f.Dir, card.RatingFile)
			a.saveRatingDraft(cmd, f, res.Answers)
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"rating": res.Rating, "descriptors": orEmpty(res.Descriptors), "answers": res.Answers, "file": file})
			}
			if len(res.Descriptors) > 0 {
				a.Out.Line("Rating: %s (%s)", res.Rating, strings.Join(res.Descriptors, ", "))
			} else {
				a.Out.Line("Rating: %s", res.Rating)
			}
			a.Out.Line("Wrote %s. `hearthroom card submit %s` sends it with the card.", file, dirArg(args))
			return nil
		},
	}
	c.Flags().BoolVar(&questions, "questions", false, "print the questionnaire (with --json, as the site returns it) and exit")
	c.Flags().StringVar(&answersPath, "answers", "", "answers JSON file, or - for stdin, instead of asking")
	c.Flags().StringVar(&locale, "locale", "", "questionnaire language: "+strings.Join(ratingLocales, ", "))
	return c
}

// fetchQuestionnaire reads the site's questionnaire in one locale.
func (a *App) fetchQuestionnaire(ctx context.Context, locale string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := a.Client().SiteGet(ctx, "/rating/questionnaire", url.Values{"locale": {locale}}, &raw, false); err != nil {
		return nil, err
	}
	var probe struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(raw, &probe) != nil || probe.Version == 0 {
		return nil, fmt.Errorf("%s did not return a rating questionnaire; the site may not support content ratings yet", a.Site)
	}
	return raw, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// readAnswers reads answers JSON from a file or stdin. A rating.json (answers
// under "answers") is accepted too.
func (a *App) readAnswers(path string) (json.RawMessage, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(a.In)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("--answers: not a JSON object: %w", err)
	}
	if inner, ok := probe["answers"]; ok {
		if _, versioned := probe["version"]; !versioned {
			return inner, nil
		}
	}
	return json.RawMessage(b), nil
}

// ratingError explains the site's answer errors in terms of what to run next.
func ratingError(err error, dir string) error {
	var ae *api.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case "rating_version_outdated", "rating_answers_invalid", "rating_required":
			return output.Exit(2, fmt.Errorf("%w: read the current questionnaire with `hearthroom card rate --questions --json` and answer again with `hearthroom card rate %s`", err, dir))
		}
	}
	return err
}

// saveRatingDraft keeps the answers as the website's draft for an owned card.
// rating.json is the source; a failure here is only reported.
func (a *App) saveRatingDraft(cmd *cobra.Command, f *card.Folder, answers card.RatingAnswers) {
	if f.State.Target != "owned" || f.State.RoleID == "" || !auth.HasCredential(a.Store, a.API) {
		return
	}
	if err := a.Client().SiteDo(cmd.Context(), "PUT", "/cards/"+url.PathEscape(f.State.RoleID)+"/rating-draft", answers, nil); err != nil {
		a.Out.Note("The website's rating draft was not saved (%v); rating.json is written and card submit sends it.", err)
		return
	}
	a.Out.Note("Saved as the website's rating draft for card %s.", f.State.RoleID)
}

func (a *App) printQuestionnaire(q *questionnaire) {
	a.Out.Line("%s", q.Intro)
	a.Out.Line("")
	a.Out.Line("%s", q.PickRule)
	names := make([]string, 0, len(q.Ratings))
	for _, r := range q.Ratings {
		names = append(names, r.ID+" "+r.Name)
	}
	a.Out.Line("Ratings, low to high: %s", strings.Join(names, " · "))
	for _, t := range q.Topics {
		a.Out.Line("")
		a.Out.Line("%s  (topic %s)", t.Title, t.ID)
		for _, o := range t.Options {
			a.Out.Line("  %-24s %-5s %s", o.ID, o.Rating, o.Text)
		}
	}
	a.Out.Line("")
	a.Out.Line("%s  (other, required)", q.Other.Title)
	for _, o := range q.Other.Options {
		a.Out.Line("  %-24s %-5s %s", o.ID, o.Rating, o.Text)
	}
	a.Out.Line("")
	a.Out.Line(`Answers (version %d): {"version": %d, "topics": {"<topic>": "<option id>"}, "other": "<option id>"}`, q.Version, q.Version)
	a.Out.Line("List only the topics the card has. Pass the file with `hearthroom card rate <dir> --answers <file>`.")
}

// askRating asks the three steps on stderr and reads numbers from in.
func (a *App) askRating(q *questionnaire, in *bufio.Reader) (card.RatingAnswers, error) {
	ans := card.RatingAnswers{Version: q.Version, Topics: map[string]string{}}
	say := func(format string, args ...any) { a.Out.Note(format, args...) }
	if q.Intro != "" {
		say("%s", q.Intro)
		say("")
	}
	say("Step 1. Content types: numbers separated by spaces, 0 for none.")
	for i, t := range q.Topics {
		say("  %d. %s", i+1, t.Title)
	}
	var picked []ratingTopic
	for picked == nil {
		line, err := readAnswer(in, "Topics: ", a)
		if err != nil {
			return ans, err
		}
		picked = parseTopics(line, q.Topics)
		if picked == nil {
			say("Enter numbers from 1 to %d, or 0 for none.", len(q.Topics))
		}
	}
	if len(picked) > 0 {
		say("")
		say("Step 2. %s", q.PickRule)
	}
	for _, t := range picked {
		o, err := a.askOption(in, t.Title, t.Options)
		if err != nil {
			return ans, err
		}
		ans.Topics[t.ID] = o.ID
	}
	say("")
	say("Step 3.")
	o, err := a.askOption(in, q.Other.Title, q.Other.Options)
	if err != nil {
		return ans, err
	}
	ans.Other = o.ID
	return ans, nil
}

func (a *App) askOption(in *bufio.Reader, title string, options []ratingOption) (ratingOption, error) {
	a.Out.Note("%s", title)
	for i, o := range options {
		a.Out.Note("  %d. %s", i+1, o.Text)
	}
	for {
		line, err := readAnswer(in, "Option: ", a)
		if err != nil {
			return ratingOption{}, err
		}
		if n, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && n >= 1 && n <= len(options) {
			return options[n-1], nil
		}
		a.Out.Note("Enter a number from 1 to %d.", len(options))
	}
}

func readAnswer(in *bufio.Reader, prompt string, a *App) (string, error) {
	fmt.Fprint(a.Out.Err, prompt)
	line, err := in.ReadString('\n')
	if err != nil && (line == "" || !errors.Is(err, io.EOF)) {
		if errors.Is(err, io.EOF) {
			return "", output.Exitf(2, "no answer given; to answer without a terminal use `hearthroom card rate --questions --json` and --answers")
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// parseTopics reads "1 3" or "1,3" (or "0" for none); nil means invalid.
func parseTopics(line string, topics []ratingTopic) []ratingTopic {
	fields := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == ',' || r == '，' || r == '、' })
	if len(fields) == 0 {
		return nil
	}
	if len(fields) == 1 && fields[0] == "0" {
		return []ratingTopic{}
	}
	seen := map[int]bool{}
	out := []ratingTopic{}
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(topics) {
			return nil
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, topics[n-1])
		}
	}
	return out
}

func (a *App) cardSubmit() *cobra.Command {
	var fandom string
	c := &cobra.Command{
		Use:   "submit [dir]",
		Short: "Submit the linked card for review on the community site, with its content rating",
		Long: `Sends the card the folder is linked to, as last pushed, for review on the
community site, together with the answers in rating.json (see card rate). Only
a card you own can be submitted: a trial card cannot, so run
"hearthroom card push --create" first to make a private card from the folder.
Local edits that were not pushed are not part of the submission.

Each submission carries an operation id kept in .hearthroom/state.json until
the site accepts it, so running submit again after a lost reply does not submit
twice. --fandom names the work the card is based on, if any.

Submitting publishes the card once reviewers approve it and may use your weekly
listing quota. Run it only when the author asks.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loadFolder(args)
			if err != nil {
				return err
			}
			dir := dirArg(args)
			rating, err := card.ReadRating(f.Dir)
			if card.IsNotExist(err) {
				return output.Exitf(2, "no %s in %s: run `hearthroom card rate %s` first; review needs the content rating", card.RatingFile, dir, dir)
			}
			if err != nil {
				return output.Exit(2, fmt.Errorf("%w; run `hearthroom card rate %s` again", err, dir))
			}
			switch {
			case f.State.RoleID == "":
				return output.Exitf(2, "this folder is not linked to a card: `hearthroom card push --create %s` makes a private card you own, then submit it", dir)
			case f.State.Target != "owned":
				return output.Exitf(2, "this folder is linked to trial card %s, which cannot be submitted for review: `hearthroom card push --create %s` makes a private card you own, then submit it", f.State.RoleID, dir)
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			if changed := unpushedSections(f); len(changed) > 0 {
				a.Out.Note("Not pushed yet: %s. The review sees the card as last pushed; run `hearthroom card push %s` first to include them.", strings.Join(changed, ", "), dir)
			}

			fandom = strings.TrimSpace(fandom)
			digest := card.Digest(map[string]any{"roleId": f.State.RoleID, "answers": rating.Answers, "fandom": fandom})
			sub := f.State.Submission
			if sub == nil || sub.RoleID != f.State.RoleID || sub.Digest != digest {
				id, err := newUUID()
				if err != nil {
					return err
				}
				sub = &card.Submission{RoleID: f.State.RoleID, OperationID: id, Digest: digest}
				f.State.Submission = sub
				if err := f.SaveState(); err != nil {
					return err
				}
			}
			body := map[string]any{"roleId": f.State.RoleID, "ratingAnswers": rating.Answers, "operationId": sub.OperationID}
			if fandom != "" {
				body["fandom"] = fandom
			}
			var raw json.RawMessage
			if err := a.Client().SiteDo(cmd.Context(), "POST", "/cards", body, &raw); err != nil {
				return submitError(err, dir)
			}
			f.State.Submission = nil
			if err := f.SaveState(); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var out struct {
				Status    string `json:"status"`
				VersionID string `json:"versionId"`
			}
			_ = json.Unmarshal(raw, &out)
			a.Out.Line("Submitted card %s for review with rating %s: %s", f.State.RoleID, rating.Rating, out.Status)
			if out.VersionID != "" {
				a.Out.Line("  version: %s", out.VersionID)
			}
			return nil
		},
	}
	c.Flags().StringVar(&fandom, "fandom", "", "the work the card is based on, if any")
	return c
}

// unpushedSections lists the sections that changed since the last push.
func unpushedSections(f *card.Folder) []string {
	urls := map[string]string{}
	for p, as := range f.State.Assets {
		urls[p] = as.URL
	}
	p, err := f.Build(urls)
	if err != nil {
		return nil
	}
	digests := p.Digests()
	var changed []string
	for _, s := range card.Sections {
		if _, ok := digests[s]; ok && f.State.LocalHashes[s] != sync.OwnedDigest(f, digests, s) {
			changed = append(changed, s)
		}
	}
	return changed
}

func submitError(err error, dir string) error {
	var ae *api.Error
	if !errors.As(err, &ae) {
		return err
	}
	switch ae.Code {
	case "rating_required", "rating_version_outdated", "rating_answers_invalid":
		return ratingError(err, dir)
	case "submission_pending":
		return output.Exit(2, fmt.Errorf("%w: the card is already waiting for review", err))
	case "weekly_quota_exceeded":
		return output.Exit(2, fmt.Errorf("%w: this week's listing quota is used up", err))
	case "publication_use_original":
		return output.Exit(2, fmt.Errorf("%w: this card is a copy of one already listed; submit the original card instead", err))
	}
	return err
}

// newUUID returns a random (version 4) UUID.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
