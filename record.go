package ligno

import "time"

// Ctx is additional context for log record.
type Ctx map[string]interface{}

// merge creates new context, merges this one with provided one and returns it.
func (ctx Ctx) merge(other Ctx) (merged Ctx) {
	newCtx := make(Ctx)
	for k, v := range ctx {
		newCtx[k] = v
	}
	for k, v := range other {
		newCtx[k] = v
	}
	return newCtx
}

// Record holds information about one log message.
type Record struct {
	Time    		time.Time 		`json:"ts"`
	Level   		Level     		`json:"lvl"`
	Message 		string    		`json:"msg"`
	ContextList 	[]interface{} 	`json:"-"`
	Pairs   		[]interface{}	`json:"-"`
	Logger  		*Logger   		`json:"-"`
	File    		string    		`json:"file"`
	Line    		int       		`json:"line"`
}
