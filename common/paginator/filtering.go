package paginator

type FilterHeader struct {
	Attribute string
	Values    []string
	States     map[string]bool  // on/off == true/false
}

type Filtering struct {
	attributes []FilterHeader
}

func (f *Filtering) Setup(attributes []string) {
	f.attributes = make([]FilterHeader, len(attributes))
	for i, Attribute := range attributes {
		f.attributes[i].Attribute = Attribute
		f.attributes[i].Values = make([]string, 0, 8)
		f.attributes[i].States = make(map[string]bool)
	}
}

func (f *Filtering) ToggleAttribute(attribute string, value string) {
	for i := range f.attributes {
		if f.attributes[i].Attribute == attribute {
			f.attributes[i].States[value] = !f.attributes[i].States[value]
			break
		}
	}
}