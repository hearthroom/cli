package cli

import (
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/preview"
)

func reviewEvidencePath(id, evidence string, query url.Values) string {
	path := "/review-ai/submissions/" + url.PathEscape(id) + "/" + evidence
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path
}

func (a *App) reviewOpening() *cobra.Command {
	var surface, screenshot string
	var offset int
	c := &cobra.Command{
		Use:   "opening <submissionId>",
		Short: "Read the rendered opening text one page at a time",
		Long: `Read a bounded static rendering of an opening. Use nextOffset for the next
page. --screenshot saves an offline static viewport PNG to a new file. That
preview includes static HTML and CSS. Scripts and external assets are disabled.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{"surface": {surface}}
			if screenshot == "" {
				q.Set("offset", strconv.Itoa(offset))
				return a.reviewCall(cmd, http.MethodGet, reviewEvidencePath(args[0], "opening", q), nil)
			}
			if cmd.Flags().Changed("offset") {
				return output.Exitf(2, "--offset applies to opening text")
			}
			q.Set("format", "preview")
			var result struct {
				State, Surface, Coverage, PreviewKind, PreviewHTML string
			}
			if err := a.reviewRequest(cmd, http.MethodGet, reviewEvidencePath(args[0], "opening", q), nil, &result); err != nil {
				return err
			}
			if result.State != "ready" || result.PreviewKind != "offline_static" || result.PreviewHTML == "" {
				return output.Exitf(1, "opening preview unavailable")
			}
			png, err := preview.ReviewOpeningScreenshot(cmd.Context(), result.PreviewHTML)
			if err != nil {
				return err
			}
			file, err := os.OpenFile(screenshot, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			_, writeErr := file.Write(png)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				_ = os.Remove(screenshot)
				if writeErr != nil {
					return writeErr
				}
				return closeErr
			}
			return a.Out.JSONValue(map[string]string{"surface": result.Surface, "coverage": result.Coverage, "previewKind": result.PreviewKind, "screenshot": screenshot})
		},
	}
	c.Flags().StringVar(&surface, "surface", "primary", "primary, welcome, page or alternate-N")
	c.Flags().IntVar(&offset, "offset", 0, "continuation offset returned by the preceding text page")
	c.Flags().StringVar(&screenshot, "screenshot", "", "save a private offline static PNG to a new file (requires Chrome)")
	return c
}

func (a *App) reviewPrecedents() *cobra.Command {
	var reason, cardType string
	var before int64
	c := &cobra.Command{
		Use:   "precedents <submissionId>",
		Short: "Read earlier human decisions by review reason or card type tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if reason = strings.TrimSpace(reason); reason != "" {
				q.Set("reason", reason)
			}
			if cardType = strings.TrimSpace(cardType); cardType != "" {
				q.Set("type", cardType)
			}
			if len(q) == 0 {
				return output.Exitf(2, "provide --reason or --type")
			}
			if cmd.Flags().Changed("before") {
				q.Set("before", strconv.FormatInt(before, 10))
			}
			return a.reviewCall(cmd, http.MethodGet, reviewEvidencePath(args[0], "precedents", q), nil)
		},
	}
	c.Flags().StringVar(&reason, "reason", "", "human review reason or improvement suggestion code")
	c.Flags().StringVar(&cardType, "type", "", "exact card type tag")
	c.Flags().Int64Var(&before, "before", 0, "optional replay cutoff in Unix milliseconds (exclusive)")
	return c
}

func (a *App) reviewAuthorHistory() *cobra.Command {
	var before int64
	c := &cobra.Command{
		Use:   "author-history <submissionId>",
		Short: "Read the author's earlier decisions and published-card quality signals",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if cmd.Flags().Changed("before") {
				q.Set("before", strconv.FormatInt(before, 10))
			}
			return a.reviewCall(cmd, http.MethodGet, reviewEvidencePath(args[0], "author-history", q), nil)
		},
	}
	c.Flags().Int64Var(&before, "before", 0, "optional replay cutoff in Unix milliseconds (exclusive)")
	return c
}

func (a *App) reviewDiff() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <submissionId>",
		Short: "Read changed excerpts against the sealed approval or rejection baseline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.reviewCall(cmd, http.MethodGet, reviewEvidencePath(args[0], "diff", nil), nil)
		},
	}
}
