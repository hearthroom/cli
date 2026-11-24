package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/output"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command {
		return []*cobra.Command{a.reviewCommand()}
	})
}

// The review commands are driven by agents, so they always print the server's
// JSON response, with or without --json.

var reviewCriteriaPattern = regexp.MustCompile(`^v\d{1,4}$`)

func (a *App) reviewCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "review",
		Short: "Staff tools for card review (the server decides who may use them)",
		Long: `Staff tools for card review submissions: list the queue, read a submission
and the evidence around it, and record an advisory suggestion or flags for one.
Suggestions do not decide a review. The server decides which accounts may use
each command.

Evidence: originality (overlap with other authors' cards), author (the author's
history and pace), author-cards (the author's other cards), card (a card's live
version) and author-note (your own notes on the author).

Every command prints the server's JSON response.`,
	}
	c.AddCommand(a.reviewQueue(), a.reviewShow(), a.reviewOriginality(), a.reviewAuthor(), a.reviewAuthorCards(),
		a.reviewCard(), a.reviewAuthorNote(), a.reviewSuggest(), a.reviewTag(), a.reviewReport())
	return c
}

func (a *App) reviewQueue() *cobra.Command {
	var scope, criteria string
	var unshadowed, count bool
	var limit int
	c := &cobra.Command{
		Use:   "queue",
		Short: "List review submissions",
		Long: `Lists review submissions. --scope pending (default) lists submissions waiting
for a decision; recent lists those submitted in the last 7 days, decided or not.
--unshadowed keeps only submissions you have not given a verdict for yet (for
the --criteria version, when given). --count prints {"count": n} instead of items.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if criteria != "" && !reviewCriteriaPattern.MatchString(criteria) {
				return output.Exitf(2, "--criteria must look like v1")
			}
			q := url.Values{"scope": {scope}, "limit": {strconv.Itoa(limit)}}
			if unshadowed {
				q.Set("unshadowed", "1")
			}
			if criteria != "" {
				q.Set("criteria", criteria)
			}
			if count {
				q.Set("count", "1")
			}
			return a.reviewCall(cmd, http.MethodGet, "/review-ai/queue?"+q.Encode(), nil)
		},
	}
	c.Flags().StringVar(&scope, "scope", "pending", "pending or recent")
	c.Flags().BoolVar(&unshadowed, "unshadowed", false, "only submissions you have not given a verdict for")
	c.Flags().StringVar(&criteria, "criteria", "", "criteria version for --unshadowed, e.g. v1")
	c.Flags().BoolVar(&count, "count", false, "print only the number of matching submissions")
	c.Flags().IntVar(&limit, "limit", 50, "maximum items (1-200)")
	return c
}

func (a *App) reviewShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show <submissionId>",
		Short: "Read one review submission with its card and the allowed suggestion codes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.reviewCall(cmd, http.MethodGet, "/review-ai/submissions/"+url.PathEscape(args[0]), nil)
		},
	}
}

func (a *App) reviewOriginality() *cobra.Command {
	return &cobra.Command{
		Use:   "originality <submissionId>",
		Short: "List other authors' cards whose setting overlaps this submission, with the overlapping text",
		Long: `Lists the cards by other authors whose character setting overlaps this
submission's setting: overall similarity, then for each similar card its card
number, name, similarity (0 to 1), whether it entered review earlier, its status
(approved, pending or removed) and the overlapping passages quoted from this
submission (at most 4,000 characters in all; overlapChars is the full length).

Use it when show reports a high originality similarity, to read what was copied
and from whom. The passages come from this submission, so they stay readable when
the other card is pending or removed. Open a listed card's live version with card.
The author's own cards are not listed; see author-cards for those.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.reviewCall(cmd, http.MethodGet, "/review-ai/submissions/"+url.PathEscape(args[0])+"/originality", nil)
		},
	}
}

