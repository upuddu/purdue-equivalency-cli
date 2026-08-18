package equiv

import (
	"slices"
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	body := "purdue_stateSelect1\nWisconsin~WI\nIllinois~IL\n\n"

	opts := parseOptions(body)

	want := []Option{{Text: "Wisconsin", Value: "WI"}, {Text: "Illinois", Value: "IL"}}
	if len(opts) != len(want) {
		t.Fatalf("got %d options, want %d", len(opts), len(want))
	}
	for i, o := range opts {
		if o != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, o, want[i])
		}
	}
}

func TestParseOptionsSkipsElementID(t *testing.T) {
	// The first line names the element to populate and is not an option.
	if opts := parseOptions("targetSelect\n"); len(opts) != 0 {
		t.Errorf("got %d options for an empty list, want 0", len(opts))
	}
}

func TestParseOptionsUnescapesEntities(t *testing.T) {
	opts := parseOptions("x\nSt Mary&#x27;s College~001234\n")

	if len(opts) != 1 {
		t.Fatalf("got %d options, want 1", len(opts))
	}
	if want := "St Mary's College"; opts[0].Text != want {
		t.Errorf("Text = %q, want %q", opts[0].Text, want)
	}
}

const reportPage = `<table><tr><td>unrelated layout table</td></tr></table>
<table>
<tr><th>Transfer School</th><th>Transfer Subject</th><th>Transfer Course</th>
<th>Transfer Title</th><th>Transfer Credits</th><th>Purdue Subject</th>
<th>Purdue Course</th><th>Purdue Title</th><th>Purdue Credits</th></tr>
<tr><td>Univ of Illinois at Chicago</td><td>MATH</td><td>417</td>
<td>Complex Analysis</td><td>3</td><td>MA</td><td>42500</td>
<td>Elem Complex Anly</td><td>3</td></tr>
<tr><td>&nbsp;</td><td></td><td></td><td></td><td></td><td>MA</td>
<td>4XTRA</td><td>Extra Undistributed Credit</td><td>1</td></tr>
</table>`

func TestParseReport(t *testing.T) {
	rows := parseReport(reportPage)

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	first := rows[0]
	if first.TransferSchool != "Univ of Illinois at Chicago" {
		t.Errorf("TransferSchool = %q", first.TransferSchool)
	}
	if first.PurdueCourse != "42500" {
		t.Errorf("PurdueCourse = %q, want 42500", first.PurdueCourse)
	}
	if first.Continuation {
		t.Error("first row marked as a continuation")
	}
}

func TestParseReportCarriesContinuationRows(t *testing.T) {
	// A blank transfer side means the course also yields a second Purdue
	// credit; the transfer details must be restated so the row stands alone.
	rows := parseReport(reportPage)

	second := rows[1]
	if !second.Continuation {
		t.Fatal("second row not marked as a continuation")
	}
	if second.TransferCourse != "417" {
		t.Errorf("TransferCourse = %q, want it carried from the row above", second.TransferCourse)
	}
	if second.PurdueCourse != "4XTRA" {
		t.Errorf("PurdueCourse = %q, want 4XTRA", second.PurdueCourse)
	}
}

func TestParseReportIgnoresNonReportTables(t *testing.T) {
	if rows := parseReport(`<table><tr><td>nav</td></tr><tr><td>chrome</td></tr></table>`); len(rows) != 0 {
		t.Errorf("got %d rows from a layout table, want 0", len(rows))
	}
}

func TestParseSubjectOptions(t *testing.T) {
	page := `<select name="purdue_subject_in" id="s1">
<option value="">Please Select...</option>
<option value="MA">MA</option>
<option value="STAT">STAT</option>
</select>`

	opts, err := parseSubjectOptions(page)
	if err != nil {
		t.Fatalf("parseSubjectOptions: %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("got %d subjects, want 2 (the empty placeholder should be dropped)", len(opts))
	}
	if opts[0].Value != "MA" {
		t.Errorf("first subject = %q, want MA", opts[0].Value)
	}
}

func TestParseSubjectOptionsMissingSelect(t *testing.T) {
	if _, err := parseSubjectOptions("<html></html>"); err == nil {
		t.Fatal("expected an error when the subject list is absent")
	}
}

// fieldNames lists the field names of an encoded body in order.
func fieldNames(body string) []string {
	var names []string
	for _, pair := range strings.Split(body, "&") {
		name, _, _ := strings.Cut(pair, "=")
		names = append(names, name)
	}
	return names
}

func TestEncodeReportFormEmitsEveryRow(t *testing.T) {
	// Banner identifies fields positionally, so a short body is rejected.
	body := encodeReportForm(nil, []ReverseQuery{{Subject: "MA", Course: "42500", School: "ALL"}})

	counts := make(map[string]int)
	for _, name := range fieldNames(body) {
		counts[name]++
	}
	for _, name := range []string{
		"location_in", "state_in", "school_in", "subject_in", "course_in",
		"purdue_subject_in", "purdue_course_in", "purdue_location_in",
		"purdue_state_in", "purdue_school_in",
	} {
		if counts[name] != formRows {
			t.Errorf("got %d %s fields, want %d", counts[name], name, formRows)
		}
	}
	if !strings.Contains(body, "purdue_subject_in=MA") {
		t.Error("populated row missing from the body")
	}
}

func TestEncodeReportFormPutsPopulatedRowFirst(t *testing.T) {
	// The populated row must occupy the first slot of its section.
	body := encodeReportForm(nil, []ReverseQuery{{Subject: "MA", Course: "42500"}})

	names := fieldNames(body)
	first := slices.Index(names, "purdue_subject_in")
	if first < 0 {
		t.Fatal("purdue_subject_in absent from body")
	}
	pairs := strings.Split(body, "&")
	if want := "purdue_subject_in=MA"; pairs[first] != want {
		t.Errorf("first reverse row = %q, want %q", pairs[first], want)
	}
}

func TestEncodeReportFormEscapesValues(t *testing.T) {
	body := encodeReportForm(nil, []ReverseQuery{{Location: "Outside US"}})

	if !strings.Contains(body, "purdue_location_in=Outside+US") {
		t.Errorf("value not escaped in body: %s", body)
	}
}
