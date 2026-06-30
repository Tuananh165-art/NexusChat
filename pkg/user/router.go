package user

import (
	"context"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
)

type Router struct {
	httpServer common.HttpServer
	grpcServer common.GrpcServer
}

func NewRouter(httpServer common.HttpServer, grpcServer common.GrpcServer) *Router {
	return &Router{httpServer, grpcServer}
}

func (r *Router) Run() {
	r.httpServer.RegisterRoutes()
	r.httpServer.Run()

	r.grpcServer.Register()
	r.grpcServer.Run()
}
func (r *Router) GracefulStop(ctx context.Context) error {
	if err := r.grpcServer.GracefulStop(); err != nil {
		return err
	}
	return r.httpServer.GracefulStop(ctx)
}
