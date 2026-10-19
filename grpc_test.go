package weft

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

type ids struct {
	trace, span       string
	hasTrace, hasSpan bool
}

func seen(ctx context.Context) ids {
	var r ids
	r.trace, r.hasTrace = TraceIDFromContext(ctx)
	r.span, r.hasSpan = CurrentSpanID(ctx)
	return r
}

type rig struct {
	client healthpb.HealthClient
	unary  chan ids
	stream chan ids
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{unary: make(chan ids, 1), stream: make(chan ids, 1)}
	lis := bufconn.Listen(1 << 16)
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(GRPC.UnaryServerInterceptor(),
			func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
				r.unary <- seen(ctx)
				return h(ctx, req)
			}),
		grpc.ChainStreamInterceptor(GRPC.StreamServerInterceptor(),
			func(s any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
				r.stream <- seen(ss.Context())
				return h(s, ss)
			}),
	)
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(GRPC.UnaryClientInterceptor()),
		grpc.WithChainStreamInterceptor(GRPC.StreamClientInterceptor()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	r.client = healthpb.NewHealthClient(conn)
	return r
}

func TestUnary_CarriesTraceAndSpan(t *testing.T) {
	r := newRig(t)
	ctx := ContextWithSpanID(ContextWithTraceID(context.Background(), "trace-1"), "span-1")
	if _, err := r.client.Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	got := <-r.unary
	if got.trace != "trace-1" || got.span != "span-1" {
		t.Errorf("handler saw %+v, want trace-1 / span-1", got)
	}
}

func TestStream_CarriesTraceAndSpan(t *testing.T) {
	r := newRig(t)
	ctx, cancel := context.WithCancel(ContextWithSpanID(ContextWithTraceID(context.Background(), "trace-2"), "span-2"))
	defer cancel()
	st, err := r.client.Watch(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Recv(); err != nil {
		t.Fatal(err)
	}
	got := <-r.stream
	if got.trace != "trace-2" || got.span != "span-2" {
		t.Errorf("handler saw %+v, want trace-2 / span-2", got)
	}
}

func TestTraceWithoutSpan(t *testing.T) {
	r := newRig(t)
	ctx := ContextWithTraceID(context.Background(), "trace-3")
	r.client.Check(ctx, &healthpb.HealthCheckRequest{})
	got := <-r.unary
	if got.trace != "trace-3" || got.hasSpan {
		t.Errorf("handler saw %+v, want trace-3 and no span", got)
	}
}

func TestNoTraceSendsNothingAndLeavesCtxAlone(t *testing.T) {
	r := newRig(t)
	r.client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if got := <-r.unary; got.hasTrace || got.hasSpan {
		t.Errorf("handler saw %+v, want no trace or span", got)
	}
}

func TestSpanWithoutTraceIsIgnored(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(grpcSpanIDKey, "orphan"))
	if got := seen(GRPC.Incoming(ctx)); got.hasTrace || got.hasSpan {
		t.Errorf("incoming = %+v, want neither", got)
	}
}

func TestOutgoingReplacesStaleIDs(t *testing.T) {
	ctx := metadata.AppendToOutgoingContext(context.Background(), grpcTraceIDKey, "old", grpcSpanIDKey, "old-span")
	ctx = ContextWithTraceID(ctx, "new")
	md, _ := metadata.FromOutgoingContext(GRPC.Outgoing(ctx))
	if got := md.Get(grpcTraceIDKey); len(got) != 1 || got[0] != "new" {
		t.Errorf("trace keys = %v, want [new]", got)
	}
	if got := md.Get(grpcSpanIDKey); len(got) != 0 {
		t.Errorf("span keys = %v, want none (the ctx has no span)", got)
	}
}

func TestOutgoingKeepsOtherMetadata(t *testing.T) {
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-custom", "7")
	ctx = ContextWithTraceID(ctx, "t")
	md, _ := metadata.FromOutgoingContext(GRPC.Outgoing(ctx))
	if got := md.Get("x-custom"); len(got) != 1 || got[0] != "7" {
		t.Errorf("x-custom = %v, want [7]", got)
	}
}
