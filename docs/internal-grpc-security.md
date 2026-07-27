# Internal gRPC security

NexusChat has two independent controls for service-to-service gRPC:

1. The existing shared-secret/service identity authenticates the calling service.
2. An Ed25519 end-user assertion binds `user_id`, `channel_id`, audience, full RPC method, request digest, issue/expiry times, key ID, and a one-time nonce.
3. Optional mutual TLS authenticates both service certificates at the transport layer.

The assertion is required whenever a generic Struct RPC or typed protobuf request contains a non-zero `user_id` or `channel_id`. Missing keys fail closed; there is no unsigned end-user fallback.

## Local configuration

The default Compose profile keeps TLS disabled. To exercise assertions, generate a short-lived development Ed25519 key outside the repository and set the base64 seed/key and public key through `.env`. Never commit private keys.

Set these variables consistently for every internal caller and server:

```text
NEXUSCHAT_GRPC_ASSERTION_KEY_ID=key-2026-01
NEXUSCHAT_GRPC_ASSERTION_PRIVATE_KEY=<base64 Ed25519 seed or private key>
NEXUSCHAT_GRPC_ASSERTION_PUBLIC_KEY=<base64 Ed25519 public key>
```

For mTLS, set `NEXUSCHAT_GRPC_TLS_ENABLED=true` and provide a CA plus client/server certificate paths. The client certificate must be trusted by every server, and the server certificate must contain the configured `NEXUSCHAT_GRPC_TLS_SERVER_NAME` in its SAN. Use direct service DNS and gRPC ports; do not route internal mTLS through the public HTTP/h2c Traefik entrypoint.

Required TLS variables:

```text
NEXUSCHAT_GRPC_TLS_CA_FILE
NEXUSCHAT_GRPC_TLS_CLIENT_CERT_FILE
NEXUSCHAT_GRPC_TLS_CLIENT_KEY_FILE
NEXUSCHAT_GRPC_TLS_SERVER_CERT_FILE
NEXUSCHAT_GRPC_TLS_SERVER_KEY_FILE
NEXUSCHAT_GRPC_TLS_SERVER_NAME
```

## Helm

Set `grpcSecurity.enabled=true`, put the assertion variables in the configured runtime Secret, and create the Secret named by `grpcSecurity.tlsSecretName` with `ca.crt`, `client.crt`, `client.key`, `server.crt`, and `server.key`. The chart mounts that Secret read-only into every gRPC service and injects the file paths. Public ingress TLS is separate and must not be reused as an internal client-authentication CA without an explicit certificate design.

## Rotation and failure behavior

Use a new key ID during assertion rotation and deploy the corresponding public/private key pair atomically to callers and verifiers. Rotate mTLS certificates before expiry. A missing, invalid, expired, replayed, wrong-audience, wrong-method, or request-digest-mismatched assertion is rejected with `Unauthenticated`; an untrusted or missing client certificate is rejected during the TLS handshake.
