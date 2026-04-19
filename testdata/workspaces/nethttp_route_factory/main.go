package main

import "net/http"

func emitMetric() {}

func sessionStore() {}

func makeUsersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		emitMetric()
		sessionStore()
		w.WriteHeader(http.StatusAccepted)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.Handle("/users/factory", makeUsersHandler())
}
