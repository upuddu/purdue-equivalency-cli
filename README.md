# purdue-equivalency-cli

A command-line client for Purdue University's [Transfer Credit Course
Equivalency Guide](https://selfservice.mypurdue.purdue.edu/prod/bzwtxcrd.p_select_info).

The web version is a chain of dependent dropdowns: five selections and a form
submission per question. This exposes the same data directly, including the
reverse lookup — given a Purdue course, which institutions have a course that
articulates to it.

## Install

```
go install github.com/upuddu/purdue-equivalency-cli@latest
```

## Usage

Start from a Purdue course you need:

```
$ purdue-equivalency-cli who MA 42500
STATE  SCHOOL                          COURSE                              CR  PURDUE
CA     Univ of California/Berkeley     MATH XB205 Theory Of Functions Cmp  4   MA 42500 Elem Complex Anly
FL     Florida Institute of Technlgy   MTH 3101 Complex Variables          3   MA 42500 Elem Complex Anly
IL     Univ of Illinois at Chicago     MATH 417 Complex Analysis           3   MA 42500 Elem Complex Anly
IN     Indiana University Bloomington  MATH M415 Elem Complex Variables W  3   MA 42500 Elem Complex Anly
```

Or from a school you might attend:

```
$ purdue-equivalency-cli schools WI
...
001846  Univ of Wisconsin Madison
$ purdue-equivalency-cli equiv 001846 MATH 341 --state WI
```

### Commands

| Command | Result |
| --- | --- |
| `who <SUBJ> <NUM>` | schools with an articulation for a Purdue course |
| `states <SUBJ> <NUM>` | states that have one |
| `courses <SUBJ>` | Purdue courses in a subject with any articulation |
| `subjects` | Purdue subject codes |
| `schools <STATE>` | schools in a state, with their school codes |
| `offerings <SCHOOL>` | subjects a school has articulations for |
| `equiv <SCHOOL> <SUBJ> [NUM] --state XX` | what a school's course becomes at Purdue |

Flags: `--state XX`, `--outside-us`, `--json`, `--csv`.

`who` runs one query per state that holds an articulation. `states` answers in
three requests, so it is the cheaper way to check a long list of courses before
sweeping the ones that turn out to have results.

## Interpreting results

- The guide lists only articulations Purdue has already evaluated, and
  articulations older than ten years have been removed. A course that is absent
  is not necessarily untransferable.
- Course codes ending in `XXXX` or `XTRA` are undistributed credit rather than a
  specific course equivalent. `MA 3XTRA` counts as 300-level mathematics credit
  but satisfies no particular requirement.
- One transfer course can produce several Purdue credits. Those extra rows are
  marked `continuation` in JSON output.
- Equivalencies change. Anything load-bearing is worth confirming with the
  registrar.

## How it works

Two endpoints, neither authenticated:

- `bzwtxcrd.p_ajax` populates the dependent dropdowns, answering with the id of
  the element to fill followed by `text~value` lines.
- `bzwtxcrd.p_display_report` runs the search. The form renders five query rows
  per direction and the backend reads them positionally, so every row has to
  appear in the request body even when blank — a body carrying only the
  populated row returns a 500.

Dropdown responses are cached for the lifetime of the process.

## License

MIT