func (a *App) reviewAuthor() *cobra.Command {
	return &cobra.Command{
		Use:   "author <submissionId>",
		Short: "Show the submitting author's history, pace, reviewer marks and your note on them",
		Long: `Shows evidence about the author of a submission, as numbers: account age in
days; earlier approved and rejected submissions with a count per rejection
reason; submissions of new cards and of updates to live cards in the last 7 and
30 days; new-card submissions in the last 24 hours; and how many accounts use
the device that registered this card. While the submission is pending it also
gives the author's tier, whether batch submission is suspected, and the marks
reviewers set on the author with their reason and note. Your own note on the
author is included (see author-note).

Counts run up to asOf: now for a pending submission, the submission time for a
decided one. Use it to judge mass production or a pattern of rejections. These
numbers may go in a suggestion's note (seen by reviewers only), never in its
authorNote (shown to the card's author).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.reviewCall(cmd, http.MethodGet, "/review-ai/submissions/"+url.PathEscape(args[0])+"/author", nil)
		},
	}
}

func (a *App) reviewAuthorCards() *cobra.Command {
	return &cobra.Command{
		Use:   "author-cards <submissionId>",
		Short: "List the author's other cards and how much of this setting each one shares",
		Long: `Lists the author's other cards, newest first: card number, name, status,
whether it is live, and its latest submission (id, kind, status, times and
length). This submission's own card is not listed.

settingSimilarity (0 to 1) is how much of this submission's setting also appears
in that card. Several cards near 1 mean one setting template reused under new
names. Only the 30 most recently registered cards are compared. null means that
card was not compared or its text is not kept (rejected versions are dropped).

Open a card's latest submission with show <latest.id>, or its live version with
card <cardId>.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.reviewCall(cmd, http.MethodGet, "/review-ai/submissions/"+url.PathEscape(args[0])+"/author-cards", nil)
		},
	}
}

var reviewCardNumberPattern = regexp.MustCompile(`^\d{1,12}$`)

func (a *App) reviewCard() *cobra.Command {
	return &cobra.Command{
		Use:   "card <cardNumber>",
		Short: "Read the version of a card that is live now",
		Long: `Reads the version of a card that players see now, in the same card and stats
shape as show. Fails with card_not_live when the card is not approved or is
hidden from the public.

Use it for other cards, from author-cards or originality, to compare them with
the submission you are judging. To read a submission itself, use show.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !reviewCardNumberPattern.MatchString(args[0]) {
				return output.Exitf(2, "card number must be digits, e.g. 100123")
			}
			return a.reviewCall(cmd, http.MethodGet, "/review-ai/cards/"+args[0], nil)
		},
	}
}

func (a *App) reviewAuthorNote() *cobra.Command {
	var set, file string
	c := &cobra.Command{
		Use:   "author-note <submissionId> [--set TEXT | --file PATH|-]",
		Short: "Read or replace your private note on the submission's author",
		Long: `Reads your note on the author of a submission. With --set or --file it
replaces the note (up to 2,000 characters); an empty note deletes it.

There is one note per author, shared by all of that author's submissions, and
author also returns it. Authors never see it. Use it to keep what you learned
about an author across reviews, such as a reused setting template or a pattern
in earlier rejections. Read the note first and include what should stay, since
writing replaces it.

--file reads plain text from a file, or from stdin with -.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/review-ai/submissions/" + url.PathEscape(args[0]) + "/author-note"
			f := cmd.Flags()
			if f.Changed("set") && f.Changed("file") {
				return output.Exitf(2, "use --set or --file, not both")
			}
			switch {
			case f.Changed("set"):
				return a.reviewCall(cmd, http.MethodPut, path, map[string]any{"note": set})
			case f.Changed("file"):
				var raw []byte
				var err error
				if file == "-" {
					raw, err = io.ReadAll(a.In)
				} else {
					raw, err = os.ReadFile(file)
				}
				if err != nil {
					return output.Exit(2, err)
				}
				return a.reviewCall(cmd, http.MethodPut, path, map[string]any{"note": string(raw)})
			}
			return a.reviewCall(cmd, http.MethodGet, path, nil)
		},
	}
	c.Flags().StringVar(&set, "set", "", "replace the note with this text (empty deletes it)")
	c.Flags().StringVar(&file, "file", "", "replace the note with the text of this file ('-' for stdin)")
	return c
}

