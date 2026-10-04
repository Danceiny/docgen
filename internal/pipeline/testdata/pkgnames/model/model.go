// Package models lives in a directory that is called model.
package models

// Req is a request.
type Req struct {
	// Q is the question.
	Q string `json:"q"`
}

// Resp is a response.
type Resp struct {
	// A is the answer.
	A string `json:"a"`
}
