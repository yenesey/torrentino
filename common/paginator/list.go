package paginator

import "torrentino/common/list"

// Aliases keep the Telegram paginator source compatible while the actual
// implementation lives in common/list, shared with the web interface.
type (
	List      = list.List
	Sorting   = list.Sorting
	Evaluator = list.Evaluator
)
