# Celcoin Wrapper API

Documentação da API do mock `celcoin-wrapper` para consulta e integração local.

## Visão geral

- Base URL (dev): `https://localhost:8443`
- TLS: sempre ativo. Use `curl -k` para ignorar certificado autoassinado em dev.
- mTLS: opcional. Habilite via `MTLS_ENABLED=true` e informe `MTLS_CA_CERT`.
- Autenticação: Bearer token em rotas protegidas.
- Conteúdo: `application/json` para requisições e respostas.

## Autenticação

1) Obtenha token

- Método/rota: `POST /v5/token`
- Corpo (exemplo):
```json
{
  "client_id": "mock",
  "client_secret": "mock",
  "grant_type": "client_credentials",
  "scope": "baas.read"
}
```
- Resposta:
```json
{
  "access_token": "mock-token",
  "token_type": "Bearer",
  "expires_in": 2400
}
```
- Observações: o token é estático no mock (`mock-token`).

2) Usar Bearer nas rotas protegidas

- Header: `Authorization: Bearer mock-token`

## Rotas

### 1. Consultar status de transações

- Método/rota: `GET /baas/v2/status`
- Headers: `Authorization: Bearer <token>`
- Query params suportados (opcionais; combináveis):
  - `id`
  - `endToEndId`
  - `movementType`
  - `clientCode`
- Exemplo:
```bash
curl -k -H "Authorization: Bearer mock-token" \
  "https://localhost:8443/baas/v2/status?id=c0a8-001"
```
- Resposta (exemplo):
```json
{
  "items": [
    {
      "id": "c0a8-001",
      "movementType": "PIXPAYMENTOUT",
      "bookingDate": "2025-10-10T18:13:21Z",
      "amount": 1250.5,
      "currency": "BRL",
      "description": "Pix para Fornecedor X",
      "endToEndId": "E1...",
      "nsu": "NSU123",
      "channel": "PIX",
      "clientCode": "cli-001",
      "counterparty": { "name": "Fornecedor X" }
    }
  ],
  "page": 1,
  "pageSize": 1
}
```
- Fonte de dados: `testdata/status_responses.json`. O mock filtra por chaves se fornecidas.
- Códigos de status:
  - `200 OK`: sucesso
  - `401 Unauthorized`: Bearer ausente
  - `405 Method Not Allowed`: método inválido
  - `500 Internal Server Error`: falha ao ler testdata

### 2. Extrato/saldos por conta

- Método/rota: `GET /baas/v2/accounts/{accountId}/statements`
- Headers: `Authorization: Bearer <token>`
- Query params obrigatórios:
  - `from=YYYY-MM-DD`
  - `to=YYYY-MM-DD`
- Exemplo:
```bash
curl -k -H "Authorization: Bearer mock-token" \
  "https://localhost:8443/baas/v2/accounts/acc_123/statements?from=2025-10-01&to=2025-10-31"
```
- Resposta (exemplo):
```json
{
  "accountId": "acc_123",
  "openingBalance": 1000.0,
  "closingBalance": 1250.5,
  "currency": "BRL"
}
```
- Fonte de dados: `testdata/statements_responses.json`. O `accountId` é ajustado com o da URL. Caso o arquivo não exista, o mock usa valores padrão.
- Códigos de status:
  - `200 OK`: sucesso
  - `400 Bad Request`: path inválido
  - `401 Unauthorized`: Bearer ausente
  - `405 Method Not Allowed`: método inválido

### 3. Registro de webhook (opcional)

- Método/rota: `POST /dda-servicewebhook-webservice/v1/webhook/register`
- Corpo: livre (não validado neste mock).
- Resposta:
```json
{ "status": "registered" }
```
- Códigos de status:
  - `200 OK`: sucesso
  - `405 Method Not Allowed`: método inválido

### 4. Disparo administrativo de webhook

- Método/rota: `POST /_admin/fire-webhook`
- Função: envia um POST para `WEBHOOK_TARGET` (ex.: `http://localhost:8080/v1/webhooks/celcoin`).
- Headers enviados:
  - `Content-Type: application/json`
  - `X-Event-Type: <type>` (se presente no corpo)
  - `Authorization: Bearer <WEBHOOK_BEARER>` (se configurado)
  - `X-Signature: sha256=<hex>` com HMAC do corpo (se `WEBHOOK_HMAC_SECRET` configurado)
- Corpo (exemplo):
```json
{
  "type": "transaction.created",
  "payload": {"id": "c0a8-001", "endToEndId": "E1...", "accountId": "acc_123"}
}
```
- Exemplo:
```bash
curl -k -X POST https://localhost:8443/_admin/fire-webhook \
  -H 'Content-Type: application/json' \
  -d '{"type":"transaction.created","payload":{"id":"c0a8-001","endToEndId":"E1...","accountId":"acc_123"}}'
```
- Resposta (do mock):
```json
{
  "target": "http://localhost:8080/v1/webhooks/celcoin",
  "status_code": 200,
  "responseBody": "... corpo devolvido pelo serviço alvo ..."
}
```
- Códigos de status:
  - `200 OK`: request ao alvo concluído (independente do status do alvo; status do alvo vem no corpo)
  - `405 Method Not Allowed`: método inválido
  - `502 Bad Gateway`: falha de conexão com o alvo

## Saúde

- `GET /healthz` → `200 OK` com corpo `ok`.

## Segurança e mTLS

- Em produção real, mTLS é obrigatório. Neste mock:
  - `MTLS_ENABLED=true` exige certificado de cliente assinado pela CA em `MTLS_CA_CERT`.
  - Para testar com `curl` e mTLS: `curl --cert client.crt --key client.key https://localhost:8443/...`
- Não use segredos reais. O mock é para ambiente local/integração interna.
