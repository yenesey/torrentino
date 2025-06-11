package paginator

import "torrentino/common/ordmap"

type Filtering struct {
	attributes *ordmap.OrderedMap[string, *ordmap.OrderedMap[string, bool]]
}

func (f *Filtering) Setup(attributes []string) {
	f.attributes = ordmap.New[string, *ordmap.OrderedMap[string, bool]]()
	for _, attr := range attributes {
		f.attributes.Set(attr, ordmap.New[string, bool]())
	}
}

func (f *Filtering) ToggleAttribute(attribute string, value string) {
	if buttons, ok := f.attributes.Get(attribute); ok {
		if enabled, ok := buttons.Get(value); ok {
			buttons.Set(value, !enabled)
		}
	}
}