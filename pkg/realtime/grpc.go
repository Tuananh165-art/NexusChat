package realtime

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/structpb"
)

type StructRPC interface {
	Invoke(context.Context, string, *structpb.Struct) (*structpb.Struct, error)
}

type StructRPCServer struct {
	Methods map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error)
}

func (s *StructRPCServer) Invoke(ctx context.Context, method string, request *structpb.Struct) (*structpb.Struct, error) {
	handler, ok := s.Methods[method]
	if !ok {
		return nil, fmt.Errorf("unknown RPC method %s", method)
	}
	return handler(ctx, request)
}

func NewGRPCServer(serviceName string, methods map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error)) *grpc.Server {
	server := grpc.NewServer()
	RegisterStructRPC(server, serviceName, methods)
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus(serviceName, healthpb.HealthCheckResponse_SERVING)
	return server
}

func RegisterStructRPC(server *grpc.Server, serviceName string, methods map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error)) {
	implementation := &StructRPCServer{Methods: methods}
	descriptors := make([]grpc.MethodDesc, 0, len(methods))
	for methodName := range methods {
		name := methodName
		descriptors = append(descriptors, grpc.MethodDesc{
			MethodName: name,
			Handler: func(
				srv any,
				ctx context.Context,
				dec func(any) error,
				interceptor grpc.UnaryServerInterceptor,
			) (any, error) {
				request := &structpb.Struct{}
				if err := dec(request); err != nil {
					return nil, err
				}
				invoke := func(callCtx context.Context, req any) (any, error) {
					return srv.(StructRPC).Invoke(callCtx, name, req.(*structpb.Struct))
				}
				if interceptor == nil {
					return invoke(ctx, request)
				}
				info := &grpc.UnaryServerInfo{
					Server:     srv,
					FullMethod: "/" + serviceName + "/" + name,
				}
				return interceptor(ctx, request, info, invoke)
			},
		})
	}
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: serviceName,
		HandlerType: (*StructRPC)(nil),
		Methods:     descriptors,
		Metadata:    serviceName + ".proto",
	}, implementation)
}

func ServeGRPC(server *grpc.Server, port string) error {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	return server.Serve(listener)
}
