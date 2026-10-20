#!/bin/sh
# A throwaway CA and a certificate for the server container.
# KEY=ecdsa gives the server a P-256 key instead of RSA 2048, which changes its handshake cost.
set -e
cd "$(dirname "$0")"
openssl req -x509 -newkey rsa:2048 -nodes -days 365 -subj /CN=http-pool-lab-ca \
  -keyout ca.key -out ca.pem 2>/dev/null
if [ "${KEY:-rsa}" = ecdsa ]; then
  openssl ecparam -name prime256v1 -genkey -noout -out server.key
else
  openssl genrsa -out server.key 2048 2>/dev/null
fi
openssl req -new -key server.key -subj /CN=server -out server.csr
printf 'subjectAltName=DNS:server,DNS:localhost,IP:127.0.0.1\n' > san.ext
openssl x509 -req -in server.csr -CA ca.pem -CAkey ca.key -CAcreateserial -days 365 \
  -extfile san.ext -out server.crt 2>/dev/null
chmod 644 server.key
rm -f server.csr san.ext ca.srl
