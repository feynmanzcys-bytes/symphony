package bridge

import "errors"

// Error is an error the office page can show in the reader's own language: the page looks up
// "error.<Code>" in its catalog and fills in Params. Error() is the English text, for logs and
// for anyone calling the API directly.
type Error struct {
	Code   string
	Params map[string]string
	Text   string
}

func (e *Error) Error() string { return e.Text }

// newError builds an *Error; params are name/value pairs for the message's placeholders.
func newError(code, text string, params ...string) *Error {
	e := &Error{Code: code, Text: text}
	for i := 0; i+1 < len(params); i += 2 {
		if e.Params == nil {
			e.Params = map[string]string{}
		}
		e.Params[params[i]] = params[i+1]
	}
	return e
}

// errorBody is how the API answers a failure: {"error": English text, "code": ..., "params": {...}}.
func errorBody(err error) map[string]any {
	body := map[string]any{"error": err.Error()}
	var e *Error
	if errors.As(err, &e) {
		body["code"] = e.Code
		if len(e.Params) > 0 {
			body["params"] = e.Params
		}
	}
	return body
}
