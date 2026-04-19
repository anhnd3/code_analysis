package main

import "net/http"

func traceRequest() {}

func userService() {}

func listUsers(w http.ResponseWriter, r *http.Request) {
	traceRequest()
	userService()
	w.WriteHeader(http.StatusOK)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/users", listUsers)
}
