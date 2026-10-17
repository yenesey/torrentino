package list

// Actor performs a domain action on the item at a view position. It reports
// whether the action succeeded; the caller decides whether to clear the
// selection. Implementations must validate the index themselves or rely on
// View.ExecuteSelected, which checks the bounds before calling.
type Actor interface {
	Execute(i int, action string) bool
}

// View is one screen's state: the list plus the current page, selection and
// controls panel. It is deliberately free of Telegram and HTTP types, so the
// exact same operations can be driven by callback data or by web commands.
type View struct {
	List
	Actor

	PerPage  int  // items per page; <= 0 means one page
	Page     int  // 0-based active page
	Selected int  // -1 when nothing is selected
	Controls bool // sorting/filtering panel is open
}

// NewView returns a view with no selection and the controls panel closed.
func NewView(perPage int) View {
	return View{PerPage: perPage, Selected: -1}
}

// Bounds returns the half-open range of view positions visible on the active
// page. Both values are clamped to the current list length.
func (v *View) Bounds() (from, to int) {
	if v.PerPage <= 0 {
		return 0, v.Len()
	}
	from = v.Page * v.PerPage
	if from < 0 {
		from = 0
	}
	if from > v.Len() {
		from = v.Len()
	}
	to = from + v.PerPage
	if to > v.Len() {
		to = v.Len()
	}
	return from, to
}

// LastPage is the index of the final non-empty page. An empty list has page 0.
func (v *View) LastPage() int {
	if v.PerPage <= 0 || v.Len() == 0 {
		return 0
	}
	return (v.Len() - 1) / v.PerPage
}

// ClampPage moves the active page back inside the list after a reload that
// made it shorter.
func (v *View) ClampPage() {
	if v.Page < 0 {
		v.Page = 0
	}
	if last := v.LastPage(); v.Page > last {
		v.Page = last
	}
}

// Prepare applies filters and sorting. Call it after changing items and
// before rendering.
func (v *View) Prepare() {
	v.Filter()
	v.Sort()
	v.ClampPage()
}

// Select selects an item. Only 0 <= i < Len is accepted; a stale or forged
// index leaves the current selection untouched. Selecting closes the controls
// panel, matching the Telegram behaviour.
func (v *View) Select(i int) bool {
	if i < 0 || i >= v.Len() {
		return false
	}
	v.Selected = i
	v.Controls = false
	return true
}

// Deselect clears the selection.
func (v *View) Deselect() {
	v.Selected = -1
}

// NextPage moves to the next page when it exists and reports whether it did.
func (v *View) NextPage() bool {
	if v.Page < v.LastPage() {
		v.Page++
		return true
	}
	return false
}

// PrevPage moves to the previous page when it exists and reports whether it
// did.
func (v *View) PrevPage() bool {
	if v.Page > 0 {
		v.Page--
		return true
	}
	return false
}

// ToggleSorting cycles the attribute and clears the selection.
func (v *View) ToggleSorting(attribute string) {
	if !v.HasSorting(attribute) {
		return
	}
	v.List.ToggleSorting(attribute)
	v.Selected = -1
}

// ToggleFilter switches the filter value, resets the page and clears the
// selection, exactly like the Telegram dispatcher.
func (v *View) ToggleFilter(attribute string, value string) bool {
	if !v.SetFilter(attribute, value, !v.FilterEnabled(attribute, value)) {
		return false
	}
	v.Page = 0
	v.Selected = -1
	return true
}

// ToggleControls opens or closes the sorting/filtering panel.
func (v *View) ToggleControls() {
	v.Controls = !v.Controls
}

// ExecuteSelected runs the action for the selected item. It never calls the
// actor for a missing or stale selection.
func (v *View) ExecuteSelected(action string) bool {
	if v.Actor == nil || v.Selected < 0 || v.Selected >= v.Len() {
		return false
	}
	if v.Actor.Execute(v.Selected, action) {
		v.Selected = -1
		return true
	}
	return false
}
