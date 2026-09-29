// Command gendocs writes the CLI manual as Markdown for the website.
//
// Every page is generated from the live cobra command tree, so the site says
// exactly what `hearthroom --help` says for the version that was built.
//
//	go run ./cmd/gendocs -out site/src/content/manual -version v0.1.5
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/cli"
)

type page struct {
	Slug     string   `json:"slug"`    // e.g. "card/push"
	Command  string   `json:"command"` // e.g. "hearthroom card push"
	Short    string   `json:"short"`
	Parent   string   `json:"parent,omitempty"`
	Children []string `json:"children,omitempty"`
}

func main() {
	out := flag.String("out", "site/src/content/manual", "output directory")
	version := flag.String("version", "dev", "version string shown in the manual")
	flag.Parse()
	root := cli.NewRoot(cli.BuildInfo{Version: *version})
	root.InitDefaultHelpCmd()
	if err := os.RemoveAll(*out); err != nil {
		fail(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	var pages []page
	walk(root, *out, &pages)
	sort.Slice(pages, func(i, j int) bool { return pages[i].Slug < pages[j].Slug })
	idx, _ := json.MarshalIndent(pages, "", "  ")
	if err := os.WriteFile(filepath.Join(*out, "index.json"), append(idx, '\n'), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %d manual pages to %s\n", len(pages), *out)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gendocs:", err)
	os.Exit(1)
}

func slugOf(c *cobra.Command) string {
	path := strings.TrimPrefix(c.CommandPath(), "hearthroom")
	path = strings.TrimSpace(path)
	if path == "" {
		return "hearthroom"
	}
	return strings.ReplaceAll(path, " ", "/")
}

func walk(c *cobra.Command, out string, pages *[]page) {
	if c.Hidden || c.Name() == "help" {
		return
	}
	p := page{Slug: slugOf(c), Command: c.CommandPath(), Short: c.Short}
	if c.HasParent() {
		p.Parent = slugOf(c.Parent())
	}
	for _, sub := range c.Commands() {
		if !sub.Hidden && sub.Name() != "help" {
			p.Children = append(p.Children, slugOf(sub))
		}
	}
	*pages = append(*pages, p)
	if err := os.WriteFile(filepath.Join(out, strings.ReplaceAll(p.Slug, "/", "__")+".md"), []byte(render(c, p)), 0o644); err != nil {
		fail(err)
	}
	for _, sub := range c.Commands() {
		walk(sub, out, pages)
	}
}

// render writes one page: front matter for the site, then the same sections
// `--help` prints, in Markdown.
func render(c *cobra.Command, p page) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nslug: %q\ncommand: %q\nshort: %q\n", p.Slug, p.Command, p.Short)
	if p.Parent != "" {
		fmt.Fprintf(&b, "parent: %q\n", p.Parent)
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", p.Command)
	long := strings.TrimSpace(c.Long)
	if long == "" {
		long = c.Short
	}
	b.WriteString(long + "\n\n")
	if c.Runnable() {
		fmt.Fprintf(&b, "## Usage\n\n```\n%s\n```\n\n", c.UseLine())
	}
	if c.Example != "" {
		fmt.Fprintf(&b, "## Examples\n\n```\n%s\n```\n\n", strings.TrimSpace(c.Example))
	}
	if len(p.Children) > 0 {
		b.WriteString("## Commands\n\n")
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" {
				continue
			}
			fmt.Fprintf(&b, "- [`%s`](/manual/%s/) — %s\n", sub.CommandPath(), slugOf(sub), sub.Short)
		}
		b.WriteString("\n")
	}
	if flags := strings.TrimRight(c.NonInheritedFlags().FlagUsages(), "\n"); strings.TrimSpace(flags) != "" {
		fmt.Fprintf(&b, "## Options\n\n```\n%s\n```\n\n", flags)
	}
	if flags := strings.TrimRight(c.InheritedFlags().FlagUsages(), "\n"); strings.TrimSpace(flags) != "" {
		fmt.Fprintf(&b, "## Global options\n\n```\n%s\n```\n\n", flags)
	}
	if c.HasParent() {
		fmt.Fprintf(&b, "## See also\n\n- [`%s`](/manual/%s/) — %s\n", c.Parent().CommandPath(), slugOf(c.Parent()), c.Parent().Short)
	}
	return b.String()
}
