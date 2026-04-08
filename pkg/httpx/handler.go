package httpx

import "net/http"

// HandlerFunc is like http.HandlerFunc but returns an error.
// Lets handlers do `return err` instead of repeating
// `WriteError(w, err); return` in every one.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Wrap converts a HandlerFunc (with error) into a standard http.HandlerFunc
// so chi can use it. Centralizes error mapping in WriteError.
func Wrap(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}
