package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/upuddu/purdue-equivalency-cli/internal/equiv"
)

// writer renders results in whichever format the flags selected.
type writer struct {
	out  io.Writer
	opts options
}

func (w writer) options(opts []equiv.Option) error {
	switch {
	case w.opts.json:
		return w.writeJSON(opts)
	case w.opts.csv:
		records := [][]string{{"value", "text"}}
		for _, o := range opts {
			records = append(records, []string{o.Value, o.Text})
		}
		return w.writeCSV(records)
	}

	if len(opts) == 0 {
		_, err := fmt.Fprintln(w.out, "no results")
		return err
	}
	tw := tabwriter.NewWriter(w.out, 0, 2, 2, ' ', 0)
	for _, o := range opts {
		if o.Text == o.Value {
			fmt.Fprintln(tw, o.Value)
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\n", o.Value, o.Text)
	}
	return tw.Flush()
}

func (w writer) equivalencies(rows []equiv.Equivalency) error {
	sortEquivalencies(rows)

	switch {
	case w.opts.json:
		return w.writeJSON(rows)
	case w.opts.csv:
		records := [][]string{{
			"state", "school", "subject", "course", "title", "credits",
			"purdue_subject", "purdue_course", "purdue_title",
		}}
		for _, r := range rows {
			records = append(records, []string{
				r.State, r.TransferSchool, r.TransferSubject, r.TransferCourse,
				r.TransferTitle, r.TransferCredits,
				r.PurdueSubject, r.PurdueCourse, r.PurdueTitle,
			})
		}
		return w.writeCSV(records)
	}

	if len(rows) == 0 {
		_, err := fmt.Fprintln(w.out, "no articulations on file")
		return err
	}
	tw := tabwriter.NewWriter(w.out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tSCHOOL\tCOURSE\tCR\tPURDUE")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s %s\t%s\t%s %s\n",
			r.State,
			truncate(r.TransferSchool, 32),
			join(r.TransferSubject, r.TransferCourse), truncate(r.TransferTitle, 30),
			r.TransferCredits,
			join(r.PurdueSubject, r.PurdueCourse), truncate(r.PurdueTitle, 28),
		)
	}
	return tw.Flush()
}

func (w writer) writeJSON(v any) error {
	enc := json.NewEncoder(w.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (w writer) writeCSV(records [][]string) error {
	cw := csv.NewWriter(w.out)
	if err := cw.WriteAll(records); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

// sortEquivalencies groups rows by state and school so a long sweep reads as a
// list of places rather than the order requests happened to return in.
func sortEquivalencies(rows []equiv.Equivalency) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.State != b.State {
			return a.State < b.State
		}
		if a.TransferSchool != b.TransferSchool {
			return a.TransferSchool < b.TransferSchool
		}
		return a.TransferCourse < b.TransferCourse
	})
}

func join(parts ...string) string {
	return strings.TrimSpace(strings.Join(parts, " "))
}

// truncate shortens s to at most limit runes, marking any elision.
func truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit-1]) + "…"
}
