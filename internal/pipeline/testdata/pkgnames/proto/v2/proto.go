// Package proto lives in a directory that is called v2.
package proto

// Reply is a response.
type Reply struct {
	// Text is the reply.
	Text string `json:"text"`
}
