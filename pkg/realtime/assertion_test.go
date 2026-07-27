package realtime

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"
)

const assertionTestService = "nexuschat.safety.v1.SafetyService"

func configureAssertionTestKey(t *testing.T) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	t.Setenv("NEXUSCHAT_GRPC_ASSERTION_PRIVATE_KEY", base64.RawStdEncoding.EncodeToString(seed))
	t.Setenv("NEXUSCHAT_GRPC_ASSERTION_KEY_ID", "test-key")
}

func incomingAssertionContext(ctx context.Context) context.Context {
	outgoing, _ := metadata.FromOutgoingContext(ctx)
	return metadata.NewIncomingContext(context.Background(), outgoing)
}

func TestStructEndUserAssertionValidAndReplayProtected(t *testing.T) {
	configureAssertionTestKey(t)
	request, err := structpb.NewStruct(map[string]any{"user_id": "42", "channel_id": "99", "content": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := StructAssertionMetadata(context.Background(), assertionTestService, "ModerateMessage", request)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyStructAssertion(incomingAssertionContext(ctx), request, "/"+assertionTestService+"/ModerateMessage"); err != nil {
		t.Fatalf("valid assertion rejected: %v", err)
	}
	if err := VerifyStructAssertion(incomingAssertionContext(ctx), request, "/"+assertionTestService+"/ModerateMessage"); err == nil || !strings.Contains(err.Error(), "replay") {
		t.Fatalf("replayed assertion was accepted: %v", err)
	}
}

func TestStructEndUserAssertionRejectsScopeDigestAndIdentityTampering(t *testing.T) {
	configureAssertionTestKey(t)
	request, _ := structpb.NewStruct(map[string]any{"user_id": "7", "channel_id": "8", "content": "hello"})
	ctx, err := StructAssertionMetadata(context.Background(), assertionTestService, "ModerateMessage", request)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		method string
		mutate func(*structpb.Struct)
	}{
		{name: "wrong method", method: "/" + assertionTestService + "/Other"},
		{name: "digest mismatch", method: "/" + assertionTestService + "/ModerateMessage", mutate: func(value *structpb.Struct) { value.Fields["content"] = structpb.NewStringValue("tampered") }},
		{name: "identity mismatch", method: "/" + assertionTestService + "/ModerateMessage", mutate: func(value *structpb.Struct) { value.Fields["user_id"] = structpb.NewStringValue("9") }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			copy := protoCloneStruct(request)
			if testCase.mutate != nil {
				testCase.mutate(copy)
			}
			if err := VerifyStructAssertion(incomingAssertionContext(ctx), copy, testCase.method); err == nil {
				t.Fatal("tampered assertion was accepted")
			}
		})
	}
}

func TestStructEndUserAssertionRejectsExpiredAssertion(t *testing.T) {
	configureAssertionTestKey(t)
	request, _ := structpb.NewStruct(map[string]any{"user_id": "1", "channel_id": "2"})
	private, _, keyID, err := assertionKeys()
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := canonicalDigest(request.AsMap())
	payload := signedEndUserAssertion{Version: 1, KeyID: keyID, UserID: "1", Channel: "2", Audience: assertionTestService, Method: "/" + assertionTestService + "/ModerateMessage", Digest: digest, IssuedAt: time.Now().Add(-5 * time.Minute).Unix(), Expires: time.Now().Add(-4 * time.Minute).Unix(), Nonce: "expired-test"}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(EndUserAssertionHeader, signedAssertionToken(t, private, payload)))
	if err := VerifyStructAssertion(ctx, request, payload.Method); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired assertion was accepted: %v", err)
	}
}

func signedAssertionToken(t *testing.T, private ed25519.PrivateKey, payload signedEndUserAssertion) string {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, body))
}

func protoCloneStruct(value *structpb.Struct) *structpb.Struct {
	copy, _ := structpb.NewStruct(value.AsMap())
	return copy
}