func (a *App) reviewSuggest() *cobra.Command {
	var file, criteria, model, verdict, note, handoffCause string
	var reasons, flags []string
	var confidence, costUSD float64
	var inputTokens, outputTokens, latencyMS int64
	c := &cobra.Command{
		Use:   "suggest <submissionId>",
		Short: "Record an advisory suggestion for a review submission",
		Long: `Records your suggestion for a submission: a verdict (pass, reject or handoff),
reason and flag codes, confidence and a short note. It is advisory only and does
not decide the review. Sending again for the same submission and criteria
version replaces your earlier suggestion.

Use flags, or --file with a JSON body (- reads stdin). Flags given on the command
line replace the matching fields of the file. Allowed codes are listed by
review show.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if file != "" {
				var err error
				if body, err = a.readReviewBody(file); err != nil {
					return err
				}
			}
			f := cmd.Flags()
			set := func(flag, key string, v any) {
				if f.Changed(flag) {
					body[key] = v
				}
			}
			set("criteria", "criteriaVersion", criteria)
			set("model", "model", model)
			set("verdict", "verdict", verdict)
			set("reason", "reasons", nonNilStrings(reasons))
			set("flag", "flags", nonNilStrings(flags))
			set("confidence", "confidence", confidence)
			set("note", "note", note)
			set("handoff-cause", "handoffCause", handoffCause)
			set("input-tokens", "inputTokens", inputTokens)
			set("output-tokens", "outputTokens", outputTokens)
			set("cost-usd", "costUsd", costUSD)
			set("latency-ms", "latencyMs", latencyMS)
			if file == "" {
				for _, key := range []string{"reasons", "flags"} {
					if _, ok := body[key]; !ok {
						body[key] = []string{}
					}
				}
			}
			if err := checkReviewCriteria(body); err != nil {
				return err
			}
			switch body["verdict"] {
			case "pass", "reject", "handoff":
			default:
				return output.Exitf(2, "verdict must be pass, reject or handoff")
			}
			return a.reviewCall(cmd, http.MethodPut, "/review-ai/submissions/"+url.PathEscape(args[0])+"/suggestion", body)
		},
	}
	c.Flags().StringVar(&file, "file", "", "JSON body to send ('-' for stdin)")
	c.Flags().StringVar(&criteria, "criteria", "", "criteria version, e.g. v1")
	c.Flags().StringVar(&model, "model", "", "model that produced the suggestion")
	c.Flags().StringVar(&verdict, "verdict", "", "pass, reject or handoff")
	c.Flags().StringArrayVar(&reasons, "reason", nil, "reason code (repeatable)")
	c.Flags().StringArrayVar(&flags, "flag", nil, "flag code (repeatable)")
	c.Flags().Float64Var(&confidence, "confidence", 0, "confidence from 0 to 1")
	c.Flags().StringVar(&note, "note", "", "short note (up to 400 characters)")
	c.Flags().StringVar(&handoffCause, "handoff-cause", "", "why the submission is handed to a person: unsure, refusal, invalid_output, timeout, error or too_large")
	c.Flags().Int64Var(&inputTokens, "input-tokens", 0, "model input tokens")
	c.Flags().Int64Var(&outputTokens, "output-tokens", 0, "model output tokens")
	c.Flags().Float64Var(&costUSD, "cost-usd", 0, "model cost in US dollars")
	c.Flags().Int64Var(&latencyMS, "latency-ms", 0, "model latency in milliseconds")
	return c
}

func (a *App) reviewTag() *cobra.Command {
	var criteria, model string
	var flags []string
	c := &cobra.Command{
		Use:   "tag <submissionId>",
		Short: "Set flag codes on a review submission without changing your verdict",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"criteriaVersion": criteria, "model": model, "flags": nonNilStrings(flags)}
			if err := checkReviewCriteria(body); err != nil {
				return err
			}
			return a.reviewCall(cmd, http.MethodPut, "/review-ai/submissions/"+url.PathEscape(args[0])+"/tags", body)
		},
	}
	c.Flags().StringVar(&criteria, "criteria", "", "criteria version, e.g. v1")
	c.Flags().StringVar(&model, "model", "", "model that produced the flags")
	c.Flags().StringArrayVar(&flags, "flag", nil, "flag code (repeatable)")
	return c
}

func (a *App) reviewReport() *cobra.Command {
	var from, to int64
	var criteria string
	c := &cobra.Command{
		Use:   "report",
		Short: "Summarize suggestions by criteria version over a time range",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if criteria != "" && !reviewCriteriaPattern.MatchString(criteria) {
				return output.Exitf(2, "--criteria must look like v1")
			}
			q := url.Values{}
			if cmd.Flags().Changed("from") {
				q.Set("from", strconv.FormatInt(from, 10))
			}
			if cmd.Flags().Changed("to") {
				q.Set("to", strconv.FormatInt(to, 10))
			}
			if criteria != "" {
				q.Set("criteria", criteria)
			}
			path := "/review-ai/report"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			return a.reviewCall(cmd, http.MethodGet, path, nil)
		},
	}
	c.Flags().Int64Var(&from, "from", 0, "start time in Unix milliseconds")
	c.Flags().Int64Var(&to, "to", 0, "end time in Unix milliseconds")
	c.Flags().StringVar(&criteria, "criteria", "", "criteria version, e.g. v1")
	return c
}

// EnvReviewKey holds the card-review identity's fixed key. When set, every
// review command except report sends it instead of signing in; report stays a
// signed-in manager command.
const EnvReviewKey = "HEARTHROOM_REVIEW_KEY"

// reviewCall sends one community request and prints the response.
func (a *App) reviewCall(cmd *cobra.Command, method, path string, body any) error {
	var raw json.RawMessage
	var err error
	if key := strings.TrimSpace(os.Getenv(EnvReviewKey)); key != "" && !strings.HasPrefix(path, "/review-ai/report") {
		err = a.Client().SiteDoKey(cmd.Context(), method, path, http.Header{"X-Review-Agent-Key": {key}}, body, &raw)
	} else {
		if err := a.RequireAuth(); err != nil {
			return err
		}
		err = a.Client().SiteDo(cmd.Context(), method, path, body, &raw)
	}
	if err != nil {
		return withErrorField(err)
	}
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	return a.Out.JSONValue(raw)
}

// withErrorField copies the response's "field" into the error detail so the
// printed error names the rejected field next to the code.
func withErrorField(err error) error {
	var e *api.Error
	if !errors.As(err, &e) || e.Detail != nil || len(e.Envelope) == 0 {
		return err
	}
	var env struct {
		Field string `json:"field"`
	}
	if json.Unmarshal(e.Envelope, &env) == nil && env.Field != "" {
		e.Detail = map[string]any{"field": env.Field}
	}
	return err
}

func (a *App) readReviewBody(path string) (map[string]any, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(a.In)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, output.Exit(2, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil || body == nil {
		return nil, output.Exitf(2, "--file must hold one JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, output.Exitf(2, "--file must hold one JSON object")
	}
	return body, nil
}

func checkReviewCriteria(body map[string]any) error {
	if s, ok := body["criteriaVersion"].(string); !ok || !reviewCriteriaPattern.MatchString(s) {
		return output.Exitf(2, "criteria version must look like v1 (--criteria)")
	}
	return nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
