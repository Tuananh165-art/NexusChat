package user

import (
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/config"
	"github.com/Tuananh165-art/NexusChat/pkg/transport"
	userpb "github.com/Tuananh165-art/NexusChat/proto/user"
)

type GrpcServer struct {
	grpcPort string
	logger   common.GrpcLog
	s        *grpc.Server
	userSvc  UserService
}

func NewGrpcServer(name string, logger common.GrpcLog, config *config.Config, userSvc UserService) *GrpcServer {
	srv := &GrpcServer{
		grpcPort: config.User.Grpc.Server.Port,
		logger:   logger,
		userSvc:  userSvc,
	}
	srv.s = transport.InitializeGrpcServer(name, srv.logger)
	return srv
}

func (srv *GrpcServer) Register() {
	userpb.RegisterUserServiceServer(srv.s, srv)
}

func (srv *GrpcServer) Run() {
	go func() {
		addr := "0.0.0.0:" + srv.grpcPort
		srv.logger.Info("grpc server listening", slog.String("addr", addr))
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			srv.logger.Error(err.Error())
			os.Exit(1)
		}
		if err := srv.s.Serve(lis); err != nil {
			srv.logger.Error(err.Error())
			os.Exit(1)
		}
	}()
}

func (srv *GrpcServer) GracefulStop() error {
	srv.s.GracefulStop()
	return nil
}
