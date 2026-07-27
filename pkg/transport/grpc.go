package transport

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/realtime"
	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/sd"
	"github.com/go-kit/kit/sd/lb"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	ServiceIdHeader string = "Service-Id"
)

func internalGRPCAuthOK(ctx context.Context, fullMethod string) bool {
	if strings.HasPrefix(fullMethod, "/grpc.health.v1.Health/") {
		return true
	}
	secret := os.Getenv("NEXUSCHAT_GRPC_SHARED_SECRET")
	if secret == "" {
		return false
	}
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	serviceIDs := metadata.ValueFromIncomingContext(ctx, "service-id")
	return len(values) > 0 && len(serviceIDs) > 0 && strings.TrimSpace(serviceIDs[0]) != "" && subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+secret)) == 1
}

func serviceAuthUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if !internalGRPCAuthOK(ctx, info.FullMethod) {
		return nil, status.Error(codes.Unauthenticated, "invalid internal grpc credentials")
	}
	if request, ok := req.(proto.Message); ok {
		if err := realtime.VerifyProtoAssertion(ctx, request, info.FullMethod); err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
	}
	return handler(ctx, req)
}

func serviceAuthStreamInterceptor(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if !internalGRPCAuthOK(stream.Context(), info.FullMethod) {
		return status.Error(codes.Unauthenticated, "invalid internal grpc credentials")
	}
	return handler(srv, stream)
}

func interceptorLogger(l common.GrpcLog) logging.Logger {
	return logging.LoggerFunc(func(_ context.Context, lvl logging.Level, msg string, fields ...any) {
		switch lvl {
		case logging.LevelDebug:
			l.Debug(msg, fields...)
		case logging.LevelInfo:
			l.Info(msg, fields...)
		case logging.LevelWarn:
			l.Warn(msg, fields...)
		case logging.LevelError:
			l.Error(msg, fields...)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

func InitializeGrpcServer(name string, logger common.GrpcLog) *grpc.Server {
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(1024 * 1024 * 8), // increase to 8 MB (default: 4 MB)
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second, // terminate the connection if a client pings more than once every 5 seconds
			PermitWithoutStream: true,            // allow pings even when there are no active streams
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Second,  // if a client is idle for 15 seconds, send a GOAWAY
			MaxConnectionAge:      600 * time.Second, // if any connection is alive for more than maxConnectionAge, send a GOAWAY
			MaxConnectionAgeGrace: 5 * time.Second,   // allow 5 seconds for pending RPCs to complete before forcibly closing connections
			Time:                  5 * time.Second,   // ping the client if it is idle for 5 seconds to ensure the connection is still active
			Timeout:               1 * time.Second,   // wait 1 second for the ping ack before assuming the connection is dead
		}),
	}

	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerCounterOptions(
			func(o *prometheus.CounterOpts) {
				o.Namespace = name
			},
			grpcprom.WithConstLabels(prometheus.Labels{"serviceID": name}),
		),
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramConstLabels(prometheus.Labels{"serviceID": name}),
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	prometheus.MustRegister(srvMetrics)
	exemplarFromContext := func(ctx context.Context) prometheus.Labels {
		if span := trace.SpanContextFromContext(ctx); span.IsSampled() {
			return prometheus.Labels{"traceID": span.TraceID().String()}
		}
		return nil
	}
	// Setup metric for panic recoveries
	panicsTotal := promauto.NewCounter(prometheus.CounterOpts{
		Namespace:   name,
		Name:        "grpc_req_panics_recovered_total",
		Help:        "Total number of gRPC requests recovered from internal panic.",
		ConstLabels: prometheus.Labels{"serviceID": name},
	})
	grpcPanicRecoveryHandler := func(p any) (err error) {
		panicsTotal.Inc()
		logger.Error("recovered from panic, stack: " + string(debug.Stack()))
		return status.Errorf(codes.Internal, "%s", p)
	}
	logTraceID := func(ctx context.Context) logging.Fields {
		if span := trace.SpanContextFromContext(ctx); span.IsSampled() {
			return logging.Fields{"traceID", span.TraceID().String()}
		}
		return nil
	}
	logOpts := []logging.Option{
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall),
		logging.WithDurationField(logging.DurationToTimeMillisFields),
		logging.WithFieldsFromContext(logTraceID),
	}

	opts = append(opts,
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainStreamInterceptor(
			serviceAuthStreamInterceptor,
			srvMetrics.StreamServerInterceptor(grpcprom.WithExemplarFromContext(exemplarFromContext)),
			logging.StreamServerInterceptor(interceptorLogger(logger), logOpts...),
			recovery.StreamServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)),
		),
		grpc.ChainUnaryInterceptor(
			serviceAuthUnaryInterceptor,
			srvMetrics.UnaryServerInterceptor(grpcprom.WithExemplarFromContext(exemplarFromContext)),
			logging.UnaryServerInterceptor(interceptorLogger(logger), logging.WithFieldsFromContext(logTraceID)),
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)),
		),
	)
	if credentials := realtime.MustServerTransportCredentials(); credentials != nil {
		opts = append(opts, grpc.Creds(credentials))
	}
	grpcSrv := grpc.NewServer(opts...)
	srvMetrics.InitializeMetrics(grpcSrv)
	return grpcSrv
}

func endUserAssertionClientInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	request, ok := req.(proto.Message)
	if !ok {
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	parts := strings.SplitN(strings.TrimPrefix(method, "/"), "/", 2)
	if len(parts) != 2 {
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	callCtx, err := realtime.ProtoAssertionMetadata(ctx, parts[0], parts[1], request)
	if err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	return invoker(callCtx, method, req, reply, cc, opts...)
}

func InitializeGrpcClient(svcHost string) (*grpc.ClientConn, error) {
	scheme := "dns"

	retryOpts := []retry.CallOption{
		// generate waits between 900ms to 1100ms
		retry.WithBackoff(retry.BackoffLinearWithJitter(1*time.Second, 0.1)),
		retry.WithMax(3),
		retry.WithCodes(codes.Unavailable, codes.Aborted),
		retry.WithPerRetryTimeout(3 * time.Second),
	}

	handler := otelgrpc.NewClientHandler()

	slog.Info("connecting to grpc host: " + svcHost)
	client, err := grpc.NewClient(
		fmt.Sprintf("%s:///%s", scheme, svcHost),
		grpc.WithTransportCredentials(realtime.MustClientTransportCredentials()),
		grpc.WithStatsHandler(handler),
		grpc.WithDisableServiceConfig(),
		grpc.WithDefaultServiceConfig(`{
			"loadBalancingPolicy": "round_robin"
		}`),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
			Timeout:             time.Second,      // wait 1 second for ping ack before considering the connection dead
			PermitWithoutStream: true,             // send pings even without active streams
		}),
		grpc.WithChainUnaryInterceptor(
			endUserAssertionClientInterceptor,
			retry.UnaryClientInterceptor(retryOpts...),
		),
		grpc.WithChainStreamInterceptor(
			retry.StreamClientInterceptor(retryOpts...),
		),
		//grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func NewGrpcEndpoint(conn *grpc.ClientConn, serviceID, serviceName, method string, grpcReply interface{}) endpoint.Endpoint {
	var options []grpctransport.ClientOption
	var (
		ep         endpoint.Endpoint
		endpointer sd.FixedEndpointer
	)

	ep = grpctransport.NewClient(
		conn,
		serviceName,
		method,
		encodeGRPCRequest,
		decodeGRPCResponse,
		grpcReply,
		append(options, grpctransport.ClientBefore(
			grpctransport.SetRequestHeader(ServiceIdHeader, serviceID),
			grpctransport.SetRequestHeader("Authorization", "Bearer "+os.Getenv("NEXUSCHAT_GRPC_SHARED_SECRET")),
		))...,
	).Endpoint()
	ep = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:    serviceName + "." + method,
		Timeout: 60 * time.Second,
	}))(ep)
	endpointer = append(endpointer, ep)
	// timeout for the whole invocation
	ep = lb.Retry(1, 15*time.Second, lb.NewRoundRobin(endpointer))

	return ep
}

func encodeGRPCRequest(_ context.Context, request interface{}) (interface{}, error) {
	return request, nil
}

func decodeGRPCResponse(_ context.Context, grpcReply interface{}) (interface{}, error) {
	return grpcReply, nil
}
