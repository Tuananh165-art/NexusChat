package realtime

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const EndUserAssertionHeader = "x-nexuschat-end-user-assertion"

var replayedAssertions = struct {
	sync.Mutex
	values map[string]time.Time
}{values: make(map[string]time.Time)}

type signedEndUserAssertion struct {
	Version  int    `json:"v"`
	KeyID    string `json:"kid"`
	UserID   string `json:"uid,omitempty"`
	Channel  string `json:"cid,omitempty"`
	Audience string `json:"aud"`
	Method   string `json:"method"`
	Digest   string `json:"digest"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
	Nonce    string `json:"nonce"`
}

func assertionKeys() (ed25519.PrivateKey, ed25519.PublicKey, string, error) {
	keyID := strings.TrimSpace(os.Getenv("NEXUSCHAT_GRPC_ASSERTION_KEY_ID"))
	if keyID == "" {
		keyID = "default"
	}
	encoded := strings.TrimSpace(os.Getenv("NEXUSCHAT_GRPC_ASSERTION_PRIVATE_KEY"))
	if encoded == "" {
		return nil, nil, "", errors.New("end-user assertion private key is not configured")
	}
	privateBytes, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		privateBytes, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, nil, "", fmt.Errorf("decode assertion private key: %w", err)
	}
	var private ed25519.PrivateKey
	switch len(privateBytes) {
	case ed25519.SeedSize:
		private = ed25519.NewKeyFromSeed(privateBytes)
	case ed25519.PrivateKeySize:
		private = ed25519.PrivateKey(privateBytes)
	default:
		if block, pemErr := x509.ParsePKCS8PrivateKey(privateBytes); pemErr == nil {
			private, _ = block.(ed25519.PrivateKey)
		}
		if len(private) == 0 {
			return nil, nil, "", errors.New("assertion private key must be an Ed25519 seed, key, or PKCS8 key")
		}
	}
	public := private.Public().(ed25519.PublicKey)
	if encodedPublic := strings.TrimSpace(os.Getenv("NEXUSCHAT_GRPC_ASSERTION_PUBLIC_KEY")); encodedPublic != "" {
		publicBytes, decodeErr := base64.RawStdEncoding.DecodeString(encodedPublic)
		if decodeErr != nil {
			publicBytes, decodeErr = base64.StdEncoding.DecodeString(encodedPublic)
		}
		if decodeErr != nil || len(publicBytes) != ed25519.PublicKeySize {
			return nil, nil, "", errors.New("invalid end-user assertion public key")
		}
		public = ed25519.PublicKey(publicBytes)
	}
	return private, public, keyID, nil
}

func requestNeedsAssertion(fields map[string]any) bool {
	for _, name := range []string{"user_id", "channel_id"} {
		if value, ok := fields[name]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" && fmt.Sprint(value) != "0" {
			return true
		}
	}
	return false
}

func canonicalDigest(fields map[string]any) (string, error) {
	body, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func identityField(fields map[string]any, name string) string {
	value, ok := fields[name]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatUint(uint64(typed), 10)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func signEndUserAssertion(fields map[string]any, audience, method string) (string, error) {
	private, _, keyID, err := assertionKeys()
	if err != nil {
		return "", err
	}
	digest, err := canonicalDigest(fields)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	now := time.Now().Unix()
	assertion := signedEndUserAssertion{
		Version: 1, KeyID: keyID, UserID: identityField(fields, "user_id"), Channel: identityField(fields, "channel_id"),
		Audience: audience, Method: method, Digest: digest, IssuedAt: now, Expires: now + 120,
		Nonce: base64.RawURLEncoding.EncodeToString(nonce),
	}
	body, err := json.Marshal(assertion)
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(private, body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func StructAssertionMetadata(ctx context.Context, service, method string, request *structpb.Struct) (context.Context, error) {
	fields := request.AsMap()
	if !requestNeedsAssertion(fields) {
		return ctx, nil
	}
	value, err := signEndUserAssertion(fields, service, "/"+service+"/"+method)
	if err != nil {
		return ctx, err
	}
	return metadata.AppendToOutgoingContext(ctx, EndUserAssertionHeader, value), nil
}

func ProtoAssertionMetadata(ctx context.Context, service, method string, request proto.Message) (context.Context, error) {
	fields, err := protoFields(request)
	if err != nil || !requestNeedsAssertion(fields) {
		return ctx, err
	}
	value, err := signEndUserAssertion(fields, service, "/"+service+"/"+method)
	if err != nil {
		return ctx, err
	}
	return metadata.AppendToOutgoingContext(ctx, EndUserAssertionHeader, value), nil
}

func protoFields(request proto.Message) (map[string]any, error) {
	if request == nil {
		return nil, nil
	}
	body, err := protojson.Marshal(request)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]any)
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func VerifyStructAssertion(ctx context.Context, request *structpb.Struct, fullMethod string) error {
	return verifyAssertion(ctx, request.AsMap(), fullMethod)
}

func VerifyProtoAssertion(ctx context.Context, request proto.Message, fullMethod string) error {
	fields, err := protoFields(request)
	if err != nil {
		return err
	}
	return verifyAssertion(ctx, fields, fullMethod)
}

func verifyAssertion(ctx context.Context, fields map[string]any, fullMethod string) error {
	if !requestNeedsAssertion(fields) {
		return nil
	}
	values := metadata.ValueFromIncomingContext(ctx, EndUserAssertionHeader)
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return errors.New("end-user assertion is required")
	}
	parts := strings.Split(values[0], ".")
	if len(parts) != 2 {
		return errors.New("invalid end-user assertion format")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("invalid end-user assertion payload")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return errors.New("invalid end-user assertion signature")
	}
	_, public, keyID, err := assertionKeys()
	if err != nil {
		return err
	}
	if !ed25519.Verify(public, body, signature) {
		return errors.New("invalid end-user assertion signature")
	}
	var assertion signedEndUserAssertion
	if err := json.Unmarshal(body, &assertion); err != nil || assertion.Version != 1 {
		return errors.New("invalid end-user assertion payload")
	}
	if assertion.KeyID != keyID || assertion.Method != fullMethod {
		return errors.New("end-user assertion scope mismatch")
	}
	expectedAudience := strings.TrimPrefix(strings.SplitN(strings.TrimPrefix(fullMethod, "/"), "/", 2)[0], "/")
	if assertion.Audience != expectedAudience {
		return errors.New("end-user assertion audience mismatch")
	}
	now := time.Now().Unix()
	if assertion.IssuedAt > now+30 || assertion.Expires <= now || assertion.Expires-assertion.IssuedAt > 120 {
		return errors.New("end-user assertion expired or invalid")
	}
	digest, err := canonicalDigest(fields)
	if err != nil || digest != assertion.Digest {
		return errors.New("end-user assertion request mismatch")
	}
	if assertion.UserID != identityField(fields, "user_id") || assertion.Channel != identityField(fields, "channel_id") {
		return errors.New("end-user assertion identity mismatch")
	}
	if assertion.Nonce == "" {
		return errors.New("end-user assertion nonce is missing")
	}
	replayedAssertions.Lock()
	defer replayedAssertions.Unlock()
	for nonce, expiry := range replayedAssertions.values {
		if expiry.Before(time.Now()) {
			delete(replayedAssertions.values, nonce)
		}
	}
	if _, exists := replayedAssertions.values[assertion.Nonce]; exists {
		return errors.New("end-user assertion replay detected")
	}
	replayedAssertions.values[assertion.Nonce] = time.Unix(assertion.Expires, 0)
	return nil
}
