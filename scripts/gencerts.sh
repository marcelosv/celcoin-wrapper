#!/usr/bin/env bash
set -euo pipefail
mkdir -p certs

# Simple self-signed server cert for localhost
cat > certs/openssl.cnf <<'EOF'
[req]
default_bits = 2048
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
C = BR
ST = SP
L = Sao Paulo
O = Celcoin Wrapper
CN = localhost

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
EOF

openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout certs/server.key -out certs/server.crt \
  -days 365 -config certs/openssl.cnf

echo "Self-signed server certificate generated in certs/server.crt and certs/server.key"
