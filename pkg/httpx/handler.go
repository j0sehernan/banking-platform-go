package httpx

import "net/http"

// HandlerFunc es como http.HandlerFunc pero devuelve error.
// Permite hacer `return err` en handlers en vez de
// `WriteError(w, err); return` repetido en cada uno.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Wrap convierte una HandlerFunc (con error) a un http.HandlerFunc estándar
// para que chi pueda usarlo. Centraliza el mapeo de errores en WriteError.
func Wrap(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}
