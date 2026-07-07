#!/bin/sh
# Generates the runtime config the SPA reads (window.__STRATOS__) from the
# chart-injected env, then serves the static bundle.
set -e
if [ -n "${STRATOS_API_HOST:-}" ]; then
  API_URL="${STRATOS_API_HOST}${STRATOS_API_PREFIX:-/api-v1}"
else
  API_URL="${STRATOS_API_URL:-}"
fi
cat > /usr/share/nginx/html/config.js <<CFG
window.__STRATOS__ = {
  apiUrl: "${API_URL}",
  authIssuer: "${STRATOS_OAUTH2_ISSUER:-}",
  authClientId: "${STRATOS_OAUTH2_CLIENT_ID:-}",
  authScope: "${STRATOS_OAUTH2_SCOPE:-openid profile email}"
};
CFG
exec nginx -g 'daemon off;'
