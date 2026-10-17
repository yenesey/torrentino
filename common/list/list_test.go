package list

import (
	"testing"
)

type testItem struct {
	name  string
	group string
	size  int
}

type testEvaluator struct {
	items []*testItem
}

func (e testEvaluator) Compare(i, j int, attribute string) bool {
	if attribute == "Size" {
		return e.items[i].size < e.items[j].size
	}
	return false
}

func (e testEvaluator) Stringify(i int, attribute string) string {
	if attribute == "Group" {
		return e.items[i].group
	}
	return ""
}

type testActor struct {
	calls []int
	ok    bool
}

func (a *testActor) Execute(i int, action string) bool {
	a.calls = append(a.calls, i)
	return a.ok
}

func makeView(t *testing.T, n int) (*View, []*testItem, *testActor) {
	t.Helper()
	items := make([]*testItem, n)
	for i := range items {
		items[i] = &testItem{name: "item", group: "g", size: i}
	}
	v := NewView(4)
	v.List.Evaluator = testEvaluator{items: items}
	v.SetupSorting([]Sorting{
		{Attribute: "Size", Alias: "size", Order: 1},
		{Attribute: "Group", Alias: "group", Order: 0},
	})
	v.SetupFiltering([]string{"Group"})
	for _, item := range items {
		v.Append(item)
	}
	actor := &testActor{ok: true}
	v.Actor = actor
	return &v, items, actor
}

func TestViewBoundsAndLastPage(t *testing.T) {
	for _, c := range []struct{ items, last int }{
		{0, 0}, {1, 0}, {4, 0}, {5, 1}, {8, 1}, {9, 2}, {12, 2},
	} {
		v, _, _ := makeView(t, c.items)
		if got := v.LastPage(); got != c.last {
			t.Errorf("%d items: LastPage = %d, want %d", c.items, got, c.last)
		}
		v.Page = 1
		from, to := v.Bounds()
		if from < 0 || from > to || to > v.Len() {
			t.Errorf("%d items: Bounds = %d,%d for len %d", c.items, from, to, v.Len())
		}
	}
}

func TestSelectValidatesBounds(t *testing.T) {
	v, _, _ := makeView(t, 8)
	if !v.Select(3) || v.Selected != 3 {
		t.Fatalf("Select(3) = false or Selected = %d", v.Selected)
	}
	for _, index := range []int{-1, 8, 100} {
		v.Selected = 3
		if v.Select(index) {
			t.Errorf("Select(%d) succeeded", index)
		}
		if v.Selected != 3 {
			t.Errorf("Select(%d) changed selection to %d", index, v.Selected)
		}
	}
	v.Controls = true
	v.Select(1)
	if v.Controls {
		t.Error("Select must close the controls panel")
	}
}

func TestPagingStopsAtBounds(t *testing.T) {
	v, _, _ := makeView(t, 8) // pages 0 and 1
	if !v.NextPage() || v.Page != 1 {
		t.Fatalf("NextPage = %v, Page = %d", v.Page, v.Page)
	}
	if v.NextPage() {
		t.Error("NextPage moved past the last page")
	}
	if v.Page != 1 {
		t.Errorf("Page = %d, want 1", v.Page)
	}
	if !v.PrevPage() || v.Page != 0 {
		t.Errorf("PrevPage failed, Page = %d", v.Page)
	}
	if v.PrevPage() {
		t.Error("PrevPage moved before the first page")
	}
}

func TestSortingCycleAndQueue(t *testing.T) {
	v, _, _ := makeView(t, 4)
	// Size starts at order 1 (descending) and is already in the queue.
	if got := v.SortingQueuePosition("Size"); got != 1 {
		t.Fatalf("initial queue position = %d, want 1", got)
	}
	v.Selected = 2
	v.ToggleSorting("Size")
	if got := sortingByAttribute(v.Sortings(), "Size").Order; got != 2 {
		t.Errorf("order after one toggle = %d, want 2", got)
	}
	if v.Selected != -1 {
		t.Error("sorting must clear the selection")
	}
	v.ToggleSorting("Size")
	if got := sortingByAttribute(v.Sortings(), "Size").Order; got != 0 {
		t.Errorf("order after two toggles = %d, want 0", got)
	}
	if got := v.SortingQueuePosition("Size"); got != 0 {
		t.Errorf("queue position after removing = %d, want 0", got)
	}
	v.ToggleSorting("Size")
	if got := sortingByAttribute(v.Sortings(), "Size").Order; got != 1 {
		t.Errorf("order after three toggles = %d, want 1", got)
	}
	if got := v.SortingQueuePosition("Size"); got != 1 {
		t.Errorf("queue position after re-adding = %d, want 1", got)
	}
}

