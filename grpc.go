package weft

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	grpcTraceIDKey = "weft-trace-id"
	grpcSpanIDKey  = "weft-span-id"
)

type grpcHelpers struct{}

var GRPC grpcHelpers

func (grpcHelpers) Outgoing(ctx context.Context) context.Context {
	traceID, ok := TraceIDFromContext(ctx)
	if !ok {
		return ctx
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(grpcTraceIDKey, traceID)
	if spanID, ok := CurrentSpanID(ctx); ok {
		md.Set(grpcSpanIDKey, spanID)
	} else {
		md.Delete(grpcSpanIDKey)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func (grpcHelpers) Incoming(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	ids := md.Get(grpcTraceIDKey)
	if len(ids) == 0 || ids[0] == "" {
		return ctx
	}
	ctx = ContextWithTraceID(ctx, ids[0])
	if ids := md.Get(grpcSpanIDKey); len(ids) > 0 && ids[0] != "" {
		ctx = ContextWithSpanID(ctx, ids[0])
	}
	return ctx
}

func (grpcHelpers) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(GRPC.Outgoing(ctx), method, req, reply, cc, opts...)
	}
}

func (grpcHelpers) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(GRPC.Outgoing(ctx), desc, cc, method, opts...)
	}
}

func (grpcHelpers) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(GRPC.Incoming(ctx), req)
	}
}

func (grpcHelpers) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &stream{ServerStream: ss, ctx: GRPC.Incoming(ss.Context())})
	}
}

type stream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stream) Context() context.Context { return s.ctx }
