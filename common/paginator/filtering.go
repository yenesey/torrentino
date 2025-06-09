package paginator

type FilterAttribute struct {
	AttributeName string
	Values        []string
	State         map[string]bool
}

type FilteringState struct {
	attributes []FilterAttribute
}

func (f *FilteringState) Setup(attributes []string) {
	f.attributes = make([]FilterAttribute, len(attributes))
	for i, AttributeName := range attributes {
		f.attributes[i].AttributeName = AttributeName
		f.attributes[i].Values = make([]string, 0, 8)
		f.attributes[i].State = make(map[string]bool)
	}
}

func (f *FilteringState) Toggle(attributeName string, attributeValue string) {
	for i := range f.attributes {
		if f.attributes[i].AttributeName == attributeName {
			f.attributes[i].State[attributeValue] = !f.attributes[i].State[attributeValue]
			break
		}
	}
}