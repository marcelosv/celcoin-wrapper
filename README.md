# Celcoin Wrapper (Mock)

Servidor mock que simula rotas essenciais do Celcoin para desenvolvimento e testes locais, com suporte a TLS, mTLS opcional, Bearer token e disparo de webhooks.

## Rotas

- `POST /v5/token` → retorna `{access_token:"mock-token", token_type:"Bearer", expires_in:2400}`
- `GET /baas/v2/status` → requer `Authorization: Bearer ...`. Filtra por `id`, `endToEndId`, `movementType`, `clientCode` usando `testdata/status_responses.json`.
- `GET /baas/v2/accounts/{accountId}/statements?from=YYYY-MM-DD&to=YYYY-MM-DD` → requer `Authorization: Bearer ...`. Lê `testdata/statements_responses.json` e ajusta `accountId`.
- `POST /dda-servicewebhook-webservice/v1/webhook/register` → resposta 200 simples.
- `POST /_admin/fire-webhook` → dispara POST para `WEBHOOK_TARGET` com headers `X-Event-Type`, `Authorization: Bearer <WEBHOOK_BEARER>` e `X-Signature` (HMAC opcional).

## Execução

Pré-requisitos: Go 1.22+

```bash
# gerar certificados de desenvolvimento (autoassinado)
./scripts/gencerts.sh

# executar o servidor
TLS_CERT=certs/server.crt TLS_KEY=certs/server.key \
SERVER_ADDR=":8443" TESTDATA_DIR=testdata \
WEBHOOK_TARGET="http://localhost:8080/v1/webhooks/celcoin" \
GOLOG=info \
go run .
```

mTLS (opcional):

```bash
MTLS_ENABLED=true MTLS_CA_CERT=certs/ca.crt \
TLS_CERT=certs/server.crt TLS_KEY=certs/server.key \
go run .
```

## Disparo de webhook

```bash
curl -k -X POST https://localhost:8443/_admin/fire-webhook \
  -H 'Content-Type: application/json' \
  -d '{"type":"transaction.created","payload":{"id":"c0a8-001","endToEndId":"E1...","accountId":"acc_123"}}'
```

Se usar mTLS, inclua `--cert` e `--key` do cliente.

## Variáveis de ambiente

- `SERVER_ADDR` (default `:8443`)
- `TLS_CERT` / `TLS_KEY` (caminhos dos arquivos PEM do servidor)
- `MTLS_ENABLED` (`true|false`) e `MTLS_CA_CERT` (CA que assina o client cert)
- `TESTDATA_DIR` (default `testdata`)
- `WEBHOOK_TARGET` (default `http://localhost:8080/v1/webhooks/celcoin`)
- `WEBHOOK_BEARER` (opcional) e `WEBHOOK_HMAC_SECRET` (opcional)

## Observações

- Para desenvolvimento rápido, rode sem mTLS e com `curl -k` para ignorar verificação do certificado autoassinado.
- Ajuste `testdata/` para criar cenários determinísticos, inclusive simulando erros/latência se desejar estender o código.
