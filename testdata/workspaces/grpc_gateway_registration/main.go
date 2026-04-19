package main

import (
	"context"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

type gatewayServer struct {
	mux *runtime.ServeMux
}

func RegisterLocalHandler(ctx context.Context, mux *runtime.ServeMux, endpoint string) error {
	return nil
}

func main() {
	mux := runtime.NewServeMux()
	_ = RegisterUsersHandlerFromEndpoint(context.Background(), mux, "localhost:9090")

	server := &gatewayServer{mux: mux}
	_ = RegisterLocalHandler(context.Background(), server.mux, "localhost:9090")
}
