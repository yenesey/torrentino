package paginator

import (
	"fmt"
	"testing"

	"torrentino/common/list"
)

type testItem struct {
	name  string
	group string
}

// fakeScreen implements Builder, Actor and Evaluator the way the real handlers
// do, but without a Telegram bot behind it.
type fakeScreen struct {
	p *Paginator

	executed  []string
	lastIndex int
}

func (f *fakeScreen) Header() string { return "" }
func (f *fakeScreen) Footer() string { return "" }

func (f *fakeScreen) Line(i int) string { return f.p.Item(i).(*testItem).name }

func (f *fakeScreen) Actions(i int) []string { return []string{"delete"} }

func (f *fakeScreen) Execute(i int, action string) bool {
	f.executed = append(f.executed, action)
	f.lastIndex = i
	return true
}

func (f *fakeScreen) Stringify(i int, attribute string) string {
	if attribute == "Group" {
		return f.p.Item(i).(*testItem).group
	}
	return ""
}

func (f *fakeScreen) Compare(i, j int, attribute string) bool { return false }

func makeItems(n int) []testItem {
	items := make([]testItem, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, testItem{name: fmt.Sprintf("item-%d", i), group: "g"})
	}
	return items
}

// newTestScreen wires a paginator to the fake screen with the same sorting and
// filtering setup the real handlers use.
func newTestScreen(t *testing.T, items ...testItem) (*Paginator, *fakeScreen) {
	t.Helper()

	p := &Paginator{prefix: "test"}
	p.View = list.NewView(4)
	f := &fakeScreen{p: p}
	p.Builder = f
	p.Actor = f
	p.View.Actor = f
	p.List.Evaluator = f

	p.SetupSorting([]Sorting{{Attribute: "Group", Alias: "group", Order: 0}})
	p.SetupFiltering([]string{"Group"})

	p.Alloc(len(items))
	for i := range items {
		p.Append(&items[i])
	}
	return p, f
}

// Regression test for the reported crash: cmd[0] was indexed without a length
// check, and the filter command indexed split[1] without checking that a
// separator was present. Both used to panic on data that Telegram can deliver.
func TestDispatchSurvivesMalformedCommands(t *testing.T) {
	commands := []string{
		"",                     // empty callback data
		"#order_by#",           // payload command without a payload
		"#action__#",           //
		"#filterby#x",          // filter command without a "/"
		"#filterby#",           //
		"not-a-command",        //
		"12abc",                // starts with a digit, but is not an index
		"-5",                   // Atoi would happily accept these
		"+5",                   //
		"99999999999999999999", // overflows int
		"%", "стоп",            // non-ASCII input
	}

	p, f := newTestScreen(t, makeItems(8)...)

	for _, cmd := range commands {
		p.dispatch(cmd) // must not panic
	}

	if p.Selected != -1 {
		t.Errorf("selectedItem = %d, want -1: malformed commands must not select an item", p.Selected)
	}
	if p.Page != 0 {
		t.Errorf("activePage = %d, want 0", p.Page)
	}
	if len(f.executed) != 0 {
		t.Errorf("Execute was called for malformed commands: %v", f.executed)
	}
}

func TestDispatchSelectsItem(t *testing.T) {
	p, _ := newTestScreen(t, makeItems(8)...)

	p.dispatch("3")
	if p.Selected != 3 {
		t.Fatalf("selectedItem = %d, want 3", p.Selected)
	}

	// Values that are not an in-range index must leave the selection alone.
	for _, cmd := range []string{"8", "100", "12abc", "-1", "+1", ""} {
		p.Selected = 3
		p.dispatch(cmd)
		if p.Selected != 3 {
			t.Errorf("command %q changed the selection to %d", cmd, p.Selected)
		}
	}
}

func TestDispatchActionRequiresValidSelection(t *testing.T) {
	p, f := newTestScreen(t, makeItems(3)...)

	// A selection left over from a longer list (or a forged callback) must not
	// reach the handler, which indexes into the item list.
	p.Selected = 999
	p.dispatch(CB_ACTION + "delete")
	if len(f.executed) != 0 {
		t.Fatalf("Execute was called with a stale selection: %v", f.executed)
	}

	p.Selected = 1
	p.dispatch(CB_ACTION + "delete")
	if len(f.executed) != 1 || f.lastIndex != 1 {
		t.Fatalf("executed = %v at index %d, want [delete] at 1", f.executed, f.lastIndex)
	}
	if p.Selected != -1 {
		t.Errorf("selectedItem = %d, want it cleared after the action", p.Selected)
	}
}

// The "next" button and its handler used to compute the last page differently,
// so a press could move past the last non-empty page and show an empty list.
func TestDispatchNextPageStopsAtLastPage(t *testing.T) {
	p, _ := newTestScreen(t, makeItems(8)...) // pages 0 and 1 only

	p.dispatch(CB_NEXT_PAGE)
	if p.Page != 1 {
		t.Fatalf("activePage = %d, want 1", p.Page)
	}

	p.dispatch(CB_NEXT_PAGE)
	if p.Page != 1 {
		t.Errorf("activePage = %d, want it to stay on the last page", p.Page)
	}
	if p.Page < p.lastPage() {
		t.Errorf("the next button would still be offered on page %d", p.Page)
	}

	p.dispatch(CB_PREV_PAGE)
	if p.Page != 0 {
		t.Errorf("activePage = %d, want 0", p.Page)
	}
}

func TestLastPage(t *testing.T) {
	for _, c := range []struct{ items, want int }{
		{0, 0}, {1, 0}, {4, 0}, {5, 1}, {8, 1}, {9, 2}, {12, 2}, {13, 3},
	} {
		p, _ := newTestScreen(t, makeItems(c.items)...)
		if got := p.lastPage(); got != c.want {
			t.Errorf("%d items: lastPage() = %d, want %d", c.items, got, c.want)
		}
	}
}

// Filter values are encoded as <attribute>/<value>, and the value itself may
// contain a slash: splitting on the first separator keeps it intact.
func TestDispatchFilterValueWithSlash(t *testing.T) {
	p, _ := newTestScreen(t, testItem{name: "a", group: "sub/g"})

	p.dispatch(CB_FILTER_BY + "Group/sub/g")

	found := false
	for _, value := range p.FilterValues("Group") {
		if value.Value == "sub/g" {
			found = true
			if !value.Enabled {
				t.Errorf("filter %q is not enabled, want it toggled as one value", "sub/g")
			}
		}
	}
	if !found {
		t.Errorf("filter value %q is missing", "sub/g")
	}
}

// /torrserver never calls SetupFiltering or SetupSorting, so those ordered maps
// stay nil; a callback for them must be ignored instead of dereferencing nil.
func TestDispatchWithoutFilterAndSortingSetup(t *testing.T) {
	p := &Paginator{prefix: "torrserver"}
	p.View = list.NewView(4)
	f := &fakeScreen{p: p}
	p.Builder = f
	p.Actor = f
	p.View.Actor = f
	p.List.Evaluator = f

	p.dispatch(CB_FILTER_BY + "Status/seeding")
	p.dispatch(CB_ORDER_BY + "Size")
	p.dispatch(CB_TOGGLE_FILTERS)
}
