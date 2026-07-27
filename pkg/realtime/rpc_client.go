package realtime

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"
)

func StructBool(value *structpb.Struct, field string) bool {
	if value == nil {
		return false
	}
	entry := value.GetFields()[field]
	return entry != nil && entry.GetBoolValue()
}

func CallStructRPC(ctx context.Context, endpoint, callerService, service, method string, fields map[string]any) (*structpb.Struct, error) {
	if endpoint == "" {
		return nil, nil
	}
	secret := os.Getenv("NEXUSCHAT_GRPC_SHARED_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("internal grpc authentication is not configured")
	}
	request, err := structpb.NewStruct(fields)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	callCtx = metadata.AppendToOutgoingContext(callCtx,
		"authorization", "Bearer "+secret,
		"service-id", callerService,
	)
	callCtx, err = StructAssertionMetadata(callCtx, service, method, request)
	if err != nil {
		cancel()
		return nil, err
	}
	defer cancel()
	conn, err := grpc.DialContext(callCtx, endpoint, grpc.WithTransportCredentials(MustClientTransportCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	response := &structpb.Struct{}
	if err := conn.Invoke(callCtx, "/"+service+"/"+method, request, response); err != nil {
		return nil, err
	}
	return response, nil
}