func sortingByAttribute(sortings []Sorting, attribute string) Sorting {
	for _, sorting := range sortings {
		if sorting.Attribute == attribute {
			return sorting
		}
	}
	return Sorting{}
}

func TestFilteringResetsPageAndSelection(t *testing.T) {
	items := []*testItem{
		{name: "a", group: "x", size: 1},
		{name: "b", group: "y", size: 2},
		{name: "c", group: "x", size: 3},
	}
	v := NewView(1)
	v.List.Evaluator = testEvaluator{items: items}
	v.SetupFiltering([]string{"Group"})
	for _, item := range items {
		v.Append(item)
	}
	v.Filter()
	v.Page = 2
	v.Selected = 1
	if !v.ToggleFilter("Group", "x") {
		t.Fatal("ToggleFilter for an existing value returned false")
	}
	if v.Page != 0 || v.Selected != -1 {
		t.Errorf("filter did not reset page/selection: page=%d selected=%d", v.Page, v.Selected)
	}
	v.Filter()
	if v.Len() != 2 {
		t.Errorf("filter x: Len = %d, want 2", v.Len())
	}
}

func TestFilterMatchesValueAndAllOffShowsEverything(t *testing.T) {
	items := []*testItem{
		{name: "a", group: "x", size: 1},
		{name: "b", group: "y", size: 2},
	}
	v := NewView(10)
	v.List.Evaluator = testEvaluator{items: items}
	v.SetupFiltering([]string{"Group"})
	for _, item := range items {
		v.Append(item)
	}
	v.Filter()
	if v.Len() != 2 {
		t.Fatalf("all filters off: Len = %d, want 2", v.Len())
	}
	v.ToggleFilter("Group", "x")
	v.Filter()
	if v.Len() != 1 || v.Item(0).(*testItem).group != "x" {
		t.Fatalf("filter x: Len = %d", v.Len())
	}
	v.ToggleFilter("Group", "x")
	v.ToggleFilter("Group", "y")
	v.Filter()
	if v.Len() != 1 || v.Item(0).(*testItem).group != "y" {
		t.Fatalf("filter y: Len = %d", v.Len())
	}
}

func TestToggleFilterUnknownIsNoop(t *testing.T) {
	v, _, _ := makeView(t, 2)
	if v.ToggleFilter("Nope", "x") {
		t.Error("ToggleFilter for an unknown attribute returned true")
	}
	if v.ToggleFilter("Group", "nope") {
		t.Error("ToggleFilter for an unknown value returned true")
	}
}

func TestExecuteSelectedChecksBounds(t *testing.T) {
	v, _, actor := makeView(t, 3)
	v.Selected = 99
	if v.ExecuteSelected("delete") {
		t.Error("ExecuteSelected ran for a stale selection")
	}
	if len(actor.calls) != 0 {
		t.Fatalf("actor was called: %v", actor.calls)
	}
	v.Selected = 1
	if !v.ExecuteSelected("delete") {
		t.Error("ExecuteSelected returned false for a valid selection")
	}
	if len(actor.calls) != 1 || actor.calls[0] != 1 {
		t.Errorf("actor calls = %v, want [1]", actor.calls)
	}
	if v.Selected != -1 {
		t.Errorf("Selected = %d after a successful action, want -1", v.Selected)
	}
}

func TestDeleteOutOfRangeIsNoop(t *testing.T) {
	v, _, _ := makeView(t, 2)
	v.Delete(-1)
	v.Delete(99)
	if v.Len() != 2 {
		t.Errorf("Len = %d after out-of-range deletes, want 2", v.Len())
	}
	v.Delete(0)
	if v.Len() != 1 {
		t.Errorf("Len = %d after delete, want 1", v.Len())
	}
}
