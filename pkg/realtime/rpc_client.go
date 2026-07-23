package realtime

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

func StructBool(value *structpb.Struct, field string) bool {
	if value == nil {
		return false
	}
	entry := value.GetFields()[field]
	return entry != nil && entry.GetBoolValue()
}

func CallStructRPC(ctx context.Context, endpoint, service, method string, fields map[string]any) (*structpb.Struct, error) {
	if endpoint == "" {
		return nil, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(callCtx, endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	request, err := structpb.NewStruct(fields)
	if err != nil {
		return nil, err
	}
	response := &structpb.Struct{}
	if err := conn.Invoke(callCtx, "/"+service+"/"+method, request, response); err != nil {
		return nil, err
	}
	return response, nil
}
