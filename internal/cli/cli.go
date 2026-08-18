// Package cli implements the ptc command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/upuddu/purdue-equivalency-cli/internal/equiv"
)

const usage = `ptc queries Purdue's Transfer Credit Course Equivalency Guide.

usage: ptc <command> [flags] [arguments]

Reverse lookup — start from the Purdue course you need:
  who <SUBJ> <NUM>       schools with an articulation for that Purdue course
  states <SUBJ> <NUM>    states that have one; cheaper than a full 'who' sweep
  courses <SUBJ>         Purdue courses in a subject that have any articulation
  subjects               Purdue subject codes

Forward lookup — start from a school:
  schools <STATE>        schools in a state, with their school codes
  offerings <SCHOOL>     subjects a school has articulations for
  equiv <SCHOOL> <SUBJ> [NUM] --state XX
                         what a school's course becomes at Purdue

Flags:
  --state XX     limit 'who' to one state; required by 'equiv'
  --outside-us   read STATE as a country outside the US
  --json         emit JSON
  --csv          emit CSV

The guide lists only articulations Purdue has already evaluated, and drops those
older than ten years. An absent course is not necessarily untransferable.
`

// ErrUsage reports a malformed invocation.
var ErrUsage = errors.New("invalid usage")

type options struct {
	state     string
	outsideUS bool
	json      bool
	csv       bool
}

// Run executes one command. It returns ErrUsage wrapped with detail when the
// arguments do not name a valid invocation.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ErrUsage
	}

	command := args[0]
	if command == "help" || command == "-h" || command == "--help" {
		fmt.Fprint(stdout, usage)
		return nil
	}

	var opts options
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	flags.StringVar(&opts.state, "state", "", "limit results to one state")
	flags.BoolVar(&opts.outsideUS, "outside-us", false, "location is outside the US")
	flags.BoolVar(&opts.json, "json", false, "emit JSON")
	flags.BoolVar(&opts.csv, "csv", false, "emit CSV")

	flagArgs, operands := partitionArgs(args[1:])
	if err := flags.Parse(flagArgs); err != nil {
		return ErrUsage
	}
	if opts.json && opts.csv {
		return fmt.Errorf("%w: --json and --csv are mutually exclusive", ErrUsage)
	}

	client := equiv.New()
	w := writer{out: stdout, opts: opts}

	switch command {
	case "subjects":
		return runOptions(ctx, w, operands, 0, func() ([]equiv.Option, error) {
			return client.PurdueSubjects(ctx)
		})

	case "courses":
		return runOptions(ctx, w, operands, 1, func() ([]equiv.Option, error) {
			return client.PurdueCourses(ctx, subject(operands[0]))
		})

	case "states":
		if len(operands) < 2 {
			return fmt.Errorf("%w: states <SUBJ> <NUM>", ErrUsage)
		}
		return states(ctx, client, w, subject(operands[0]), operands[1])

	case "who":
		if len(operands) < 2 {
			return fmt.Errorf("%w: who <SUBJ> <NUM>", ErrUsage)
		}
		rows, err := client.WhoOffers(ctx, subject(operands[0]), operands[1], opts.state)
		if err != nil {
			return err
		}
		return w.equivalencies(rows)

	case "schools":
		return runOptions(ctx, w, operands, 1, func() ([]equiv.Option, error) {
			return client.Schools(ctx, subject(operands[0]), location(opts))
		})

	case "offerings":
		return runOptions(ctx, w, operands, 1, func() ([]equiv.Option, error) {
			return client.SchoolSubjects(ctx, operands[0])
		})

	case "equiv":
		if len(operands) < 2 {
			return fmt.Errorf("%w: equiv <SCHOOL> <SUBJ> [NUM] --state XX", ErrUsage)
		}
		if opts.state == "" {
			// The report filters by state before school, so the code alone is
			// not enough to locate a school.
			return fmt.Errorf("%w: equiv requires --state", ErrUsage)
		}
		course := "ALL"
		if len(operands) > 2 {
			course = operands[2]
		}
		rows, err := client.Report(ctx, []equiv.ForwardQuery{{
			Location: location(opts),
			State:    subject(opts.state),
			School:   operands[0],
			Subject:  subject(operands[1]),
			Course:   course,
		}}, nil)
		if err != nil {
			return err
		}
		return w.equivalencies(rows)

	default:
		fmt.Fprint(stderr, usage)
		return fmt.Errorf("%w: unknown command %q", ErrUsage, command)
	}
}

func runOptions(ctx context.Context, w writer, operands []string, want int, fetch func() ([]equiv.Option, error)) error {
	if len(operands) < want {
		return fmt.Errorf("%w: missing argument", ErrUsage)
	}
	opts, err := fetch()
	if err != nil {
		return err
	}
	return w.options(opts)
}

// states lists every state holding an articulation, annotated by region.
func states(ctx context.Context, client *equiv.Client, w writer, subj, number string) error {
	locations, err := client.PurdueLocations(ctx, subj, number)
	if err != nil {
		return err
	}
	var all []equiv.Option
	for _, loc := range locations {
		found, err := client.PurdueStates(ctx, subj, number, loc.Value)
		if err != nil {
			return err
		}
		for _, st := range found {
			all = append(all, equiv.Option{
				Text:  fmt.Sprintf("%s (%s)", st.Text, loc.Value),
				Value: st.Value,
			})
		}
	}
	return w.options(all)
}

func location(o options) string {
	if o.outsideUS {
		return "Outside US"
	}
	return "US"
}

// subject normalizes a code the form expects in upper case.
func subject(s string) string { return strings.ToUpper(s) }

// partitionArgs splits flags from operands so flags may appear anywhere on the
// line. The standard flag package stops at the first operand, which makes the
// natural "who MA 42500 --json" fail.
func partitionArgs(args []string) (flags, operands []string) {
	// Flags taking a separate value must carry it along when hoisted.
	valueFlags := map[string]bool{"-state": true, "--state": true}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return flags, append(operands, args[i+1:]...)
		case !strings.HasPrefix(arg, "-") || arg == "-":
			operands = append(operands, arg)
		case valueFlags[arg] && i+1 < len(args):
			flags = append(flags, arg, args[i+1])
			i++
		default:
			flags = append(flags, arg)
		}
	}
	return flags, operands
}
